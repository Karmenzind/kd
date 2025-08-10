package main

import (
	"fmt"
	"log"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/internal/tts"
	"github.com/Karmenzind/kd/logger"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"go.uber.org/zap"
)

func main() {
	if err := config.InitConfig(); err != nil {
		if !pkg.HasAnyFlag("status", "edit-config", "generate-config") { // XXX (k): <2024-10-18 22:35> 可能不够
			d.EchoFatal(err.Error())
		}
		d.EchoWarn(err.Error())
	}
	cfg := config.Cfg
	d.ApplyConfig(cfg.EnableEmoji)


	if cfg.Logging.Enable {
		l, err := logger.InitLogger(&cfg.Logging)
		if err != nil {
			d.EchoFatal(err.Error())
		}
		defer func() {
			if r := recover(); r != nil {
				zap.S().Errorln("Application crashed", zap.Any("reason", r))
				if syncErr := l.Sync(); syncErr != nil {
					fmt.Printf("Failed to sync logger: %v\n", syncErr)
				}
			}
		}()
	}
	zap.S().Debugf("Got configuration: %+v", cfg)
	zap.S().Debugf("Got run info: %+v", run.Info)

	if err := tts.Speak("abandon") ; err != nil {
		log.Printf("Error: %s", err)
	}
}
