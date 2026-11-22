package query

import (
	"reflect"
	"testing"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
)

// A jsonapi_s result goes through the same cache as a legacy one; reading it
// back (including the read-time Sanitize) must give the same content.
func TestJSONAPIResultSurvivesCacheRoundTrip(t *testing.T) {
	originalRoot, originalDB := cache.CACHE_ROOT_PATH, cache.LiteDB
	cache.CACHE_ROOT_PATH = t.TempDir()
	if err := cache.InitDB(); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	db := cache.LiteDB
	t.Cleanup(func() {
		db.Close()
		cache.CACHE_ROOT_PATH, cache.LiteDB = originalRoot, originalDB
	})

	for _, tt := range []struct {
		file, query string
		isEN        bool
	}{
		{"take.json", "take", true},
		{"look_forward_to.json", "look forward to", true},
		{"arent.json", "aren't", true},
		{"nihao.json", "你好", false},
	} {
		t.Run(tt.query, func(t *testing.T) {
			stored := jsonapiResult(tt.query, tt.isEN, false, false)
			outcome, err := fetchJSONAPIFile(t, tt.file, stored)
			assertFound(t, outcome, err)
			stored.Found = true
			if err := cache.UpdateQueryCache(stored); err != nil {
				t.Fatalf("UpdateQueryCache() error = %v", err)
			}

			got := &model.Result{BaseResult: &model.BaseResult{Query: tt.query, IsEN: tt.isEN}}
			if err := cache.GetCachedQuery(got); err != nil {
				t.Fatalf("GetCachedQuery() error = %v", err)
			}
			got.BaseResult = stored.BaseResult
			if !reflect.DeepEqual(got, stored) {
				t.Fatalf("cached result differs:\n got  %+v\n want %+v", got, stored)
			}
		})
	}
}
