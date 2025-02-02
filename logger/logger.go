package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/pkg"
	"go.uber.org/zap"
)

var LOG_FILE string

func getUserBasedLogfileName() string {
	username := pkg.GetCurUsername()
	if username == "" {
		return "kd.log"
	}
	name := strings.ReplaceAll(username, " ", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return fmt.Sprintf("kd_%s.log", name)
}

func buildLogger(logCfg *config.LoggerConfig, options ...zap.Option) (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	if logCfg.RedirectToStream {
		LOG_FILE = "[stream]"
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stderr"}
	} else {
		var f string
		if logCfg.Path == "" {
			f = filepath.Join(os.TempDir(), getUserBasedLogfileName())
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
		log.Panicln(err)
	}

	zap.ReplaceGlobals(l)
	return l, err
}
