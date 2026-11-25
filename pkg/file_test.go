package pkg

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestJSONFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := map[string]int{"first": 1, "second": 2}
	if err := SaveJson(path, want); err != nil {
		t.Fatalf("SaveJson() error = %v", err)
	}

	var got map[string]int
	if err := LoadJson(path, &got); err != nil {
		t.Fatalf("LoadJson() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadJson() = %#v, want %#v", got, want)
	}
	if !IsPathExists(path) {
		t.Fatalf("IsPathExists(%q) = false, want true", path)
	}
}

func TestLoadJSONErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.json")
		var got map[string]int
		if err := LoadJson(path, &got); !os.IsNotExist(err) {
			t.Fatalf("LoadJson(missing) error = %v, want os.ErrNotExist", err)
		}
		if IsPathExists(path) {
			t.Fatalf("IsPathExists(%q) = true, want false", path)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "invalid.json")
		if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		var got map[string]int
		if err := LoadJson(path, &got); err == nil {
			t.Fatal("LoadJson(invalid) returned nil error")
		}
	})
}

func TestAddExecutablePermission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte("!/bin/sh\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := AddExecutablePermission(path); err != nil {
		t.Fatalf("AddExecutablePermission() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	// Windows has no execute bits; Chmod only toggles read-only there.
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Fatalf("mode %o, want execute bits set", info.Mode())
	}
}

func TestAddExecutablePermissionMissing(t *testing.T) {
	err := AddExecutablePermission(filepath.Join(t.TempDir(), "missing"))
	if !os.IsNotExist(err) {
		t.Fatalf("AddExecutablePermission(missing) error = %v, want ErrNotExist", err)
	}
}

func TestLoadJSONEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	var got map[string]int
	if err := LoadJson(path, &got); err != nil {
		t.Fatalf("LoadJson(empty) error = %v", err)
	}
	if got != nil {
		t.Fatalf("LoadJson(empty) = %#v, want nil map", got)
	}
}
