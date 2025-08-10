package run

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Karmenzind/kd/internal/model"
	d "github.com/Karmenzind/kd/pkg/decorate"
)

var Info *model.RunInfo

var CACHE_ROOT_PATH string      // kdCache目录完整路径
var CACHE_WORDS_PATH string     // 放word缓存文件的目录完整路径
var CACHE_AUDIO_DIR_PATH string // 放word缓存文件的目录完整路径
var CACHE_RUN_PATH string       // 存放运行信息
var CACHE_STAT_DIR_PATH string

// -----------------------------------------------------------------------------

// TODO (k) 支持config
var SERVER_PORT = 19707

var cacheDirname = "kdcache"

func getCacheRootPath() string {
	var target string
	userdir, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "linux", "windows":
		target = filepath.Join(userdir, ".cache", cacheDirname)
	case "darwin":
		target = filepath.Join(userdir, "Library/Caches", cacheDirname)
	default:
		d.EchoWarn("检测到架构为%s，此平台尚未进行测试，如出现问题请通过issue反馈", runtime.GOOS)
		target = filepath.Join(userdir, ".cache", cacheDirname)
	}
	return target

}

func init() {
	exepath, err := os.Executable()
	if err != nil {
		d.EchoFatal(err.Error())
	}
	Info = &model.RunInfo{
		PID:       os.Getpid(),
		StartTime: time.Now().Unix(),
		ExePath:   exepath,
		ExeName:   filepath.Base(exepath),
	}

	CACHE_ROOT_PATH = getCacheRootPath()
	CACHE_WORDS_PATH = filepath.Join(CACHE_ROOT_PATH, "words")
	CACHE_AUDIO_DIR_PATH = filepath.Join(CACHE_ROOT_PATH, "audio")
	CACHE_STAT_DIR_PATH = filepath.Join(CACHE_ROOT_PATH, "stat")
	CACHE_RUN_PATH = filepath.Join(CACHE_ROOT_PATH, "run")

	for _, directory := range []string{
		CACHE_ROOT_PATH,
		CACHE_WORDS_PATH,
		CACHE_STAT_DIR_PATH,
		CACHE_RUN_PATH,
		CACHE_AUDIO_DIR_PATH,
	} {
		err := os.MkdirAll(directory, os.ModePerm)
		if err != nil {
			d.EchoFatal(fmt.Sprintf("Failed to create %s", directory))
		}
	}

}
