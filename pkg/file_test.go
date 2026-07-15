package pkg

import (
	"os"
	"path/filepath"
	"reflect"
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
