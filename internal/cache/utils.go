package cache

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/Karmenzind/kd/pkg"
)

var CACHE_ROOT_DIRNAME = "kdcache"
var CACHE_ROOT_PATH string        // kdCache目录完整路径
var CACHE_WORDS_PATH string       // 放word缓存文件的目录完整路径
var CACHE_RUN_PATH string         // 存放运行信息
var CACHE_STAT_DIR_PATH string

var saveJson = pkg.SaveJson
var loadJson = pkg.LoadJson

func ensureCacheDir() string {
	userdir, _ := os.UserHomeDir()
	var target string
	switch runtime.GOOS {
	case "linux":
		target = filepath.Join(userdir, ".cache", CACHE_ROOT_DIRNAME)
	case "darwin":
		target = filepath.Join(userdir, "Library/Caches", CACHE_ROOT_DIRNAME)
	case "windows":
		target = filepath.Join(userdir, ".cache", CACHE_ROOT_DIRNAME)
	}
	err := os.MkdirAll(target, os.ModePerm)
	if err != nil {
		panic("Failed to create cache dir")
	}
	return target
}

func init() {
	CACHE_ROOT_PATH = ensureCacheDir()
	CACHE_WORDS_PATH = filepath.Join(CACHE_ROOT_PATH, "words")
	CACHE_STAT_DIR_PATH = filepath.Join(CACHE_ROOT_PATH, "stat")
	CACHE_RUN_PATH = filepath.Join(CACHE_ROOT_PATH, "run")
	os.Mkdir(CACHE_WORDS_PATH, os.ModePerm)
	os.Mkdir(CACHE_STAT_DIR_PATH, os.ModePerm)
	os.Mkdir(CACHE_RUN_PATH, os.ModePerm)
}
