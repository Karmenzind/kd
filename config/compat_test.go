package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The JSON fixtures were generated with configor before replacing the loader.
// FileExists and ModTime describe the input file rather than its configuration.
func TestConfigCompatibility(t *testing.T) {
	originalPath, originalCfg := CONFIG_PATH, Cfg
	t.Cleanup(func() { CONFIG_PATH, Cfg = originalPath, originalCfg })
	paths, err := filepath.Glob("testdata/compat/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no compatibility fixtures found")
	}
	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".toml"), func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			golden, err := os.ReadFile(strings.TrimSuffix(path, ".toml") + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var want Config
			if err := json.Unmarshal(golden, &want); err != nil {
				t.Fatal(err)
			}
			CONFIG_PATH = filepath.Join(t.TempDir(), "kd.toml")
			if err := os.WriteFile(CONFIG_PATH, content, 0o600); err != nil {
				t.Fatal(err)
			}
			Cfg = Config{}
			if err := parseConfig(); err != nil {
				t.Fatalf("parseConfig() error = %v", err)
			}
			info, err := os.Stat(CONFIG_PATH)
			if err != nil {
				t.Fatal(err)
			}
			if !Cfg.FileExists || Cfg.ModTime != info.ModTime().Unix() {
				t.Fatalf("file metadata = (%v, %d), want (true, %d)", Cfg.FileExists, Cfg.ModTime, info.ModTime().Unix())
			}
			got := Cfg
			got.FileExists, got.ModTime = false, 0
			if got != want {
				t.Fatalf("config = %+v, want configor result %+v", got, want)
			}
		})
	}
}
