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
	"sync"
	"time"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/pkg"
	"go.uber.org/zap"
)

func GetCachedQuery(r *model.Result) (err error) {
	z, err := getCachedRow(r.Query, r.IsEN)
	if err != nil {
		zap.S().Debugf("Failed to query database for %s: %s", r.Query, err)
		return
	}

	j, err := decompressDetail(z)
	if err != nil {
		zap.S().Debugf("Failed to decompress data for %s: %s", r.Query, err)
		return
	}
	zap.S().Debugf("Got cached json %s", j)

	if len(j) > 0 {
		err = json.Unmarshal(j, r)
		if err != nil {
			zap.S().Debugf("Failed to unmarshal for %s: %s", r.Query, err)
			return
		}
		r.Sanitize()
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

	detail, err := compressDetail(j)
	if err != nil {
		zap.S().Warnf("Failed to compress detail for '%s': %s", r.Query, err)
		return
	}

	err = saveCachedRow(r.Query, r.IsEN, detail)

	if err != nil {
		zap.S().Errorf("Failed to update cache for '%s'. Error: %s", r.Query, err)
	}
	zap.S().Debugf("Updated cache for '%s'. len: %d", r.Query, len(detail))
	return
}

func decompressDetail(detail []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(detail))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var body bytes.Buffer
	if _, err = io.Copy(&body, reader); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

func compressDetail(body []byte) ([]byte, error) {
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(body); err != nil {
		writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

//  -----------------------------------------------------------------------------
//  Long Text query cache
//  -----------------------------------------------------------------------------

type LongTextData struct {
	Result   string `json:"r"`
	AccessTS int64  `json:"a"`
	CreateTS int64  `json:"c"`
}

var longTextCacheMu sync.Mutex

func loadLongTextCache(path string) (map[string]LongTextData, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]LongTextData), nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return make(map[string]LongTextData), nil
	}

	cacheData := make(map[string]LongTextData)
	if err := json.Unmarshal(body, &cacheData); err != nil {
		return nil, err
	}
	if cacheData == nil { // Preserve compatibility with files containing JSON null.
		cacheData = make(map[string]LongTextData)
	}
	return cacheData, nil
}

func saveLongTextCache(path string, cacheData map[string]LongTextData) (err error) {
	body, err := json.Marshal(cacheData)
	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".long_text_results-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		temp.Close()
		os.Remove(tempPath)
	}()

	if err = temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err = temp.Write(body); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func GetLongTextCache(r *model.Result) (err error) {
	longTextCacheMu.Lock()
	defer longTextCacheMu.Unlock()

	m, err := loadLongTextCache(LONG_TEXT_CACHE_FILE)
	if err != nil {
		return err
	}
	res, ok := m[r.Query]
	if !ok {
		return fmt.Errorf("no cache for %s", r.Query)
	}
	r.MachineTrans = res.Result
	zap.S().Debugf("Got cached '%s'", r.Query)
	res.AccessTS = time.Now().Unix()
	m[r.Query] = res
	return saveLongTextCache(LONG_TEXT_CACHE_FILE, m)
}

func UpdateLongTextCache(r *model.Result) (err error) {
	longTextCacheMu.Lock()
	defer longTextCacheMu.Unlock()

	m, err := loadLongTextCache(LONG_TEXT_CACHE_FILE)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	m[r.Query] = LongTextData{Result: r.MachineTrans, AccessTS: now, CreateTS: now}
	return saveLongTextCache(LONG_TEXT_CACHE_FILE, m)
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
