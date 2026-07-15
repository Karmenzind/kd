package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfigAndApply(t *testing.T) {
	originalPath := CONFIG_PATH
	originalCfg := Cfg
	t.Cleanup(func() {
		CONFIG_PATH = originalPath
		Cfg = originalCfg
	})

	CONFIG_PATH = filepath.Join(t.TempDir(), "kd.toml")
	content := `
theme = "wudao"
brief = true
paging = false

[logging]
enable = false
level = "WARNING"
stderr = true
`
	if err := os.WriteFile(CONFIG_PATH, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	Cfg = Config{}
	if err := parseConfig(); err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if err := Cfg.CheckAndApply(); err != nil {
		t.Fatalf("CheckAndApply() error = %v", err)
	}

	if !Cfg.FileExists {
		t.Fatal("FileExists = false, want true")
	}
	if Cfg.Theme != "wudao" || !Cfg.Brief || Cfg.Paging {
		t.Fatalf("parsed config = %+v", Cfg)
	}
	if Cfg.Logging.Enable || !Cfg.Logging.Stderr || Cfg.Logging.Level != "warn" {
		t.Fatalf("parsed logging config = %+v", Cfg.Logging)
	}
	if !Cfg.EnableEmoji {
		t.Fatal("EnableEmoji default = false, want true")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		level   string
	}{
		{
			name:    "invalid proxy",
			config:  Config{HTTPProxy: "localhost:8080"},
			wantErr: true,
		},
		{
			name:    "invalid log level",
			config:  Config{Logging: LoggerConfig{Level: "verbose"}},
			wantErr: true,
		},
		{
			name:   "warning alias",
			config: Config{Logging: LoggerConfig{Level: "WARNING"}},
			level:  "warn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.CheckAndApply()
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckAndApply() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.level != "" && tt.config.Logging.Level != tt.level {
				t.Fatalf("Logging.Level = %q, want %q", tt.config.Logging.Level, tt.level)
			}
		})
	}
}

func TestGenerateDefaultConfig(t *testing.T) {
	originalCfg := Cfg
	Cfg = Config{Theme: "temp", Paging: true, EnableEmoji: true}
	t.Cleanup(func() { Cfg = originalCfg })

	got, err := GenerateDefaultConfig()
	if err != nil {
		t.Fatalf("GenerateDefaultConfig() error = %v", err)
	}
	for _, unexpected := range []string{"debug =", "enable_emoji =", "fileexists", "modtime"} {
		if strings.Contains(strings.ToLower(got), unexpected) {
			t.Fatalf("generated config contains internal field %q:\n%s", unexpected, got)
		}
	}
	if !strings.Contains(got, `theme = "temp"`) || !strings.Contains(got, "[logging]") {
		t.Fatalf("generated config is missing expected settings:\n%s", got)
	}
}

func TestMissingConfigUsesDefaults(t *testing.T) {
	originalPath := CONFIG_PATH
	originalCfg := Cfg
	t.Cleanup(func() {
		CONFIG_PATH = originalPath
		Cfg = originalCfg
	})

	CONFIG_PATH = filepath.Join(t.TempDir(), "missing.toml")
	Cfg = Config{}
	if err := parseConfig(); err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if Cfg.FileExists {
		t.Fatal("FileExists = true for missing config")
	}
	if Cfg.Theme != "temp" || !Cfg.EnableEmoji {
		t.Fatalf("defaults = %+v, want theme=temp and enable_emoji=true", Cfg)
	}
	if !Cfg.Logging.Enable || Cfg.Logging.Level != "warn" {
		t.Fatalf("logging defaults = %+v", Cfg.Logging)
	}
}

func TestGeneratedConfigRoundTrip(t *testing.T) {
	originalPath := CONFIG_PATH
	originalCfg := Cfg
	t.Cleanup(func() {
		CONFIG_PATH = originalPath
		Cfg = originalCfg
	})

	Cfg = Config{
		Paging:       true,
		PagerCommand: "less -RF",
		EnglishOnly:  true,
		Theme:        "wudao",
		Brief:        true,
		Logging: LoggerConfig{
			Enable: true,
			Level:  "info",
			Stderr: true,
		},
	}
	generated, err := GenerateDefaultConfig()
	if err != nil {
		t.Fatalf("GenerateDefaultConfig() error = %v", err)
	}

	CONFIG_PATH = filepath.Join(t.TempDir(), "generated.toml")
	if err := os.WriteFile(CONFIG_PATH, []byte(generated), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	Cfg = Config{}
	if err := parseConfig(); err != nil {
		t.Fatalf("parse generated config error = %v\n%s", err, generated)
	}

	if Cfg.Theme != "wudao" || !Cfg.Paging || Cfg.PagerCommand != "less -RF" || !Cfg.EnglishOnly || !Cfg.Brief {
		t.Fatalf("round-tripped config = %+v", Cfg)
	}
	if !Cfg.Logging.Enable || Cfg.Logging.Level != "info" || !Cfg.Logging.Stderr {
		t.Fatalf("round-tripped logging config = %+v", Cfg.Logging)
	}
	if !Cfg.EnableEmoji {
		t.Fatal("omitted enable_emoji did not recover its default")
	}
}

func TestConfigPathFor(t *testing.T) {
	home := filepath.Join("home", "tester")
	for _, tt := range []struct {
		goos string
		want string
	}{
		{goos: "windows", want: filepath.Join(home, "kd.toml")},
		{goos: "linux", want: filepath.Join(home, ".config", "kd.toml")},
		{goos: "darwin", want: filepath.Join(home, ".config", "kd.toml")},
	} {
		t.Run(tt.goos, func(t *testing.T) {
			if got := configPathFor(tt.goos, home); got != tt.want {
				t.Fatalf("configPathFor(%q, %q) = %q, want %q", tt.goos, home, got, tt.want)
			}
		})
	}
}
