package daemon

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"github.com/Karmenzind/kd/pkg/systemd"

	"github.com/shirou/gopsutil/v4/process"
	"go.uber.org/zap"
)

var SYSTEMD_UNIT_NAME = "kd-server"

func getKdPIDs() {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("taskkill", "/NH", "/FO", "csv")
	}
	output, err := cmd.Output()
	if err != nil {
		_ = err
	}
	strings.Split(string(output), "\n")
}

func ServerIsRunning() bool {
	_, err := PingDaemon(DefaultAddress())
	return err == nil
}

func processNameMatched(n string) bool {
	return n == "kd" || (runtime.GOOS == "windows" && n == "kd.exe")
}

func FindServerProcess() (*process.Process, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}
	for _, p := range processes {
		// XXX err
		n, _ := p.Name()
		di, err := GetDaemonInfo()
		if err == nil && p.Pid == int32(di.PID) {
			zap.S().Debugf("Got daemon process %v via daemon info", di.PID)
			cmdslice, _ := p.CmdlineSlice()
			if len(cmdslice) > 1 && cmdslice[1] == "--server" {
				return p, nil
			}
		}

		if processNameMatched(n) {
			cmd, _ := p.Cmdline()
			zap.S().Debugf("Found process kd with PID: %v CMD: %s", p.Pid, cmd)
			// user, _ := p.Username()
			// zap.S().Debugf("Found process kd with PID: %v CMD: %s User: %s", p.Pid, cmd, user)
			// cmdSlice, _ := p.CmdlineSlice()
			// zap.S().Debugf("Found process kd with CMD slice: %s", cmdSlice)
			if strings.Contains(cmd, " --server") {
				zap.S().Debugf("Found deamon process %+v Cmd: `%s`", p, cmd)
				return p, nil
			}
		}
	}
	return nil, nil
}

func StartDaemonProcess() error {
	ping, err := startDaemon(DefaultAddress(), DaemonStartTimeout, launchDaemonProcess)
	if err != nil {
		return err
	}
	zap.S().Info("Started or reused daemon process")
	d.EchoOkay("守护进程已就绪，PID：%d", ping.PID)
	return nil
}

type daemonLauncher func() (<-chan error, error)

func launchDaemonProcess() (<-chan error, error) {
	kdpath, err := pkg.GetExecutablePath()
	if err != nil {
		zap.S().Errorf("Failed to get current file path: %s", err)
		return nil, err
	}
	zap.S().Debugf("Got executable path %s", kdpath)

	cmd := exec.Command(kdpath, "--server")
	if err = cmd.Start(); err != nil {
		zap.S().Errorf("Failed to start daemon with system command: %s", err)
		return nil, err
	}
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		close(exited)
	}()
	return exited, nil
}

func startDaemon(addr string, timeout time.Duration, launch daemonLauncher) (*model.DaemonPing, error) {
	ping, err := PingDaemon(addr)
	if err == nil {
		return ping, nil
	}
	if !errors.Is(err, ErrDaemonNotRunning) {
		if errors.Is(err, ErrNotKDDaemon) || errors.Is(err, ErrDaemonNoResponse) {
			return nil, errors.Join(ErrPortOccupied, err)
		}
		return nil, err
	}
	exited, err := launch()
	if err != nil {
		return nil, errors.Join(ErrDaemonInit, err)
	}
	return WaitDaemonReady(addr, timeout, exited)
}

func WaitDaemonReady(addr string, timeout time.Duration, exited <-chan error) (*model.DaemonPing, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(DaemonRetryInterval)
	defer ticker.Stop()
	var lastErr error
	var exitErr error
	var processExited bool
	for {
		ping, err := PingDaemon(addr)
		if err == nil {
			return ping, nil
		}
		lastErr = err
		if errors.Is(err, ErrNotKDDaemon) || errors.Is(err, ErrProtocolIncompatible) {
			return nil, errors.Join(ErrPortOccupied, err)
		}
		select {
		case err, ok := <-exited:
			processExited = true
			if ok {
				exitErr = err
			}
			exited = nil
		default:
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			err := errors.Join(ErrDaemonStartTimeout, lastErr)
			if exitErr != nil {
				err = errors.Join(err, fmt.Errorf("daemon process exited: %w", exitErr))
			} else if processExited {
				err = errors.Join(err, errors.New("daemon process exited before becoming ready"))
			}
			return nil, err
		}
	}
}

func KillDaemonIfRunning() error {
	if runtime.GOOS == "linux" {
		if yes, _ := systemd.ServiceIsActive(SYSTEMD_UNIT_NAME, true); yes {
			d.EchoWarn("检测到daemon作为systemd unit运行，将使用systemctl停止，再次启动需执行systemctl start --user %s", SYSTEMD_UNIT_NAME)
			_, err := systemd.StopService(SYSTEMD_UNIT_NAME, true)
			if err == nil {
				d.EchoOkay("已经通过systemd停止kd-server服务")
			}
			return err
		}
	}
	ping, err := PingDaemon(DefaultAddress())
	if errors.Is(err, ErrDaemonNotRunning) {
		d.EchoOkay("未发现守护进程，无需停止")
		return nil
	}
	if err != nil {
		return err
	}
	if err := requestShutdown(DefaultAddress()); err != nil {
		return err
	}
	deadline := time.Now().Add(DaemonStartTimeout)
	for time.Now().Before(deadline) {
		if _, err := PingDaemon(DefaultAddress()); errors.Is(err, ErrDaemonNotRunning) {
			zap.S().Info("Stopped daemon process")
			d.EchoOkay("守护进程已经停止")
			return nil
		}
		time.Sleep(DaemonRetryInterval)
	}
	return fmt.Errorf("停止守护进程 PID %d 超时", ping.PID)
}

func requestShutdown(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, PingTimeout)
	if err != nil {
		return fmt.Errorf("connect daemon for shutdown: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(ConnectionTimeout)); err != nil {
		return err
	}
	request := model.TCPQuery{Action: "shutdown", ProtocolVersion: model.DaemonProtocolVersion}
	if err := model.WriteProtocolMessage(conn, request); err != nil {
		return fmt.Errorf("send daemon shutdown request: %w", err)
	}
	var response model.DaemonResponse
	if err := model.NewProtocolReader(conn).Read(&response); err != nil {
		return fmt.Errorf("read daemon shutdown response: %w", err)
	}
	if response.Error != "" {
		return errors.New(response.Error)
	}
	return nil
}

// TODO (k): <2024-05-05 15:56>
func SendHUP2Daemon() error {
	return nil
}

func RestartDaemon() error {
	var err error
	if runtime.GOOS == "linux" {
		if yes, _ := systemd.ServiceIsActive(SYSTEMD_UNIT_NAME, true); yes {
			zap.S().Debugf("Found systemd unit: %s", SYSTEMD_UNIT_NAME)
			d.EchoWarn("检测到daemon存在相应systemd unit，将使用systemctl重启")
			_, err = systemd.RestartService(SYSTEMD_UNIT_NAME, true)
			if err == nil {
				d.EchoOkay("已经通过systemctl重启daemon服务")
			}
			return err
		}
	}
	err = KillDaemonIfRunning()
	if err == nil {
		err = StartDaemonProcess()
	}
	return err
}
