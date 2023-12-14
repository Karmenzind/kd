package daemon

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"

	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"
)

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
		n, err := p.Name()
		if err != nil {
			return nil, err
		}
		if n == "kd" {
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

