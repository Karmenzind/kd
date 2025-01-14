package daemon

import (
    "errors"
    "fmt"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "time"

    "github.com/Karmenzind/kd/internal/model"
    "github.com/Karmenzind/kd/internal/run"
    "github.com/Karmenzind/kd/pkg"
    d "github.com/Karmenzind/kd/pkg/decorate"
    "github.com/Karmenzind/kd/pkg/proc"
    "github.com/Karmenzind/kd/pkg/systemd"

    "github.com/shirou/gopsutil/v3/process"
    "go.uber.org/zap"
)

var SYSTEMD_UNIT_NAME = "kd-server"
var DaemonInfo = &model.RunInfo{}

func GetDaemonInfoPath() string {
    return filepath.Join(run.CACHE_RUN_PATH, "daemon.json")
}

func GetDaemonInfoFromFile() (*model.RunInfo, error) {
    dipath := filepath.Join(run.CACHE_RUN_PATH, "daemon.json")
    if !pkg.IsPathExists(dipath) {
        return DaemonInfo, errors.New("获取守护进程信息失败，文件不存在")
    }
    err := pkg.LoadJson(dipath, DaemonInfo)
    return DaemonInfo, err
}

func GetDaemonInfo() (*model.RunInfo, error) {
    var err error
    if *DaemonInfo == (model.RunInfo{}) {
        dipath := filepath.Join(run.CACHE_RUN_PATH, "daemon.json")
        if !pkg.IsPathExists(dipath) {
            return DaemonInfo, errors.New("获取守护进程信息失败，文件不存在")
        }
        if err = pkg.LoadJson(dipath, DaemonInfo); err != nil {
            return DaemonInfo, err
        }
    }
    return DaemonInfo, err
}

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
        di, err := GetDaemonInfo()
        if err == nil && p.Pid == int32(di.PID) {
            zap.S().Debugf("Got daemon process %v via daemon info", di.PID)
            cmdslice, _ := p.CmdlineSlice()
            if len(cmdslice) > 1 && cmdslice[1] == "--server" {
                return p, nil
            }
        }

        if n == "kd" || (runtime.GOOS == "windows" && n == "kd.exe") {
            cmd, _ := p.Cmdline()
            // zap.S().Debugf("Found process kd with CMD: %s", cmd)
            if strings.Contains(cmd, " --server") {
                zap.S().Debugf("Found process %+v Cmd: `%s`", p, cmd)
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
        err = errors.New("启动失败，请重试。如果多次启动失败，请创建Issue并提交日志文件")
        return err
    }
    return nil
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
    p, err := FindServerProcess()
    if err == nil {
        if p == nil {
            d.EchoOkay("未发现守护进程，无需停止")
            return nil
        }
    } else {
        zap.S().Warnf("[process] Failed to find daemon: %s", err)
        return err
    }

    zap.S().Debugf("try killing process: %v", p)
    err = proc.KillProcess(p)

    if err == nil {
        zap.S().Info("Terminated daemon process.")
        d.EchoOkay("守护进程已经停止")
    } else {
        zap.S().Warnf("Failed to terminate daemon process: %s", err)
    }
    return err
}

// TODO (k): <2024-05-05 15:56>
func SendHUP2Daemon() error {
    return nil
}

func RestartDaemon() error {
    if runtime.GOOS == "linux" {
        if yes, _ := systemd.ServiceIsActive(SYSTEMD_UNIT_NAME, true); yes {
            zap.S().Debugf("Found systemd unit: %s", SYSTEMD_UNIT_NAME)
            d.EchoWarn("检测到daemon存在相应systemd unit，将使用systemctl重启")
            _, err := systemd.RestartService(SYSTEMD_UNIT_NAME, true)
            if err == nil {
                d.EchoOkay("已经通过systemctl重启daemon服务")
            }
            return err
        }
    }
    err := KillDaemonIfRunning()
    if err == nil {
        err = StartDaemonProcess()
    }
    return err
}
