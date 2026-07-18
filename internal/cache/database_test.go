package cache

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

func useTestDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "cache.db")
	db, err := initDBAtPath(dbPath)
	if err != nil {
		t.Fatalf("initDBAtPath() error = %v", err)
	}

	originalDB := LiteDB
	LiteDB = db
	t.Cleanup(func() {
		db.Close()
		LiteDB = originalDB
	})
	return dbPath
}

func TestInitDBAtPathCreatesSchema(t *testing.T) {
	useTestDB(t)

	for _, table := range []string{"en", "ch"} {
		var definition string
		err := LiteDB.QueryRow(
			"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?",
			table,
		).Scan(&definition)
		if err != nil {
			t.Fatalf("query schema for %q error = %v", table, err)
		}
		if definition == "" {
			t.Fatalf("schema for %q is empty", table)
		}
	}
}

func TestInitDBAtPathRejectsDirectory(t *testing.T) {
	db, err := initDBAtPath(t.TempDir())
	if err == nil {
		db.Close()
		t.Fatal("initDBAtPath() with a directory path returned nil error")
	}
}

func TestCachedRowRoundTripAndReplace(t *testing.T) {
	useTestDB(t)

	tests := []struct {
		name  string
		query string
		isEN  bool
	}{
		{name: "English table", query: "abandon", isEN: true},
		{name: "Chinese table", query: "放弃", isEN: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := saveCachedRow(tt.query, tt.isEN, []byte("first")); err != nil {
				t.Fatalf("saveCachedRow(first) error = %v", err)
			}
			if err := saveCachedRow(tt.query, tt.isEN, []byte("replacement")); err != nil {
				t.Fatalf("saveCachedRow(replacement) error = %v", err)
			}

			got, err := getCachedRow(tt.query, tt.isEN)
			if err != nil {
				t.Fatalf("getCachedRow() error = %v", err)
			}
			if want := "replacement"; string(got) != want {
				t.Fatalf("getCachedRow() = %q, want %q", got, want)
			}
		})
	}

	if _, err := getCachedRow("not-present", true); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("getCachedRow(missing) error = %v, want sql.ErrNoRows", err)
	}
}

func TestCachedRowFailedWriteRollsBack(t *testing.T) {
	useTestDB(t)

	if err := saveCachedRow("invalid", true, nil); err == nil {
		t.Fatal("saveCachedRow() with nil detail returned nil error")
	}
	if _, err := getCachedRow("invalid", true); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("getCachedRow() after failed write error = %v, want sql.ErrNoRows", err)
	}

	if err := saveCachedRow("valid", true, []byte("value")); err != nil {
		t.Fatalf("saveCachedRow() after rollback error = %v", err)
	}
}

func TestDatabaseTimeRoundTrip(t *testing.T) {
	useTestDB(t)

	want := time.Date(2024, time.November, 5, 13, 14, 15, 123456789, time.FixedZone("UTC+8", 8*60*60))
	if _, err := LiteDB.Exec(
		"INSERT INTO en (query, detail, update_time) VALUES (?, ?, ?)",
		"timestamp", []byte("value"), want,
	); err != nil {
		t.Fatalf("insert timestamp error = %v", err)
	}

	var raw any
	if err := LiteDB.QueryRow("SELECT update_time FROM en WHERE query = ?", "timestamp").Scan(&raw); err != nil {
		t.Fatalf("scan timestamp error = %v", err)
	}
	got, err := parseDatabaseTime(raw)
	if err != nil {
		t.Fatalf("parseDatabaseTime(%T(%v)) error = %v", raw, raw, err)
	}
	if !got.Equal(want) || got.Format("-07:00") != want.Format("-07:00") {
		t.Fatalf("timestamp = %s (%s), want %s (%s)", got, got.Location(), want, want.Location())
	}
}

func TestParseDatabaseTimeCompatibility(t *testing.T) {
	want := time.Date(2024, time.November, 5, 13, 14, 15, 123456789, time.FixedZone("UTC+8", 8*60*60))
	for _, tt := range []struct {
		name  string
		value any
	}{
		{name: "time value", value: want},
		{name: "RFC3339 text", value: "2024-11-05T13:14:15.123456789+08:00"},
		{name: "SQLite text with offset", value: "2024-11-05 13:14:15.123456789+08:00"},
		{name: "Go time string bytes", value: []byte("2024-11-05 13:14:15.123456789 +0800 UTC+8")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDatabaseTime(tt.value)
			if err != nil {
				t.Fatalf("parseDatabaseTime(%T(%v)) error = %v", tt.value, tt.value, err)
			}
			if !got.Equal(want) {
				t.Fatalf("parseDatabaseTime(%v) = %s, want %s", tt.value, got, want)
			}
		})
	}
}

