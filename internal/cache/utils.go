package cache

import (
	"path/filepath"

	"github.com/Karmenzind/kd/internal/run"
)

var CACHE_ROOT_PATH = run.CACHE_ROOT_PATH
var CACHE_WORDS_PATH = run.CACHE_WORDS_PATH
var CACHE_RUN_PATH = run.CACHE_RUN_PATH
var CACHE_STAT_DIR_PATH = run.CACHE_STAT_DIR_PATH

var LONG_TEXT_CACHE_FILE string

func init() {
	LONG_TEXT_CACHE_FILE = filepath.Join(CACHE_ROOT_PATH, "long_text_results.json")
}
