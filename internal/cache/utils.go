package cache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Karmenzind/kd/internal/run"
	d "github.com/Karmenzind/kd/pkg/decorate"
)

var CACHE_ROOT_PATH = run.CACHE_ROOT_PATH
var CACHE_WORDS_PATH = run.CACHE_WORDS_PATH
var CACHE_RUN_PATH = run.CACHE_RUN_PATH
var CACHE_STAT_DIR_PATH = run.CACHE_STAT_DIR_PATH

var LONG_TEXT_CACHE_FILE string

func init() {
	for _, directory := range []string{
		CACHE_ROOT_PATH,
		CACHE_WORDS_PATH,
		CACHE_STAT_DIR_PATH,
		CACHE_RUN_PATH,
	} {
		err := os.MkdirAll(directory, os.ModePerm)
		if err != nil {
			d.EchoFatal(fmt.Sprintf("Failed to create %s", directory))
		}
	}
    LONG_TEXT_CACHE_FILE = filepath.Join(CACHE_ROOT_PATH, "long_text_results.json")
}
