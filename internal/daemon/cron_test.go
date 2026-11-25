package daemon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/internal/update"
	"github.com/Karmenzind/kd/pkg"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// These tests change package globals and must remain serial. Join cron workers
// before restoring globals or deleting their temporary files.
func replaceForTest[T any](t *testing.T, target *T, value T) {
	t.Helper()
	old := *target
	*target = value
	t.Cleanup(func() { *target = old })
}

func useCronCache(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, target := range []*string{&cache.CACHE_ROOT_PATH, &run.CACHE_ROOT_PATH} {
		replaceForTest(t, target, root)
	}
	for _, target := range []*string{&cache.CACHE_RUN_PATH, &run.CACHE_RUN_PATH} {
		replaceForTest(t, target, filepath.Join(root, "run"))
	}
	for _, target := range []*string{&cache.CACHE_WORDS_PATH, &run.CACHE_WORDS_PATH} {
		replaceForTest(t, target, filepath.Join(root, "words"))
	}
	for _, target := range []*string{&cache.CACHE_STAT_DIR_PATH, &run.CACHE_STAT_DIR_PATH} {
		replaceForTest(t, target, filepath.Join(root, "stat"))
	}
	replaceForTest(t, &cache.LONG_TEXT_CACHE_FILE, filepath.Join(root, "long_text_results.json"))
	replaceForTest(t, &update.LATEST_TAG_FILE, filepath.Join(root, "latest_tag"))
	for _, path := range []string{cache.CACHE_RUN_PATH, cache.CACHE_WORDS_PATH, cache.CACHE_STAT_DIR_PATH} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	oldDB := cache.LiteDB
	if err := cache.InitDB(); err != nil {
		t.Fatal(err)
	}
	db := cache.LiteDB
	t.Cleanup(func() {
		db.Close() // applyTempDB also closes it on Windows; Close is idempotent.
		cache.LiteDB = oldDB
	})
	if _, err := db.Exec("INSERT INTO meta VALUES ('test_marker', 'original', CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, cache.DB_FILENAME)
}

func cronLogHook(t *testing.T, hook func(string)) {
	t.Helper()
	core, _ := observer.New(zap.DebugLevel)
	restore := zap.ReplaceGlobals(zap.New(core, zap.Hooks(func(entry zapcore.Entry) error {
		hook(entry.Message)
		return nil
	})))
	t.Cleanup(restore)
}

func launchCron(t *testing.T, ctx context.Context, cancel context.CancelFunc, cron func(context.Context)) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		cron(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		awaitCron(t, done)
	})
	return done
}

func awaitCron(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second): // Windows applyTempDB has a fixed 3s delay.
		t.Fatal("cron worker did not stop")
	}
}

type cronRoundTripper func(*http.Request) (*http.Response, error)

func (f cronRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Route every allowed remote URL to a loopback server; never fall back to the
// real transport for an unexpected URL.
func routeDataRequests(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	localURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: nil}
	t.Cleanup(transport.CloseIdleConnections)
	replaceForTest(t, &http.DefaultTransport, http.RoundTripper(cronRoundTripper(func(req *http.Request) (*http.Response, error) {
		path := ""
		switch req.URL.String() {
		case "https://ipinfo.io/json":
			path = "/ip"
		case DATA_ZIP_URL_CN:
			path = "/cn.zip"
		case DATA_ZIP_URL_GLOBAL:
			path = "/global.zip"
		default:
			return nil, fmt.Errorf("unexpected HTTP request: %s", req.URL)
		}
		clone := req.Clone(req.Context())
		clone.URL = &url.URL{Scheme: localURL.Scheme, Host: localURL.Host, Path: path}
		clone.Host = localURL.Host
		return transport.RoundTrip(clone)
	})))
}

