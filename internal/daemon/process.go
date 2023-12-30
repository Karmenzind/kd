package daemon

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"

	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"
)

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
	p, _ := FindServerProcess()
	return p != nil
}

func FindServerProcess() (*process.Process, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}
	for _, p := range processes {
		// XXX err
		n, _ := p.Name()
		if n == "kd" || (runtime.GOOS == "windows" && n == "kd.exe") {
			cmd, _ := p.Cmdline()
			if strings.Contains(cmd, " --server") {
				zap.S().Debugf("Found process %+v Cmd: %s", p, cmd)
				return p, nil
			}
		}
	}
	return nil, nil
}

func StartDaemonProcess() error {
	kdpath, err := pkg.GetExecutablePath()
	if err != nil {
		zap.S().Errorf("Failed to get current file path: %s", err)
		return err
	}
	zap.S().Debugf("Got executable path %s", kdpath)

	cmd := exec.Command(kdpath, "--server")
	err = cmd.Start()
	if err != nil {
		zap.S().Errorf("Failed to start daemon with system command: %s", err)
		return err
	}
	var p *process.Process
	for i := 0; i < 3; i++ {
		time.Sleep(time.Second)
		p, err_ := FindServerProcess()
		if err_ != nil {
			zap.S().Warnf("Failed finding daemon process: %s", err_)
		}
		if p != nil {
			zap.S().Infof("Started daemon process.")
			d.EchoOkay(fmt.Sprintf("成功启动守护进程，PID：%d", p.Pid))
			return nil
		}
		d.EchoRun("正在检查运行结果，稍等...")
	}
	if p == nil {
		err = fmt.Errorf("启动失败，请重试。如果多次启动失败，请创建Issue并提交日志文件")
		return err
	}
	return nil
}

func KillDaemonIfRunning() error {
	p, err := FindServerProcess()
	var trySysKill bool
	if err == nil {
		if p == nil {
			d.EchoOkay("未发现守护进程，无需停止")
			return nil
		} else if runtime.GOOS != "windows" {
			zap.S().Infof("Found running daemon PID: %d,", p.Pid)
			errSig := p.SendSignal(syscall.SIGINT)
			if errSig != nil {
				zap.S().Warnf("Failed to stop PID %d with syscall.SIGINT: %s", p.Pid, errSig)
				trySysKill = true
			}
		} else {
			trySysKill = true
		}
	} else {
		zap.S().Warnf("[process] Failed to find daemon: %s", err)
		trySysKill = true
	}
	pidStr := strconv.Itoa(int(p.Pid))

	if trySysKill {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("taskkill", "/F", "/T", "/PID", pidStr)
			// cmd = exec.Command("taskkill", "/im", "kd", "/T", "/F")
		case "linux":
			cmd = exec.Command("kill", "-9", pidStr)
			// cmd = exec.Command("killall", "kd")
		}
		output, err := cmd.Output()
		zap.S().Infof("Executed '%s'. Output %s", cmd, output)
		if err != nil {
			zap.S().Warnf("Failed to kill daemon with system command. Error: %s", output, err)
		}
	}
	if err == nil {
		zap.S().Info("Terminated daemon process.")
		d.EchoOkay("守护进程已经停止")
	}
	return err
}
