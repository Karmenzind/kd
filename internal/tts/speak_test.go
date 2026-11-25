package tts

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/run"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

var testAudio = []byte{0xff, 0xfb, 0x90, 0x64, 0x00, 0x01}

func TestBuildAudioURL(t *testing.T) {
	word := "hello 世界"
	got := buildAudioUrl(word)
	want := fmt.Sprintf(googleTTSURLFormat, url.QueryEscape(word))
	if got != want {
		t.Fatalf("buildAudioUrl() = %q, want %q", got, want)
	}
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

func TestValidAudioFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.mp3")
	if valid, err := validAudioFile(missing); err != nil || valid {
		t.Fatalf("missing file valid = %v, err = %v", valid, err)
	}

	validPath := filepath.Join(dir, "valid.mp3")
	if err := os.WriteFile(validPath, testAudio, 0o600); err != nil {
		t.Fatal(err)
	}
	if valid, err := validAudioFile(validPath); err != nil || !valid {
		t.Fatalf("valid audio valid = %v, err = %v", valid, err)
	}

	emptyPath := filepath.Join(dir, "empty.mp3")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if valid, err := validAudioFile(emptyPath); err != nil || valid {
		t.Fatalf("empty file valid = %v, err = %v", valid, err)
	}

	htmlPath := filepath.Join(dir, "html.mp3")
	if err := os.WriteFile(htmlPath, []byte("<html>blocked</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if valid, err := validAudioFile(htmlPath); err != nil || valid {
		t.Fatalf("html payload valid = %v, err = %v", valid, err)
	}
	if _, err := os.Stat(htmlPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid html cache was not removed: %v", err)
	}
}

func TestResolveCachedAudioUncachedAndUnsafeLegacy(t *testing.T) {
	withAudioCacheDirs(t)
	target, cached, err := resolveCachedAudio("fresh-query")
	if err != nil || cached || target != buildTargetPath("fresh-query") {
		t.Fatalf("uncached resolve = %q, %v, %v", target, cached, err)
	}

	unsafeLegacy := filepath.Join(run.CACHE_AUDIO_DIR_PATH, "../escape.mp3")
	if err := os.WriteFile(unsafeLegacy, testAudio, 0o600); err != nil {
		t.Fatal(err)
	}
	target, cached, err = resolveCachedAudio("../escape")
	if err != nil || cached || target != buildTargetPath("../escape") {
		t.Fatalf("unsafe legacy resolve = %q, %v, %v", target, cached, err)
	}
}

func TestPlayAudioUsesFakePlayer(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t)))
	audioPath := filepath.Join(t.TempDir(), "audio.mp3")
	if err := os.WriteFile(audioPath, testAudio, 0o600); err != nil {
		t.Fatal(err)
	}
	marker := installFakePlayer(t, 0)
	ctx := context.Background()
	if err := playAudio(ctx, runtime.GOOS, fakePlayerName(), audioPath); err != nil {
		t.Fatalf("playAudio() error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), audioPath) {
		t.Fatalf("fake player output = %q, want path %q", got, audioPath)
	}
}

func TestPlayAudioFailureAndCancellation(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t)))
	binDir := t.TempDir()
	name := fakePlayerName()
	installFakeProgram(t, binDir, name, fakePlayerExit(1))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	audioPath := filepath.Join(t.TempDir(), "audio.mp3")
	if err := os.WriteFile(audioPath, testAudio, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := playAudio(context.Background(), runtime.GOOS, name, audioPath); err == nil {
		t.Fatal("playAudio() error = nil, want failure")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := playAudio(ctx, runtime.GOOS, name, audioPath); err == nil {
		t.Fatal("playAudio() error = nil, want cancellation")
	}
}

func TestSpeak(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t)))

	t.Run("validation", func(t *testing.T) {
		if err := Speak(context.Background(), "  ", 1); err == nil {
			t.Fatal("Speak() empty query error = nil")
		}
		limitMB := uint64(math.MaxInt64)/(1<<20) + 1
		if _, err := cacheLimitBytes(limitMB); err == nil {
			t.Fatal("cacheLimitBytes() oversized error = nil")
		}
	})

	t.Run("no player", func(t *testing.T) {
		withAudioCacheDirs(t)
		t.Setenv("PATH", t.TempDir())
		err := Speak(context.Background(), "word", 1)
		if runtime.GOOS == "windows" {
			if err == nil {
				t.Fatal("Speak() on Windows without players error = nil")
			}
		} else if !errors.Is(err, ErrNoSpeaker) {
			t.Fatalf("Speak() error = %v, want ErrNoSpeaker", err)
		}
	})

	t.Run("cached audio", func(t *testing.T) {
		withAudioCacheDirs(t)
		installFakePlayer(t, 0)
		word := "cached-word"
		target := buildTargetPath(word)
		if err := os.WriteFile(target, testAudio, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Speak(context.Background(), word, 64); err != nil {
			t.Fatalf("Speak() cached error = %v", err)
		}
	})

	t.Run("download", func(t *testing.T) {
		withAudioCacheDirs(t)
		installFakePlayer(t, 0)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write(testAudio)
		}))
		defer server.Close()
		oldFormat := googleTTSURLFormat
		googleTTSURLFormat = server.URL + "?q=%s"
		t.Cleanup(func() { googleTTSURLFormat = oldFormat })

		word := "download-word"
		if err := Speak(context.Background(), word, 64); err != nil {
			t.Fatalf("Speak() download error = %v", err)
		}
		if valid, err := validAudioFile(buildTargetPath(word)); err != nil || !valid {
			t.Fatalf("cached download valid = %v, err = %v", valid, err)
		}
	})

	t.Run("uncached limit", func(t *testing.T) {
		withAudioCacheDirs(t)
		installFakePlayer(t, 0)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write(testAudio)
		}))
		defer server.Close()
		oldFormat := googleTTSURLFormat
		googleTTSURLFormat = server.URL + "?q=%s"
		t.Cleanup(func() { googleTTSURLFormat = oldFormat })

		word := "ephemeral-word"
		if err := Speak(context.Background(), word, 0); err != nil {
			t.Fatalf("Speak() uncached error = %v", err)
		}
		if _, err := os.Stat(buildTargetPath(word)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("uncached Speak left persistent cache file: %v", err)
		}
	})
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

func fakePlayerName() string {
	if runtime.GOOS == "darwin" {
		return "afplay"
	}
	return "ffplay"
}

func installFakePlayer(t *testing.T, exitCode int) string {
	t.Helper()
	binDir := t.TempDir()
	name := fakePlayerName()
	marker := filepath.Join(t.TempDir(), "played.txt")
	body := fakePlayerRecord(marker, exitCode)
	installFakeProgram(t, binDir, name, body)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
}

func installFakeProgram(t *testing.T, binDir, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(binDir, name+".bat")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	path := filepath.Join(binDir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func fakePlayerRecord(marker string, exitCode int) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("@echo off\r\nfor %%%%A in (%%*) do set \"LAST=%%%%A\"\r\necho %%LAST%%> %q\r\nexit /b %d\r\n", marker, exitCode)
	}
	return fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nexit %d\n", marker, exitCode)
}

func fakePlayerExit(code int) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("@echo off\r\nexit /b %d\r\n", code)
	}
	return fmt.Sprintf("#!/bin/sh\nexit %d\n", code)
}
