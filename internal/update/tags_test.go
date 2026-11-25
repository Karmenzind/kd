package update

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setTagTestPaths(t *testing.T, url, cachePath string) {
	t.Helper()
	oldURL, oldFile := TAGLIST_URL, LATEST_TAG_FILE
	TAGLIST_URL, LATEST_TAG_FILE = url, cachePath
	t.Cleanup(func() {
		TAGLIST_URL, LATEST_TAG_FILE = oldURL, oldFile
	})
}

func TestGetLatestTag(t *testing.T) {
	for _, tt := range []struct {
		name    string
		body    string
		status  int
		want    string
		wantErr bool
	}{
		{name: "first tag is cached", body: `[{"name":"v2.3.4"},{"name":"v9.0.0"}]`, want: "v2.3.4"},
		{name: "empty list", body: `[]`, wantErr: true},
		{name: "null list", body: `null`, wantErr: true},
		{name: "invalid JSON", body: `[{"name":`, wantErr: true},
		{name: "wrong tag type", body: `[{"name":42}]`, wantErr: true},
		{name: "API error object", status: http.StatusForbidden, body: `{"message":"rate limited"}`, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/tags" {
					t.Errorf("request = %s %s, want GET /tags", r.Method, r.URL.Path)
				}
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			cachePath := filepath.Join(t.TempDir(), "latest_tag")
			writeTestFile(t, cachePath, "previous tag")
			setTagTestPaths(t, server.URL+"/tags", cachePath)

			tag, err := GetLatestTag()
			if (err != nil) != tt.wantErr || tag != tt.want {
				t.Fatalf("GetLatestTag() = %q, %v; want %q, error=%v", tag, err, tt.want, tt.wantErr)
			}
			wantCache := tt.want
			if tt.wantErr {
				wantCache = "previous tag"
			}
			assertTestFile(t, cachePath, wantCache)
		})
	}
}

func TestGetLatestTagRequestFailures(t *testing.T) {
	closedServer := httptest.NewServer(http.NotFoundHandler())
	closedURL := closedServer.URL
	closedServer.Close()
	truncatedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, `[{"name":"v2.0.0"}]`)
	}))
	defer truncatedServer.Close()

	for _, tt := range []struct {
		name string
		url  string
	}{
		{name: "invalid request URL", url: "://invalid"},
		{name: "connection refused", url: closedURL},
		{name: "truncated response", url: truncatedServer.URL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cachePath := filepath.Join(t.TempDir(), "latest_tag")
			writeTestFile(t, cachePath, "previous tag")
			setTagTestPaths(t, tt.url, cachePath)
			if tag, err := GetLatestTag(); err == nil || tag != "" {
				t.Fatalf("GetLatestTag() = %q, %v; want empty tag and error", tag, err)
			}
			assertTestFile(t, cachePath, "previous tag")
		})
	}
}

func TestGetLatestTagCacheWriteFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"v2.0.0"}]`)
	}))
	defer server.Close()
	cachePath := filepath.Join(t.TempDir(), "missing", "latest_tag")
	setTagTestPaths(t, server.URL, cachePath)
	if tag, err := GetLatestTag(); err != nil || tag != "v2.0.0" {
		t.Fatalf("GetLatestTag() = %q, %v; cache failure must not discard the online tag", tag, err)
	}
	assertTestMissing(t, cachePath)
}

func TestGetCachedLatestTag(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "latest_tag")
	setTagTestPaths(t, "unused", cachePath)
	if got := GetCachedLatestTag(); got != "" {
		t.Fatalf("missing cache = %q, want empty", got)
	}
	writeTestFile(t, cachePath, "v2.3.4\n")
	if got := GetCachedLatestTag(); got != "v2.3.4\n" {
		t.Fatalf("cached tag = %q, want verbatim file contents", got)
	}
	LATEST_TAG_FILE = root // A directory exists but cannot be read as a tag file.
	if got := GetCachedLatestTag(); got != "" {
		t.Fatalf("unreadable cache = %q, want empty", got)
	}
}

func TestGetNewerVersion(t *testing.T) {
	for _, tt := range []struct {
		name    string
		current string
		body    string
		want    string
		wantErr bool
	}{
		{name: "newer", current: "v1.9.9", body: `[{"name":"v2.0.0"}]`, want: "v2.0.0"},
		{name: "equal", current: "2.0.0", body: `[{"name":"v2.0.0"}]`},
		{name: "older", current: "v3.0.0", body: `[{"name":"v2.0.0"}]`},
		{name: "invalid current", current: "development", body: `[{"name":"v2.0.0"}]`},
		{name: "invalid latest", current: "v1.0.0", body: `[{"name":"latest"}]`},
		{name: "fetch failure", current: "v1.0.0", body: `[]`, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			setTagTestPaths(t, server.URL, filepath.Join(t.TempDir(), "latest_tag"))
			if got, err := GetNewerVersion(tt.current); got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("GetNewerVersion(%q) = %q, %v; want %q, error=%v", tt.current, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertTestFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("ReadFile(%q) = %q, %v; want %q", path, got, err, want)
	}
}

func assertTestMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%q) = %v, want not exist", path, err)
	}
}