func TestCronUpdateDataZip(t *testing.T) {
	for _, outcome := range []string{"success", "retry download", "download failure", "invalid zip", "empty zip", "directory only zip"} {
		t.Run(outcome, func(t *testing.T) {
			dbPath := useCronCache(t)
			original, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			zipPath := filepath.Join(t.TempDir(), "download.zip")
			var replacement []byte
			switch outcome {
			case "success", "retry download":
				newPath := filepath.Join(t.TempDir(), "new.db")
				db, err := sql.Open("sqlite", newPath)
				if err != nil {
					t.Fatal(err)
				}
				_, execErr := db.Exec("CREATE TABLE replacement (value TEXT); INSERT INTO replacement VALUES ('downloaded')")
				closeErr := db.Close()
				if err := errors.Join(execErr, closeErr); err != nil {
					t.Fatal(err)
				}
				replacement, err = os.ReadFile(newPath)
				if err != nil {
					t.Fatal(err)
				}
				writeTestZIP(t, zipPath, []zipEntry{{name: "data", dir: true}, {name: "data/db", body: string(replacement)}})
			case "empty zip":
				writeTestZIP(t, zipPath, nil)
			case "directory only zip":
				writeTestZIP(t, zipPath, []zipEntry{{name: "data", dir: true}})
			default:
				if err := os.WriteFile(zipPath, []byte("invalid zip"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var downloads atomic.Int32
			routeDataRequests(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/ip" {
					fmt.Fprint(w, `{"country":"CN"}`)
					return
				}
				attempt := downloads.Add(1)
				if outcome == "download failure" || (outcome == "retry download" && attempt == 1) {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				http.ServeFile(w, r, zipPath)
			})
			replaceForTest(t, &dataZipCheckInterval, 10*time.Millisecond)
			ctx, cancel := context.WithCancel(context.Background())
			var failures atomic.Int32
			cronLogHook(t, func(message string) {
				if strings.HasPrefix(message, "Failed to download zip file:") || strings.HasPrefix(message, "Failed:") {
					failures.Add(1)
					if outcome != "retry download" {
						cancel()
					}
				}
			})
			var shutdowns atomic.Int32
			done := launchCron(t, ctx, cancel, func(ctx context.Context) {
				cronUpdateDataZip(ctx, func() { shutdowns.Add(1) })
			})
			awaitCron(t, done)
			if downloads.Load() == 0 {
				t.Fatal("no data download attempted")
			}
			body, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(cache.CACHE_RUN_PATH, "last_fetch_db")
			backups, err := filepath.Glob(dbPath + ".backup.*")
			if err != nil {
				t.Fatal(err)
			}
			if outcome == "success" || outcome == "retry download" {
				if outcome == "retry download" {
					if downloads.Load() != 2 || failures.Load() == 0 {
						t.Fatalf("retry did not recover: downloads=%d failures=%d", downloads.Load(), failures.Load())
					}
				} else if failures.Load() != 0 {
					t.Fatalf("successful download reported %d failures", failures.Load())
				}
				if string(body) != string(replacement) || shutdowns.Load() != 1 {
					t.Fatalf("replacement differs or wrong callbacks: shutdowns=%d failures=%d", shutdowns.Load(), failures.Load())
				}
				if len(backups) != 1 {
					t.Fatalf("backups = %v, want one", backups)
				}
				backup, err := os.ReadFile(backups[0])
				if err != nil || string(backup) != string(original) {
					t.Fatalf("original database was not preserved in backup: %v", err)
				}
				timestamp, err := os.ReadFile(marker)
				if err != nil {
					t.Fatal(err)
				}
				if n, err := strconv.ParseInt(string(timestamp), 10, 64); err != nil || n <= 0 {
					t.Fatalf("invalid update marker %q: %v", timestamp, err)
				}
				if _, err := os.Stat(dbPath + ".temp"); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("temporary database remains: %v", err)
				}
			} else {
				if string(body) != string(original) || len(backups) != 0 || shutdowns.Load() != 0 || failures.Load() == 0 {
					t.Fatalf("failure changed database or stopped daemon: backups=%v shutdowns=%d failures=%d", backups, shutdowns.Load(), failures.Load())
				}
				var value string
				if err := cache.LiteDB.QueryRow("SELECT value FROM meta WHERE key='test_marker'").Scan(&value); err != nil || value != "original" {
					t.Fatalf("original database no longer usable: value=%q err=%v", value, err)
				}
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed update recorded as completed: %v", err)
				}
				if outcome != "download failure" {
					if _, err := os.Stat(filepath.Join(cache.CACHE_ROOT_PATH, "kd_data.zip")); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("invalid archive was not removed: %v", err)
					}
				}
			}
		})
	}
}

