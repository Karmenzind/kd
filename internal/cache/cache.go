package cache

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/pkg"
	"go.uber.org/zap"
)

func GetCachedQuery(r *model.Result) (err error) {
	z, err := getCachedRow(r.Query, r.IsEN)
	if err != nil {
		zap.S().Debugf("Failed to query database for %s: %s", r.Query, err)
		return err
	}

	zb := bytes.NewBuffer(z)
	c, err := zlib.NewReader(zb)
	if err != nil {
		zap.S().Debugf("Failed to decompress data for %s: %s", r.Query, err)
		return err
	}
	var jb bytes.Buffer
	_, err = io.Copy(&jb, c)
	if err != nil {
		zap.S().Errorf("Failed to read buffer: %s", err)
	}
	c.Close()
	j := jb.Bytes()
	zap.S().Debugf("Got cached json %s", j)

	if len(j) > 0 {
		err = json.Unmarshal(j, r)
		if err != nil {
			zap.S().Debugf("Failed to unmarshal for %s: %s", r.Query, err)

			return err
		}
	}
	zap.S().Debugf("Got cached %s. (len: %d)", r.Query, len(j))
	return
}

func UpdateQueryCache(r *model.Result) (err error) {
	if !r.Found {
		return
	}

	j, err := json.Marshal(r)
	if err != nil {
		zap.S().Warnf("Failed to marshal %+v: %s", r, err)
		return
	}
	zap.S().Debugf("Got marshalled json to save: %s", j)

	var zb bytes.Buffer
	jw := zlib.NewWriter(&zb)
	jw.Write(j)
	jw.Close()
	detail := zb.Bytes()

	err = saveCachedRow(r.Query, r.IsEN, detail)

	if err != nil {
		zap.S().Errorf("Failed to update cache for '%s'. Error: %s", r.Query, err)
	}
	zap.S().Debugf("Updated cache for '%s'. len: %d", r.Query, len(detail))
	return
}

//  -----------------------------------------------------------------------------
//  Long Text query cache
//  -----------------------------------------------------------------------------

type LongTextData struct {
	Result   string `json:"r"`
	AccessTS int64  `json:"a"`
	CreateTS int64  `json:"c"`
}

func GetLongTextCache(r *model.Result) (err error) {
	var m map[string]LongTextData
	if pkg.IsPathExists(LONG_TEXT_CACHE_FILE) {
		err = pkg.LoadJson(LONG_TEXT_CACHE_FILE, &m)
		if err != nil {
			return err
		}
		if res, ok := m[r.Query]; ok {
			r.MachineTrans = res.Result
			zap.S().Debugf("Got cached '%s'", r.Query)
			(&res).AccessTS = time.Now().Unix()
			m[r.Query] = res
			go pkg.SaveJson(LONG_TEXT_CACHE_FILE, &m)
			return
		} else {
			return fmt.Errorf("no cache for %s", r.Query)
		}
	}
	return fmt.Errorf("cache file not found")
}

func UpdateLongTextCache(r *model.Result) (err error) {
	var m map[string]LongTextData
	if pkg.IsPathExists(LONG_TEXT_CACHE_FILE) {
		err = pkg.LoadJson(LONG_TEXT_CACHE_FILE, &m)
		if err != nil {
			return err
		}
	} else {
		m = map[string]LongTextData{}
	}
	now := time.Now().Unix()
	m[r.Query] = LongTextData{r.MachineTrans, now, now}
	err = pkg.SaveJson(LONG_TEXT_CACHE_FILE, m)
	return err
}

//  -----------------------------------------------------------------------------
// deprecated
//  JSON version
//  -----------------------------------------------------------------------------

func GetCachedQueryFromJson(r *model.Result) (err error) {
	fpath := getQueryCacheFilePath(r.Query)
	if _, err = os.Stat(fpath); errors.Is(err, os.ErrNotExist) {
		zap.S().Debugf("Cache file for '%s' doesn't exist.", r.Query)
		return err
	}
	j, err := os.ReadFile(fpath)
	if err != nil {
		zap.S().Debugf("Failed to read file for %s: %s", r.Query, err)
		return err
	}
	if len(j) > 0 {
		err = json.Unmarshal(j, r)
		if err != nil {
			zap.S().Debugf("Failed to unmarshal for %s: %s", r.Query, err)
			return err
		}
	}
	zap.S().Debugf("Got cached %s. (len: %d)", r.Query, len(j))
	return
}

func UpdateQueryCacheJson(r *model.Result) (err error) {
	if !r.Found {
		return
	}
	err = pkg.SaveJson(getQueryCacheFilePath(r.Query), r)
	if err != nil {
		zap.S().Errorf("Failed to update cache for '%s'. Error: %s", r.Query, err)
	}
	return
}

func getQueryCacheFilePath(query string) string {
	fpath := filepath.Join(CACHE_WORDS_PATH, query)
	return fpath
}
