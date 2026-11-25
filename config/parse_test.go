package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigTOMLErrors(t *testing.T) {
	originalPath, originalCfg := CONFIG_PATH, Cfg
	t.Cleanup(func() { CONFIG_PATH, Cfg = originalPath, originalCfg })
	for _, tt := range []struct {
		name, content string
	}{
		{"syntax", "[logging\n"},
		{"type", "paging = \"yes\"\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("http_proxy", "")
			CONFIG_PATH = getConfigPath()
			if err := os.MkdirAll(filepath.Dir(CONFIG_PATH), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(CONFIG_PATH, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			Cfg = Config{}
			if err := parseConfig(); err == nil || !strings.HasPrefix(err.Error(), "toml") {
				t.Fatalf("parseConfig() error = %v, want toml prefix", err)
			}
			Cfg = Config{}
			if err := InitConfig(); err == nil || !strings.HasPrefix(err.Error(), "解析配置文件失败，请检查toml文件语法（") {
				t.Fatalf("InitConfig() error = %v, want TOML error wrapper", err)
			}
		})
	}
}

func TestConfigPathIsDirectory(t *testing.T) {
	originalPath, originalCfg := CONFIG_PATH, Cfg
	t.Cleanup(func() { CONFIG_PATH, Cfg = originalPath, originalCfg })
	CONFIG_PATH = t.TempDir()
	Cfg = Config{}
	if err := parseConfig(); err == nil {
		t.Fatal("parseConfig() succeeded for a directory, want read error")
	}
	info, err := os.Stat(CONFIG_PATH)
	if err != nil {
		t.Fatal(err)
	}
	if !Cfg.FileExists || Cfg.ModTime != info.ModTime().Unix() {
		t.Fatalf("directory metadata = (%v, %d), want (true, %d)", Cfg.FileExists, Cfg.ModTime, info.ModTime().Unix())
	}
}

func TestConfigIgnoresConfigorFeatures(t *testing.T) {
	originalPath, originalCfg := CONFIG_PATH, Cfg
	t.Cleanup(func() { CONFIG_PATH, Cfg = originalPath, originalCfg })
	for key, value := range map[string]string{
		"CONFIGOR_ENV":          "custom",
		"CONFIGOR_THEME":        "wudao",
		"CONFIGOR_ENV_PREFIX":   "KD_TEST_CONFIG",
		"KD_TEST_CONFIG_THEME":  "wudao",
		"CONFIGOR_DEBUG_MODE":   "true",
		"CONFIGOR_VERBOSE_MODE": "true",
		"CONFIGOR_SILENT_MODE":  "true",
	} {
		t.Setenv(key, value)
	}
	for _, name := range []string{"kd.toml", "missing.toml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for _, extra := range []string{"kd.custom.toml", "kd.example.toml", "missing.custom.toml", "missing.example.toml"} {
				if err := os.WriteFile(filepath.Join(dir, extra), []byte("theme = \"wudao\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			CONFIG_PATH = filepath.Join(dir, name)
			if name == "kd.toml" {
				if err := os.WriteFile(CONFIG_PATH, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			Cfg = Config{}
			if err := parseConfig(); err != nil {
				t.Fatal(err)
			}
			if Cfg.Theme != "temp" {
				t.Fatalf("Theme = %q, want default temp", Cfg.Theme)
			}
		})
	}
}
