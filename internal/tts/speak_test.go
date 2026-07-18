package tts

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/run"
)

var testAudio = []byte{0xff, 0xfb, 0x90, 0x64, 0x00, 0x01}

func TestBuildAudioURL(t *testing.T) {
	word := "hello 世界"
	got := buildAudioUrl(word)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", got, err)
	}
	if parsed.Scheme != "https" || parsed.Host != "translate.google.com" {
		t.Fatalf("audio URL = %q, want Google Translate HTTPS endpoint", got)
	}
	if query := parsed.Query(); query.Get("q") != word || query.Get("tl") != "en" {
		t.Fatalf("audio URL query = %#v, want q=%q and tl=en", query, word)
	}
}

func TestBuildTargetPathUsesOpaqueStableName(t *testing.T) {
	original := run.CACHE_AUDIO_DIR_PATH
	run.CACHE_AUDIO_DIR_PATH = t.TempDir()
	t.Cleanup(func() { run.CACHE_AUDIO_DIR_PATH = original })

	for _, query := range []string{"hello world", "../escape", `quote'\"<>|?*`, "中文"} {
		got := buildTargetPath(query)
		if filepath.Dir(got) != run.CACHE_AUDIO_DIR_PATH {
			t.Fatalf("buildTargetPath(%q) escaped cache directory: %q", query, got)
		}
		if strings.Contains(got, query) || filepath.Ext(got) != ".mp3" {
			t.Fatalf("buildTargetPath(%q) = %q, want opaque mp3 name", query, got)
		}
		if got != buildTargetPath(query) {
			t.Fatalf("buildTargetPath(%q) is not stable", query)
		}
	}
	if buildTargetPath("hello world") == buildTargetPath("hello_world") {
		t.Fatal("distinct queries produced the same audio cache path")
	}
}

func TestResolveCachedAudioMigratesSafeLegacyFile(t *testing.T) {
	withAudioCacheDirs(t)
	legacy := filepath.Join(run.CACHE_AUDIO_DIR_PATH, "hello_world.mp3")
	if err := os.WriteFile(legacy, testAudio, 0o600); err != nil {
		t.Fatal(err)
	}
	got, cached, err := resolveCachedAudio("hello world")
	if err != nil || !cached {
		t.Fatalf("resolveCachedAudio() = %q, %v, %v", got, cached, err)
	}
	if got != buildTargetPath("hello world") {
		t.Fatalf("resolved path = %q, want %q", got, buildTargetPath("hello world"))
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy cache still exists: %v", err)
	}
}

func TestSafeLegacyAudioName(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{name: "hello_world.mp3", want: true},
		{name: "中文.mp3", want: true},
		{name: "../escape.mp3"},
		{name: `dir\\escape.mp3`},
		{name: `bad:name.mp3`},
	} {
		if got := safeLegacyAudioName(tt.name); got != tt.want {
			t.Errorf("safeLegacyAudioName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestDownloadAudio(t *testing.T) {
	t.Run("success and request headers", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			if !strings.Contains(r.Header.Get("Accept"), "audio/mpeg") || r.Header.Get("User-Agent") != "kd" {
				t.Errorf("headers = %#v", r.Header)
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write(testAudio)
		}))
		defer server.Close()
		path := filepath.Join(t.TempDir(), "audio.mp3")
		client := &http.Client{Timeout: time.Second}
		if err := downloadAudio(context.Background(), "word", path, client, func(string) string { return server.URL }); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || !reflect.DeepEqual(got, testAudio) {
			t.Fatalf("downloaded audio = %v, %v", got, err)
		}
		if err := downloadAudio(context.Background(), "word", path, client, func(string) string { return server.URL }); err != nil {
			t.Fatal(err)
		}
		if requests.Load() != 1 {
			t.Fatalf("requests = %d, want 1 after cache hit", requests.Load())
		}
	})

	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "bad status", handler: func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "failed", http.StatusBadGateway) }},
		{name: "empty", handler: func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("Content-Length", "0") }},
		{name: "html", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>error</html>"))
		}},
		{name: "oversize", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "10485761")
			w.WriteHeader(http.StatusOK)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()
			dir := t.TempDir()
			path := filepath.Join(dir, "audio.mp3")
			err := downloadAudio(context.Background(), "word", path, server.Client(), func(string) string { return server.URL })
			if err == nil {
				t.Fatal("downloadAudio() error = nil")
			}
			assertNoAudioArtifacts(t, dir)
		})
	}
}

