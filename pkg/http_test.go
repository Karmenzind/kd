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