func TestCronUpdateSkipsRecordedDownload(t *testing.T) {
	dbPath := useCronCache(t)
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(cache.CACHE_RUN_PATH, "last_fetch_db")
	if err := os.WriteFile(marker, []byte("already fetched"), 0o600); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	replaceForTest(t, &http.DefaultTransport, http.RoundTripper(cronRoundTripper(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("network forbidden")
	})))
	replaceForTest(t, &dataZipCheckInterval, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	var shutdowns atomic.Int32
	done := launchCron(t, ctx, cancel, func(ctx context.Context) { cronUpdateDataZip(ctx, func() { shutdowns.Add(1) }) })
	awaitCron(t, done)
	after, err := os.ReadFile(dbPath)
	if err != nil || string(after) != string(before) || requests.Load() != 0 || shutdowns.Load() != 0 {
		t.Fatalf("recorded update had side effects: requests=%d shutdowns=%d err=%v", requests.Load(), shutdowns.Load(), err)
	}
}

func useDaemonPort(t *testing.T, addr string) {
	t.Helper()
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	replaceForTest(t, &run.SERVER_PORT, port)
}

func TestCronEnsureDaemonJSON(t *testing.T) {
	for _, scenario := range []string{"missing", "corrupt", "foreign owner", "other PID"} {
		t.Run(scenario, func(t *testing.T) {
			useDaemonInfoDir(t)
			owner := &model.RunInfo{PID: 321, Port: "1", Instance: "owner", StartTime: 1}
			replaceForTest(t, &run.Info, owner)
			ctx, cancel := context.WithCancel(context.Background())
			cronLogHook(t, func(message string) {
				if strings.Contains(message, "belongs to another instance") {
					cancel()
				}
			})
			var pings atomic.Int32
			addr, stop := startProtocolStub(t, func(conn net.Conn) {
				pings.Add(1)
				if scenario == "other PID" {
					writePing(conn, model.DaemonProtocolVersion)
					cancel()
					return
				}
				writePing(conn, model.DaemonProtocolVersion)
			})
			t.Cleanup(stop)
			useDaemonPort(t, addr)
			if scenario == "other PID" {
				owner.PID = 999
			}
			var original []byte
			switch scenario {
			case "corrupt":
				original = []byte("broken JSON")
			case "foreign owner":
				if err := WriteDaemonInfo(GetDaemonInfoPath(), &model.RunInfo{PID: 321, Port: "1", Instance: "foreign"}); err != nil {
					t.Fatal(err)
				}
				var err error
				original, err = os.ReadFile(GetDaemonInfoPath())
				if err != nil {
					t.Fatal(err)
				}
			}
			if original != nil {
				if err := os.WriteFile(GetDaemonInfoPath(), original, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			replaceForTest(t, &daemonJSONCheckInterval, 10*time.Millisecond)
			done := launchCron(t, ctx, cancel, cronEnsureDaemonJsonFile)
			if scenario == "missing" || scenario == "corrupt" {
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				timeout := time.NewTimer(time.Second)
				defer timeout.Stop()
			waiting:
				for {
					select {
					case <-ticker.C:
						got, err := ReadDaemonInfo(GetDaemonInfoPath())
						if err == nil && got.Instance == owner.Instance {
							cancel()
							break waiting
						}
					case <-timeout.C:
						t.Fatal("runtime file was not repaired")
					}
				}
			}
			awaitCron(t, done)
			switch scenario {
			case "foreign owner":
				body, err := os.ReadFile(GetDaemonInfoPath())
				if err != nil || string(body) != string(original) || pings.Load() != 0 {
					t.Fatalf("foreign runtime changed or pinged: pings=%d err=%v", pings.Load(), err)
				}
			case "other PID":
				if _, err := os.Stat(GetDaemonInfoPath()); !errors.Is(err, os.ErrNotExist) || pings.Load() == 0 {
					t.Fatalf("runtime written for a different daemon: pings=%d err=%v", pings.Load(), err)
				}
			default:
				got, err := ReadDaemonInfo(GetDaemonInfoPath())
				if err != nil || got.Instance != owner.Instance || got.PID != owner.PID || pings.Load() == 0 {
					t.Fatalf("runtime not repaired: info=%+v pings=%d err=%v", got, pings.Load(), err)
				}
			}
		})
	}
}

func TestCronDeleteSpam(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "kd")
	for _, path := range []string{exe, exe + ".update_backup"} {
		if err := os.WriteFile(path, []byte("test executable"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	replaceForTest(t, &pkg.EXECUTABLE_PATH, exe)
	replaceForTest(t, &spamCleanupInterval, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cronLogHook(t, func(message string) {
		if message == "Started clearing spam" {
			cancel()
		}
	})
	done := launchCron(t, ctx, cancel, cronDeleteSpam)
	awaitCron(t, done)
	if _, err := os.Stat(exe + ".update_backup"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup was not removed: %v", err)
	}
	if body, err := os.ReadFile(exe); err != nil || string(body) != "test executable" {
		t.Fatalf("executable changed: %q, %v", body, err)
	}
}

func TestCronSanitizeRetriesAndStopsAfterSuccess(t *testing.T) {
	useCronCache(t)
	if _, err := cache.LiteDB.Exec("DROP TABLE meta; INSERT INTO en VALUES ('aren''t', 'old', CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	replaceForTest(t, &sanitizeStartDelay, time.Millisecond)
	replaceForTest(t, &sanitizeRetryInterval, 10*time.Millisecond)
	failed := make(chan struct{})
	var once sync.Once
	cronLogHook(t, func(message string) {
		if strings.HasPrefix(message, "Failed to sanitize cached rows") {
			once.Do(func() { close(failed) })
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := launchCron(t, ctx, cancel, cronSanitizeCache)
	select {
	case <-failed:
	case <-time.After(time.Second):
		t.Fatal("sanitize did not report missing meta table")
	}
	if _, err := cache.LiteDB.Exec("CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT, update_time DATETIME)"); err != nil {
		t.Fatal(err)
	}
	awaitCron(t, done) // Successful sanitize returns without cancellation.
	var version, count int
	if err := cache.LiteDB.QueryRow("SELECT value FROM meta WHERE key='sanitize_version'").Scan(&version); err != nil || version <= 0 {
		t.Fatalf("sanitize version not recorded: version=%d err=%v", version, err)
	}
	if err := cache.LiteDB.QueryRow("SELECT COUNT(*) FROM en").Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy row not purged: count=%d err=%v", count, err)
	}
}

func TestCronsCancelBeforeFirstWork(t *testing.T) {
	useCronCache(t)
	// In particular, GetLatestTag uses its own transport and cannot be routed
	// with http.DefaultTransport. Keep its first tick far beyond this test.
	replaceForTest(t, &dataZipCheckInterval, time.Hour)
	replaceForTest(t, &daemonJSONCheckInterval, time.Hour)
	replaceForTest(t, &spamCleanupInterval, time.Hour)
	replaceForTest(t, &updateCheckInterval, time.Hour)
	replaceForTest(t, &sanitizeStartDelay, time.Hour)
	for name, cron := range map[string]func(context.Context){
		"data zip": func(ctx context.Context) { cronUpdateDataZip(ctx, func() { t.Error("unexpected shutdown") }) },
		"runtime":  cronEnsureDaemonJsonFile,
		"spam":     cronDeleteSpam,
		"update":   cronCheckUpdate,
		"sanitize": cronSanitizeCache,
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			done := launchCron(t, ctx, cancel, cron)
			cancel()
			awaitCron(t, done)
		})
	}
}
