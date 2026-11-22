//go:build e2e && windows

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// openTerminal opens the console the tests were started from. os.NewFile
// recognizes it as a console and writes Unicode text, so Chinese shows up
// correctly whatever the console code page is.
func openTerminal() (*os.File, error) {
	name, err := syscall.UTF16PtrFromString("CONOUT$")
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), "CONOUT$"), nil
}

// zsh and fish are only tested on Linux and macOS. Windows PowerShell 5.1 is
// not supported; the README recommends pwsh instead.
func candidateShells() []*shell {
	return []*shell{
		posixShell("bash", gitBash()),
		powerShell("pwsh", lookPath("pwsh")),
		cmdShell(),
	}
}

// gitBash finds Git for Windows' bash. The bash.exe found first on PATH is
// usually the WSL launcher in System32, which would run a Linux shell.
func gitBash() string {
	if p := os.Getenv("KD_E2E_BASH"); p != "" {
		return p
	}
	git := lookPath("git")
	if git == "" {
		return ""
	}
	// git.exe lives in <Git>\cmd, <Git>\bin or <Git>\mingw64\bin.
	for dir := filepath.Dir(git); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		p := filepath.Join(dir, "bin", "bash.exe")
		if _, err := os.Stat(filepath.Join(dir, "git-bash.exe")); err == nil {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func cmdShell() *shell {
	path := os.Getenv("ComSpec")
	if path == "" {
		path = lookPath("cmd")
	}
	return &shell{
		name:  "cmd",
		path:  path,
		noTab: "cmd 没有可编程的补全，kd 不提供",
		command: func(ctx context.Context, sh *shell, line string) *exec.Cmd {
			c := exec.CommandContext(ctx, sh.path)
			// cmd does not follow the argument quoting Go applies, so pass the
			// command line exactly as a user would type it after "cmd /c".
			c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c "` + line + `"`}
			return c
		},
		// Double quotes keep cmd's metacharacters literal; embedded quotes use
		// the backslash escape that kd's own argument parser understands.
		quote:  func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` },
		invoke: func(sh *shell, path string) string { return sh.quote(path) },
		pipe:   func(line string) string { return line + ` | findstr "^"` },
	}
}