func TestDownloadAudioRecoversInvalidCacheAndCancellation(t *testing.T) {
	t.Run("invalid cache", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "audio.mp3")
		if err := os.WriteFile(path, []byte("<html>stale error</html>"), 0o600); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write(testAudio)
		}))
		defer server.Close()
		if err := downloadAudio(context.Background(), "word", path, server.Client(), func(string) string { return server.URL }); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); !reflect.DeepEqual(got, testAudio) {
			t.Fatalf("recovered cache = %v", got)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		dir := t.TempDir()
		err := downloadAudio(ctx, "word", filepath.Join(dir, "audio.mp3"), server.Client(), func(string) string { return server.URL })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("downloadAudio() error = %v, want context.Canceled", err)
		}
		assertNoAudioArtifacts(t, dir)
	})
}

func TestConcurrentAudioDownloadPublishesValidFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(testAudio)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "audio.mp3")
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- downloadAudio(context.Background(), "word", path, server.Client(), func(string) string { return server.URL })
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("downloadAudio() error = %v", err)
		}
	}
	if valid, err := validAudioFile(path); err != nil || !valid {
		t.Fatalf("published cache valid = %v, error = %v", valid, err)
	}
}

func TestFindSpeakerAndPlayerCommand(t *testing.T) {
	available := map[string]bool{"mpg123": true, "mpv": true}
	lookPath := func(name string) (string, error) {
		if available[name] {
			return name, nil
		}
		return "", os.ErrNotExist
	}
	if got := findSpeaker("linux", lookPath); got != "mpg123" {
		t.Fatalf("findSpeaker(linux) = %q, want mpg123", got)
	}
	if got := findSpeaker("darwin", lookPath); got != "mpv" {
		t.Fatalf("findSpeaker(darwin) = %q, want mpv", got)
	}
	if got := findSpeaker("windows", func(string) (string, error) { return "", os.ErrNotExist }); got != systemPlayer {
		t.Fatalf("findSpeaker(windows) = %q, want system", got)
	}
	if got := findSpeaker("plan9", lookPath); got != "" {
		t.Fatalf("findSpeaker(plan9) = %q, want empty", got)
	}

	path := `C:\audio\quote'file.mp3`
	program, args, err := playerCommand("windows", systemPlayer, path)
	if err != nil || program != "explorer.exe" || !reflect.DeepEqual(args, []string{path}) {
		t.Fatalf("playerCommand() = %q, %#v, %v", program, args, err)
	}
	for _, tt := range []struct {
		player string
		want   []string
	}{
		{player: "mpv", want: []string{"--no-terminal", "--really-quiet", "audio.mp3"}},
		{player: "ffplay", want: []string{"-nodisp", "-autoexit", "-loglevel", "quiet", "audio.mp3"}},
		{player: "mpg123", want: []string{"-q", "audio.mp3"}},
		{player: "afplay", want: []string{"audio.mp3"}},
	} {
		_, args, err := playerCommand("linux", tt.player, "audio.mp3")
		if err != nil || !reflect.DeepEqual(args, tt.want) {
			t.Errorf("playerCommand(%q) args = %#v, %v, want %#v", tt.player, args, err, tt.want)
		}
	}
}

func TestLimitedBuffer(t *testing.T) {
	var buffer limitedBuffer
	input := strings.Repeat("x", playerOutputLimit+100)
	if n, err := buffer.Write([]byte(input)); err != nil || n != len(input) {
		t.Fatalf("Write() = %d, %v", n, err)
	}
	if len(buffer.String()) != playerOutputLimit {
		t.Fatalf("buffer length = %d, want %d", len(buffer.String()), playerOutputLimit)
	}
}

func withAudioCacheDirs(t *testing.T) {
	t.Helper()
	originalAudio := run.CACHE_AUDIO_DIR_PATH
	originalRun := run.CACHE_RUN_PATH
	root := t.TempDir()
	run.CACHE_AUDIO_DIR_PATH = filepath.Join(root, "audio")
	run.CACHE_RUN_PATH = filepath.Join(root, "run")
	if err := os.MkdirAll(run.CACHE_AUDIO_DIR_PATH, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		run.CACHE_AUDIO_DIR_PATH = originalAudio
		run.CACHE_RUN_PATH = originalRun
	})
}

func assertNoAudioArtifacts(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("download left artifacts: %v", entries)
	}
}
