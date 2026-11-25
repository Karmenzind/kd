package config

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInitConfigLoadsHomeConfig(t *testing.T) {
	originalPath, originalCfg, originalTransport := CONFIG_PATH, Cfg, http.DefaultTransport
	t.Cleanup(func() {
		CONFIG_PATH = originalPath
		Cfg = originalCfg
		http.DefaultTransport = originalTransport
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("http_proxy", "")

	var wantPath string
	if runtime.GOOS == "windows" {
		wantPath = filepath.Join(home, "kd.toml")
	} else {
		wantPath = filepath.Join(home, ".config", "kd.toml")
	}
	if err := os.MkdirAll(filepath.Dir(wantPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := "theme = \"wudao\"\nbrief = true\n"
	if err := os.WriteFile(wantPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	Cfg = Config{}
	if err := InitConfig(); err != nil {
		t.Fatalf("InitConfig() error = %v", err)
	}
	if !Cfg.FileExists || Cfg.Theme != "wudao" || !Cfg.Brief {
		t.Fatalf("loaded config = %+v", Cfg)
	}
	if CONFIG_PATH != wantPath {
		t.Fatalf("CONFIG_PATH = %q, want %q", CONFIG_PATH, wantPath)
	}
}

func TestInitConfigAppliesHTTPProxy(t *testing.T) {
	originalPath, originalCfg, originalTransport := CONFIG_PATH, Cfg, http.DefaultTransport
	t.Cleanup(func() {
		CONFIG_PATH = originalPath
		Cfg = originalCfg
		http.DefaultTransport = originalTransport
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("http_proxy", "")

	CONFIG_PATH = filepath.Join(t.TempDir(), "kd.toml")
	content := "http_proxy = \"http://127.0.0.1:8080\"\n"
	if err := os.WriteFile(CONFIG_PATH, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	Cfg = Config{}
	if err := InitConfig(); err != nil {
		t.Fatalf("InitConfig() error = %v", err)
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || transport.Proxy == nil {
		t.Fatal("InitConfig() did not install an HTTP proxy transport")
	}
}

func TestCheckAndApplyLogToStreamFlag(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	os.Args = []string{"kd", "--log-to-stream"}
	cfg := Config{}
	if err := cfg.CheckAndApply(); err != nil {
		t.Fatalf("CheckAndApply() error = %v", err)
	}
	if !cfg.Logging.Enable || !cfg.Logging.RedirectToStream {
		t.Fatalf("logging config = %+v, want redirect to stream", cfg.Logging)
	}
}
