package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Karmenzind/kd/config"
)

func TestEnsureDefaultConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "kd.toml")
	created, err := ensureDefaultConfigFile(path)
	if err != nil {
		t.Fatalf("ensureDefaultConfigFile() error = %v", err)
	}
	if !created {
		t.Fatal("ensureDefaultConfigFile() created = false, want true")
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want, err := config.GenerateDefaultConfig()
	if err != nil {
		t.Fatalf("GenerateDefaultConfig() error = %v", err)
	}
	if string(body) != want {
		t.Fatalf("generated config = %q, want %q", body, want)
	}

	const existing = "paging = false\n"
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	created, err = ensureDefaultConfigFile(path)
	if err != nil {
		t.Fatalf("ensureDefaultConfigFile(existing) error = %v", err)
	}
	if created {
		t.Fatal("ensureDefaultConfigFile(existing) created = true, want false")
	}
	body, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(existing) error = %v", err)
	}
	if string(body) != existing {
		t.Fatalf("existing config = %q, want unchanged %q", body, existing)
	}
}
