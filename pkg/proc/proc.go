package proc

import (
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"go.uber.org/zap"
)

// not for Win
func KillProcess(p *process.Process) (err error) {
	if runtime.GOOS != "windows" {
		errSig := p.SendSignal(syscall.SIGINT)
		if errSig == nil {
			blockingTime := time.Duration(0)
			for {
				interval := time.Duration(50)
				zap.S().Debugf("Sleep for %v ms...", interval)
				time.Sleep(interval * time.Millisecond)
				blockingTime += interval
				yes, errCheck := p.IsRunning()
				zap.S().Infof("Checking if running (pid %v): %v err: %v", p.Pid, yes, errCheck)
				if blockingTime >= 1000 {
					break
				}
				if errCheck == nil && !yes {
					zap.S().Infof("Stopped process %v with SIGINT.", p.Pid)
					return
				}
			}
		} else {
			zap.S().Warnf("Failed to stop PID %v with syscall.SIGINT: %s", p.Pid, errSig)
		}
	}
	return SysKillPID(p.Pid)
}

func GetKillCMD(pid int32) *exec.Cmd {
	pidStr := strconv.Itoa(int(pid))
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("taskkill", "/F", "/T", "/PID", pidStr)
	case "linux", "darwin":
		cmd = exec.Command("kill", "-9", pidStr)
	default:
		cmd = exec.Command("kill", "-9", pidStr)
	}
	return cmd
}

func SysKillPID(pid int32) (err error) {
	cmd := GetKillCMD(pid)
	output, err := cmd.Output()

	zap.S().Infof("Executed '%s'. Output '%s'", cmd, output)
	if err != nil {
		var stderr string
		if exitE, ok := err.(*exec.ExitError); ok {
			stderr = string(exitE.Stderr)
		}
		zap.S().Warnf("Failed to kill %v with system command. Output: `%s` Error: `%s` StdErr: %q", pid, output, err, stderr)
	}
	return
}

func SendSignalToProcess(pid int32, signal syscall.Signal) error {
	return nil
}
