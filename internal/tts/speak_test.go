package tts

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Karmenzind/kd/internal/run"
)

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

func TestBuildTargetPath(t *testing.T) {
	original := run.CACHE_AUDIO_DIR_PATH
	run.CACHE_AUDIO_DIR_PATH = t.TempDir()
	t.Cleanup(func() { run.CACHE_AUDIO_DIR_PATH = original })

	got := buildTargetPath("hello world")
	want := filepath.Join(run.CACHE_AUDIO_DIR_PATH, "hello_world.mp3")
	if got != want {
		t.Fatalf("buildTargetPath() = %q, want %q", got, want)
	}
}

func TestDownloadAudioUsesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cached.mp3")
	if err := os.WriteFile(path, []byte("cached"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := downloadAudio("must not contact network", path); err != nil {
		t.Fatalf("downloadAudio(existing) error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.TrimSpace(string(body)) != "cached" {
		t.Fatalf("cached audio = %q, want unchanged", body)
	}
}
