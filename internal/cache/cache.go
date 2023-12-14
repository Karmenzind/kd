package cache

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/Karmenzind/kd/internal/model"
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
	}

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
	err = saveJson(getQueryCacheFilePath(r.Query), r)
	if err != nil {
		zap.S().Errorf("Failed to update cache for '%s'. Error: %s", r.Query, err)
	}
	return
}

func getQueryCacheFilePath(query string) string {
	fpath := filepath.Join(CACHE_WORDS_PATH, query)
	return fpath
}
