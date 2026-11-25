package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Karmenzind/kd/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestComponentField(t *testing.T) {
	var output bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&output), zap.InfoLevel)
	logger := zap.New(core, componentField("server"))
	logger.Info("started")

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("Unmarshal(log output) error = %v; output = %q", err, output.String())
	}
	if got := entry["component"]; got != "server" {
		t.Fatalf("component field = %v, want server", got)
	}
}

func TestBuildLoggerRedirectToStream(t *testing.T) {
	t.Cleanup(func() { LOG_FILE = "" })

	logger, err := buildLogger(&config.LoggerConfig{
		RedirectToStream: true,
		Level:            "info",
	})
	if err != nil {
		t.Fatalf("buildLogger() error = %v", err)
	}
	t.Cleanup(func() { _ = logger.Sync() })
	if LOG_FILE != "[stream]" {
		t.Fatalf("LOG_FILE = %q, want [stream]", LOG_FILE)
	}
}

func TestBuildLoggerInvalidLevel(t *testing.T) {
	_, err := buildLogger(&config.LoggerConfig{Level: "verbose"})
	if err == nil {
		t.Fatal("buildLogger() with invalid level returned nil error")
	}
}

func TestBuildLoggerWritesToPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("zap keeps log files open; t.TempDir cleanup fails on Windows")
	}
	t.Cleanup(func() { LOG_FILE = "" })

	logPath := filepath.Join(t.TempDir(), "kd-test.log")
	logger, err := buildLogger(&config.LoggerConfig{Path: logPath, Level: "debug"})
	if err != nil {
		t.Fatalf("buildLogger() error = %v", err)
	}
	t.Cleanup(func() { _ = logger.Sync() })
	if LOG_FILE != logPath {
		t.Fatalf("LOG_FILE = %q, want %q", LOG_FILE, logPath)
	}

	logger.Info("test line")
	if err := logger.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(body) == 0 {
		t.Fatal("log file is empty")
	}
}

func TestBuildLoggerReusesExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("zap keeps log files open; t.TempDir cleanup fails on Windows")
	}
	t.Cleanup(func() { LOG_FILE = "" })

	logPath := filepath.Join(t.TempDir(), "kd-existing.log")
	if err := os.WriteFile(logPath, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	logger, err := buildLogger(&config.LoggerConfig{Path: logPath, Level: "info"})
	if err != nil {
		t.Fatalf("buildLogger() error = %v", err)
	}
	t.Cleanup(func() { _ = logger.Sync() })
	logger.Info("appended")
}

func TestInitLoggerSetsGlobals(t *testing.T) {
	t.Cleanup(func() { LOG_FILE = "" })

	l, err := InitLogger(&config.LoggerConfig{RedirectToStream: true, Level: "info"}, "server")
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}
	t.Cleanup(func() { _ = l.Sync() })
	if LOG_FILE != "[stream]" {
		t.Fatalf("LOG_FILE = %q, want [stream]", LOG_FILE)
	}
	if zap.L() != l {
		t.Fatal("InitLogger() did not install the returned logger as global")
	}
}

func TestInitLoggerOpensWithoutExplicitComponent(t *testing.T) {
	t.Cleanup(func() { LOG_FILE = "" })

	l, err := InitLogger(&config.LoggerConfig{RedirectToStream: true, Level: "warn"})
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}
	t.Cleanup(func() { _ = l.Sync() })
	if zap.L() != l {
		t.Fatal("InitLogger() without component did not install global logger")
	}
}
