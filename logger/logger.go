package logger

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/Karmenzind/kd/config"
	"go.uber.org/zap"
)

func buildLogger(logCfg *config.LoggerConfig, options ...zap.Option) (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	var f string
	if logCfg.Path == "" {
		u, err := user.Current()
		if err != nil {
			f = fmt.Sprintf("%s/kd.log", os.TempDir())
		} else {
			f = fmt.Sprintf("%s/kd_%s.log", os.TempDir(), strings.ReplaceAll(u.Username, " ", "_"))
		}
	} else {
		f = logCfg.Path
	}
	if _, err := os.Stat(f); err == nil {
		os.Chmod(f, 0o666)
	}

	cfg.OutputPaths = []string{f}
	cfg.ErrorOutputPaths = []string{f}

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
