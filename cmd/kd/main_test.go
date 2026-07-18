package main

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLISmoke(t *testing.T) {
	if runtime.GOOS != "windows" {
		current, err := user.Current()
		if err == nil && current.Username == "root" {
			t.Skip("kd intentionally refuses to run as root")
		}
	}

	tempDir := t.TempDir()
	binaryName := "kd-test"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(tempDir, binaryName)
	build := exec.Command("go", "build", "-mod=mod", "-o", binaryPath, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	home := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("MkdirAll(home) error = %v", err)
	}
	run := func(t *testing.T, args ...string) (stdout, stderr string, err error) {
		t.Helper()
		cmd := exec.Command(binaryPath, args...)
		cmd.Env = append(os.Environ(),
			"HOME="+home,
			"USERPROFILE="+home,
			"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		)
		var stdoutBuffer, stderrBuffer strings.Builder
		cmd.Stdout = &stdoutBuffer
		cmd.Stderr = &stderrBuffer
		err = cmd.Run()
		return stdoutBuffer.String(), stderrBuffer.String(), err
	}

	t.Run("help", func(t *testing.T) {
		stdout, stderr, err := run(t, "--help")
		if err != nil {
			t.Fatalf("kd --help error = %v, stderr = %q", err, stderr)
		}
		if stderr != "" {
			t.Fatalf("kd --help stderr = %q, want empty", stderr)
		}
		if !strings.Contains(stdout, "GLOBAL OPTIONS") || !strings.Contains(stdout, "--version") {
			t.Fatalf("help stdout = %q", stdout)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout, stderr, err := run(t, "--version")
		if err != nil {
			t.Fatalf("kd --version error = %v, stderr = %q", err, stderr)
		}
		if stderr != "" {
			t.Fatalf("kd --version stderr = %q, want empty", stderr)
		}
		if !strings.Contains(stdout, VERSION) {
			t.Fatalf("version stdout = %q, want %q", stdout, VERSION)
		}
	})

	t.Run("missing query", func(t *testing.T) {
		stdout, stderr, err := run(t)
		if err != nil {
			t.Fatalf("kd without query error = %v, stderr = %q", err, stderr)
		}
		if !strings.Contains(stdout, "<text>") || !strings.Contains(stdout, "查看详细帮助") {
			t.Fatalf("prompt stdout = %q", stdout)
		}
	})

	t.Run("local status", func(t *testing.T) {
		stdout, stderr, err := run(t, "--status")
		if err != nil {
			t.Fatalf("kd --status error = %v, stderr = %q", err, stderr)
		}
		for _, want := range []string{"版本", "Daemon状态", "配置文件地址", "数据文件目录"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("status stdout = %q, missing %q", stdout, want)
			}
		}
	})

	t.Run("invalid flag", func(t *testing.T) {
		stdout, stderr, err := run(t, "--definitely-invalid")
		if err == nil {
			t.Fatalf("invalid flag exited successfully; stdout = %q stderr = %q", stdout, stderr)
		}
		if stderr == "" || !strings.Contains(stdout+stderr, "definitely-invalid") {
			t.Fatalf("invalid flag stdout = %q stderr = %q", stdout, stderr)
		}
	})
}

func TestShouldEnableQueryProgress(t *testing.T) {
	for _, tt := range []struct {
		name        string
		jsonOutput  bool
		logToStream bool
		terminal    bool
		want        bool
	}{
		{name: "interactive query", terminal: true, want: true},
		{name: "redirected", terminal: false},
		{name: "JSON", jsonOutput: true, terminal: true},
		{name: "log stream", logToStream: true, terminal: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldEnableQueryProgress(tt.jsonOutput, tt.logToStream, tt.terminal); got != tt.want {
				t.Fatalf("shouldEnableQueryProgress(%v, %v, %v) = %v, want %v", tt.jsonOutput, tt.logToStream, tt.terminal, got, tt.want)
			}
		})
	}
}
