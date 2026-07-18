package tts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAudioCleanupDue(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "last-cleanup")
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	if due, err := audioCleanupDue(marker, 100, now); err != nil || !due {
		t.Fatalf("missing marker due = %v, error = %v", due, err)
	}
	if err := writeCleanupMarker(marker, 100, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if due, err := audioCleanupDue(marker, 100, now); err != nil || due {
		t.Fatalf("fresh marker due = %v, error = %v", due, err)
	}
	if due, err := audioCleanupDue(marker, 200, now); err != nil || !due {
		t.Fatalf("changed limit due = %v, error = %v", due, err)
	}
	if err := os.Chtimes(marker, now.Add(-25*time.Hour), now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if due, err := audioCleanupDue(marker, 100, now); err != nil || !due {
		t.Fatalf("old marker due = %v, error = %v", due, err)
	}
	if due, err := audioCleanupDue(marker, 0, now); err != nil || !due {
		t.Fatalf("zero limit due = %v, error = %v", due, err)
	}
}

func TestTrimAudioCacheRemovesOldestFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	oldest := writeSizedAudio(t, dir, "oldest.mp3", 5, now.Add(-3*time.Hour))
	middle := writeSizedAudio(t, dir, "middle.mp3", 5, now.Add(-2*time.Hour))
	newest := writeSizedAudio(t, dir, "newest.mp3", 5, now.Add(-time.Hour))
	if err := os.WriteFile(filepath.Join(dir, "unrelated.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := trimAudioCache(dir, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || result.Freed != 5 {
		t.Fatalf("cleanup result = %+v", result)
	}
	if _, err := os.Stat(oldest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oldest file remains: %v", err)
	}
	for _, path := range []string{middle, newest, filepath.Join(dir, "unrelated.txt")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %q: %v", path, err)
		}
	}
}

func TestTrimAudioCacheZeroAndStaleTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeSizedAudio(t, dir, "one.mp3", 5, now)
	writeSizedAudio(t, dir, "two.mp3", 5, now)
	staleTemp := writeSizedAudio(t, dir, ".audio-download-old.tmp", 3, now.Add(-2*time.Hour))
	freshTemp := writeSizedAudio(t, dir, ".audio-download-new.tmp", 3, now)

	result, err := trimAudioCache(dir, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 3 || result.Freed != 13 {
		t.Fatalf("cleanup result = %+v", result)
	}
	if _, err := os.Stat(staleTemp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale temp remains: %v", err)
	}
	if _, err := os.Stat(freshTemp); err != nil {
		t.Fatalf("fresh temp removed: %v", err)
	}
}

func TestTrimAudioCacheContinuesAfterRemoveFailure(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	failed := writeSizedAudio(t, dir, "a.mp3", 5, now.Add(-time.Hour))
	removed := writeSizedAudio(t, dir, "b.mp3", 5, now)
	remove := func(path string) error {
		if path == failed {
			return errors.New("locked")
		}
		return os.Remove(path)
	}
	result, err := trimAudioCacheWithRemove(dir, 0, now, remove)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Failures) != 1 || result.Removed != 1 {
		t.Fatalf("cleanup result = %+v", result)
	}
	if _, err := os.Stat(removed); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second file remains: %v", err)
	}
}

func TestTrimAudioCacheIgnoresDirectoriesAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested.mp3")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	target := writeSizedAudio(t, dir, "target.txt", 5, time.Now())
	link := filepath.Join(dir, "link.mp3")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := trimAudioCache(dir, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{nested, target, link} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("ignored path %q was removed: %v", path, err)
		}
	}
}

func writeSizedAudio(t *testing.T, dir, name string, size int, modTime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	return path
}
