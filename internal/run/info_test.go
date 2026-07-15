package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheRootPathFor(t *testing.T) {
	home := filepath.Join("home", "tester")
	for _, tt := range []struct {
		goos string
		want string
	}{
		{goos: "windows", want: filepath.Join(home, ".cache", "kdcache")},
		{goos: "linux", want: filepath.Join(home, ".cache", "kdcache")},
		{goos: "darwin", want: filepath.Join(home, "Library", "Caches", "kdcache")},
		{goos: "other", want: filepath.Join(home, ".cache", "kdcache")},
	} {
		t.Run(tt.goos, func(t *testing.T) {
			if got := cacheRootPathFor(tt.goos, home); got != tt.want {
				t.Fatalf("cacheRootPathFor(%q, %q) = %q, want %q", tt.goos, home, got, tt.want)
			}
		})
	}
}

func TestEnsureDirectories(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "cache"),
		filepath.Join(root, "cache", "run"),
		filepath.Join(root, "cache", "audio"),
	}
	if err := ensureDirectories(paths...); err != nil {
		t.Fatalf("ensureDirectories() error = %v", err)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("directory %q: info=%v err=%v", path, info, err)
		}
	}
}
