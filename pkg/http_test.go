package pkg

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadFileWithTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_, _ = w.Write([]byte("binary contents"))
	}))
	t.Cleanup(server.Close)

	path := filepath.Join(t.TempDir(), "download")
	if err := DownloadFileWithTimeout(path, server.URL, time.Second); err != nil {
		t.Fatalf("DownloadFileWithTimeout() error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "binary contents" {
		t.Fatalf("downloaded body = %q", body)
	}
}

func TestDownloadFileWithTimeoutErrors(t *testing.T) {
	t.Run("bad status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "failed", http.StatusServiceUnavailable)
		}))
		t.Cleanup(server.Close)

		err := DownloadFileWithTimeout(filepath.Join(t.TempDir(), "download"), server.URL, time.Second)
		if err == nil || !strings.Contains(err.Error(), "503 Service Unavailable") {
			t.Fatalf("DownloadFileWithTimeout() error = %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		t.Cleanup(server.Close)

		err := DownloadFileWithTimeout(filepath.Join(t.TempDir(), "download"), server.URL, 20*time.Millisecond)
		if err == nil {
			t.Fatal("DownloadFileWithTimeout(timeout) returned nil error")
		}
	})
}

func TestGetRandomUA(t *testing.T) {
	seen := make(map[string]struct{})
	for range 20 {
		ua := GetRandomUA()
		if ua == "" {
			t.Fatal("GetRandomUA() returned empty string")
		}
		if !strings.HasPrefix(ua, "Mozilla/5.0 ") {
			t.Fatalf("GetRandomUA() = %q, want Mozilla prefix", ua)
		}
		seen[ua] = struct{}{}
	}
	if len(seen) == 0 {
		t.Fatal("GetRandomUA() never returned a value")
	}
}

func TestDownloadFileDelegatesTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	path := filepath.Join(t.TempDir(), "out")
	if err := DownloadFile(path, server.URL); err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}

func TestDownloadFileWithProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "5")
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(server.Close)

	path := filepath.Join(t.TempDir(), "progress")
	if err := DownloadFileWithProgress(path, server.URL); err != nil {
		t.Fatalf("DownloadFileWithProgress() error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
}

func TestDownloadFileWithProgressErrors(t *testing.T) {
	t.Run("bad status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusTeapot)
		}))
		t.Cleanup(server.Close)
		err := DownloadFileWithProgress(filepath.Join(t.TempDir(), "x"), server.URL)
		if err == nil || !strings.Contains(err.Error(), "418") {
			t.Fatalf("DownloadFileWithProgress() error = %v", err)
		}
	})

	t.Run("unknown content length", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.(http.Flusher).Flush()
			_, _ = w.Write([]byte("hello"))
		}))
		t.Cleanup(server.Close)
		err := DownloadFileWithProgress(filepath.Join(t.TempDir(), "x"), server.URL)
		if err == nil || !strings.Contains(err.Error(), "content length is unknown") {
			t.Fatalf("DownloadFileWithProgress() error = %v", err)
		}
	})
}
