package logger

import (
    "fmt"
    "os"
    "os/user"
    "path/filepath"
    "strings"

    "github.com/Karmenzind/kd/config"
    "go.uber.org/zap"
)

var LOG_FILE string

func buildLogger(logCfg *config.LoggerConfig, options ...zap.Option) (*zap.Logger, error) {
    cfg := zap.NewDevelopmentConfig()
    if logCfg.RedirectToStream {
        LOG_FILE = "[stream]"
        cfg.OutputPaths = []string{"stdout"}
        cfg.ErrorOutputPaths = []string{"stderr"}
    } else {
        var f string
        if logCfg.Path == "" {
            u, err := user.Current()
            if err != nil {
                f = filepath.Join(os.TempDir(), "kd.log")
            } else {
                name := strings.ReplaceAll(u.Username, " ", "_")
                name = strings.ReplaceAll(name, "\\", "_")
                f = filepath.Join(os.TempDir(), fmt.Sprintf("kd_%s.log", name))
            }
        } else {
            f = logCfg.Path
        }
        if _, err := os.Stat(f); err == nil {
            os.Chmod(f, 0o666)
        }
        LOG_FILE = f

        cfg.OutputPaths = []string{f}
        cfg.ErrorOutputPaths = []string{f}
    }
    level, err := zap.ParseAtomicLevel(logCfg.Level)
    if err != nil {
        return nil, err
    }
    cfg.Level = level
    return cfg.Build(options...)
}

func InitLogger(logCfg *config.LoggerConfig) (*zap.Logger, error) {
    l, err := buildLogger(logCfg)
    if err != nil {
        panic(err)
    }

    zap.ReplaceGlobals(l)

    return l, err
}
