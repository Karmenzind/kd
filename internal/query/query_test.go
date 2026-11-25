package query

import (
	"path/filepath"
	"testing"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
)

func useTempCacheDB(t *testing.T) {
	t.Helper()
	originalRoot, originalDB := cache.CACHE_ROOT_PATH, cache.LiteDB
	originalLongText := cache.LONG_TEXT_CACHE_FILE
	root := t.TempDir()
	cache.CACHE_ROOT_PATH = root
	cache.LONG_TEXT_CACHE_FILE = filepath.Join(root, "long_text_results.json")
	if err := cache.InitDB(); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	db := cache.LiteDB
	t.Cleanup(func() {
		db.Close()
		cache.CACHE_ROOT_PATH = originalRoot
		cache.LiteDB = originalDB
		cache.LONG_TEXT_CACHE_FILE = originalLongText
	})
}

func TestFetchCachedWordHit(t *testing.T) {
	useTempCacheDB(t)

	stored := &model.Result{
		BaseResult: &model.BaseResult{Query: "abandon", IsEN: true, Found: true},
		Keyword:    "abandon",
		Paraphrase: []string{"v. 放弃"},
	}
	if err := cache.UpdateQueryCache(stored); err != nil {
		t.Fatalf("UpdateQueryCache() error = %v", err)
	}

	got := &model.Result{BaseResult: &model.BaseResult{Query: "abandon", IsEN: true}}
	if err := FetchCached(got); err != nil {
		t.Fatalf("FetchCached() error = %v", err)
	}
	if !got.Found || got.Keyword != "abandon" {
		t.Fatalf("FetchCached() = %+v, want a found result", got)
	}
}

func TestFetchCachedLongTextHit(t *testing.T) {
	useTempCacheDB(t)

	stored := &model.Result{
		BaseResult: &model.BaseResult{Query: "long paragraph", IsLongText: true, MachineTrans: "译文"},
	}
	if err := cache.UpdateLongTextCache(stored); err != nil {
		t.Fatalf("UpdateLongTextCache() error = %v", err)
	}

	got := &model.Result{BaseResult: &model.BaseResult{Query: "long paragraph", IsLongText: true}}
	if err := FetchCached(got); err != nil {
		t.Fatalf("FetchCached() error = %v", err)
	}
	if !got.Found || got.MachineTrans != "译文" {
		t.Fatalf("FetchCached() = %+v, want long-text cache hit", got)
	}
}

func TestFetchCachedMiss(t *testing.T) {
	useTempCacheDB(t)

	got := &model.Result{BaseResult: &model.BaseResult{Query: "missing", IsEN: true}}
	if err := FetchCached(got); err == nil {
		t.Fatal("FetchCached() on cache miss returned nil error")
	}
	if got.Found {
		t.Fatal("FetchCached() set Found=true on cache miss")
	}
}

func TestFetchOutcomeString(t *testing.T) {
	for outcome, want := range map[fetchOutcome]string{
		fetchFound:    "found",
		fetchNotFound: "not found",
		fetchUnknown:  "unknown",
	} {
		if got := outcome.String(); got != want {
			t.Fatalf("fetchOutcome(%d).String() = %q, want %q", outcome, got, want)
		}
	}
}