func TestDatabaseSupportsMultipleOpenHandles(t *testing.T) {
	dbPath := useTestDB(t)
	second, err := initDBAtPath(dbPath)
	if err != nil {
		t.Fatalf("second initDBAtPath() error = %v", err)
	}
	t.Cleanup(func() { second.Close() })

	if err := saveCachedRow("shared", true, []byte("visible")); err != nil {
		t.Fatalf("saveCachedRow() error = %v", err)
	}
	var got []byte
	if err := second.QueryRow("SELECT detail FROM en WHERE query = ?", "shared").Scan(&got); err != nil {
		t.Fatalf("query through second handle error = %v", err)
	}
	if string(got) != "visible" {
		t.Fatalf("second handle detail = %q, want visible", got)
	}
}

func TestDatabaseSupportsConcurrentReads(t *testing.T) {
	useTestDB(t)

	const workers = 8
	for worker := range workers {
		query := fmt.Sprintf("concurrent-%d", worker)
		value := []byte(fmt.Sprintf("value-%d", worker))
		if err := saveCachedRow(query, true, value); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}

	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			query := fmt.Sprintf("concurrent-%d", worker)
			want := []byte(fmt.Sprintf("value-%d", worker))
			got, err := getCachedRow(query, true)
			if err != nil {
				errorsByWorker <- fmt.Errorf("read %q: %w", query, err)
				return
			}
			if !reflect.DeepEqual(got, want) {
				errorsByWorker <- fmt.Errorf("read %q = %q, want %q", query, got, want)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		t.Error(err)
	}
}

func TestDatabasePersistsAcrossReopen(t *testing.T) {
	dbPath := useTestDB(t)
	if err := saveCachedRow("persistent", true, []byte("value")); err != nil {
		t.Fatalf("saveCachedRow() error = %v", err)
	}
	if err := LiteDB.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := initDBAtPath(dbPath)
	if err != nil {
		t.Fatalf("reopen database error = %v", err)
	}
	LiteDB = reopened
	t.Cleanup(func() { reopened.Close() })

	got, err := getCachedRow("persistent", true)
	if err != nil {
		t.Fatalf("getCachedRow() after reopen error = %v", err)
	}
	if want := "value"; string(got) != want {
		t.Fatalf("getCachedRow() after reopen = %q, want %q", got, want)
	}
}

func TestExistingSchemaCompatibility(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "existing.db")
	db, err := sql.Open(sqliteDriverName, dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err = db.Exec(schema); err != nil {
		t.Fatalf("create existing schema error = %v", err)
	}
	if _, err = db.Exec(
		"INSERT INTO en (query, detail, update_time) VALUES (?, ?, ?)",
		"existing", []byte("unchanged"), time.Now(),
	); err != nil {
		t.Fatalf("insert existing row error = %v", err)
	}
	if err = db.Close(); err != nil {
		t.Fatalf("close existing database error = %v", err)
	}

	db, err = initDBAtPath(dbPath)
	if err != nil {
		t.Fatalf("initDBAtPath(existing database) error = %v", err)
	}
	originalDB := LiteDB
	LiteDB = db
	t.Cleanup(func() {
		db.Close()
		LiteDB = originalDB
	})

	got, err := getCachedRow("existing", true)
	if err != nil {
		t.Fatalf("getCachedRow(existing) error = %v", err)
	}
	if want := "unchanged"; string(got) != want {
		t.Fatalf("existing row = %q, want %q", got, want)
	}
}

func TestQueryCacheRoundTrip(t *testing.T) {
	useTestDB(t)

	source := &model.Result{
		BaseResult: &model.BaseResult{Query: "abandon", IsEN: true, Found: true},
		Keyword:    "abandon",
		Pronounce:  map[string]string{"uk": "əˈbændən"},
		Paraphrase: []string{"v. 放弃"},
		Examples:   map[string][][]string{"sentences": {{"Never abandon hope.", "永远不要放弃希望。"}}},
	}
	if err := UpdateQueryCache(source); err != nil {
		t.Fatalf("UpdateQueryCache() error = %v", err)
	}

	got := &model.Result{BaseResult: &model.BaseResult{Query: source.Query, IsEN: source.IsEN}}
	if err := GetCachedQuery(got); err != nil {
		t.Fatalf("GetCachedQuery() error = %v", err)
	}

	if got.Keyword != source.Keyword ||
		!reflect.DeepEqual(got.Pronounce, source.Pronounce) ||
		!reflect.DeepEqual(got.Paraphrase, source.Paraphrase) ||
		!reflect.DeepEqual(got.Examples, source.Examples) {
		t.Fatalf("cached result = %+v, want payload from %+v", got, source)
	}
}

func TestQueryCacheSkipsNotFoundResult(t *testing.T) {
	useTestDB(t)

	r := &model.Result{BaseResult: &model.BaseResult{Query: "missing", IsEN: true, Found: false}}
	if err := UpdateQueryCache(r); err != nil {
		t.Fatalf("UpdateQueryCache() error = %v", err)
	}
	if _, err := getCachedRow(r.Query, r.IsEN); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("getCachedRow() error = %v, want sql.ErrNoRows", err)
	}
}
