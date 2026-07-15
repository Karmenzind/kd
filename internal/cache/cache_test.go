package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/core"
	"github.com/Karmenzind/kd/internal/model"
)

func setLongTextCachePath(t *testing.T) string {
	t.Helper()
	originalPath := LONG_TEXT_CACHE_FILE
	LONG_TEXT_CACHE_FILE = filepath.Join(t.TempDir(), "long_text_results.json")
	t.Cleanup(func() { LONG_TEXT_CACHE_FILE = originalPath })
	return LONG_TEXT_CACHE_FILE
}

func TestLongTextCacheRoundTrip(t *testing.T) {
	setLongTextCachePath(t)

	want := &model.Result{
		BaseResult: &model.BaseResult{Query: "-long text", IsLongText: true, MachineTrans: "translated"},
	}
	if err := UpdateLongTextCache(want); err != nil {
		t.Fatalf("UpdateLongTextCache() error = %v", err)
	}

	got := &model.Result{BaseResult: &model.BaseResult{Query: want.Query, IsLongText: true}}
	if err := GetLongTextCache(got); err != nil {
		t.Fatalf("GetLongTextCache() error = %v", err)
	}
	if got.MachineTrans != want.MachineTrans {
		t.Fatalf("MachineTrans = %q, want %q", got.MachineTrans, want.MachineTrans)
	}
}

func TestUpdateLongTextCacheRecoversEmptyState(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty file", body: ""},
		{name: "JSON null", body: "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := setLongTextCachePath(t)
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			result := &model.Result{
				BaseResult: &model.BaseResult{Query: "query", IsLongText: true, MachineTrans: "translated"},
			}
			if err := UpdateLongTextCache(result); err != nil {
				t.Fatalf("UpdateLongTextCache() error = %v", err)
			}

			stored, err := loadLongTextCache(path)
			if err != nil {
				t.Fatalf("loadLongTextCache() error = %v", err)
			}
			if got := stored[result.Query].Result; got != result.MachineTrans {
				t.Fatalf("stored result = %q, want %q", got, result.MachineTrans)
			}
		})
	}
}

func TestUpdateLongTextCacheSerializesConcurrentWrites(t *testing.T) {
	path := setLongTextCachePath(t)
	const count = 32

	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			query := fmt.Sprintf("query-%d", i)
			err := UpdateLongTextCache(&model.Result{
				BaseResult: &model.BaseResult{Query: query, IsLongText: true, MachineTrans: "translated-" + query},
			})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("UpdateLongTextCache() error = %v", err)
	}

	stored, err := loadLongTextCache(path)
	if err != nil {
		t.Fatalf("loadLongTextCache() error = %v", err)
	}
	if len(stored) != count {
		t.Fatalf("stored entries = %d, want %d", len(stored), count)
	}
}

func TestNotFoundLifecycle(t *testing.T) {
	originalRoot := CACHE_ROOT_PATH
	CACHE_ROOT_PATH = t.TempDir()
	t.Cleanup(func() { CACHE_ROOT_PATH = originalRoot })

	if line, err := CheckNotFound("missing"); err != nil || line != 0 {
		t.Fatalf("CheckNotFound() on an absent file = (%d, %v), want (0, nil)", line, err)
	}

	for _, query := range []string{"first", "target"} {
		if err := AppendNotFound(query); err != nil {
			t.Fatalf("AppendNotFound(%q) error = %v", query, err)
		}
	}

	if line, err := CheckNotFound("target"); err != nil || line != 2 {
		t.Fatalf("CheckNotFound(target) = (%d, %v), want (2, nil)", line, err)
	}

	if err := RemoveNotFound("target"); err != nil {
		t.Fatalf("RemoveNotFound(target) error = %v", err)
	}
	if line, err := CheckNotFound("target"); err != nil || line != 0 {
		t.Fatalf("CheckNotFound(target) after removal = (%d, %v), want (0, nil)", line, err)
	}
	if line, err := CheckNotFound("first"); err != nil || line != 1 {
		t.Fatalf("CheckNotFound(first) after removal = (%d, %v), want (1, nil)", line, err)
	}

	content, err := os.ReadFile(filepath.Join(CACHE_ROOT_PATH, "online_not_found"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(content), "first"; got != want {
		t.Fatalf("not-found file = %q, want %q", got, want)
	}
}

func TestCounterIncr(t *testing.T) {
	originalStatDir := CACHE_STAT_DIR_PATH
	CACHE_STAT_DIR_PATH = t.TempDir()
	t.Cleanup(func() { CACHE_STAT_DIR_PATH = originalStatDir })

	increment := func(query string) int {
		t.Helper()
		history := make(chan int, 1)
		core.WG.Add(1)
		CounterIncr(query, history)
		return <-history
	}

	if got := increment("abandon"); got != 1 {
		t.Fatalf("first CounterIncr() = %d, want 1", got)
	}
	if got := increment("abandon"); got != 2 {
		t.Fatalf("second CounterIncr() = %d, want 2", got)
	}
	if got := increment("other"); got != 1 {
		t.Fatalf("independent CounterIncr() = %d, want 1", got)
	}

	now := time.Now()
	counterPath := filepath.Join(CACHE_STAT_DIR_PATH, "counter-"+now.Format("200601")+".json")
	body, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", counterPath, err)
	}
	var counter MonthCounter
	if err := json.Unmarshal(body, &counter); err != nil {
		t.Fatalf("Unmarshal(counter) error = %v", err)
	}
	if counter["abandon"] != 2 || counter["other"] != 1 {
		t.Fatalf("counter = %#v, want abandon=2 and other=1", counter)
	}
}

func TestCounterIncrRecoversFromInvalidFile(t *testing.T) {
	originalStatDir := CACHE_STAT_DIR_PATH
	CACHE_STAT_DIR_PATH = t.TempDir()
	t.Cleanup(func() { CACHE_STAT_DIR_PATH = originalStatDir })

	counterPath := filepath.Join(CACHE_STAT_DIR_PATH, "counter-"+time.Now().Format("200601")+".json")
	if err := os.WriteFile(counterPath, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	history := make(chan int, 1)
	core.WG.Add(1)
	CounterIncr("abandon", history)
	if got := <-history; got != 0 {
		t.Fatalf("CounterIncr() with invalid state = %d, want fallback 0", got)
	}
}
