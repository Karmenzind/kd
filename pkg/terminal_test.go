package pkg

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOutputResultInvalidPagerCommand(t *testing.T) {
	stderr, errW := captureStderr(t, func() {
		err := OutputResult("hello", true, `less 'unclosed`)
		if err != nil {
			t.Fatalf("OutputResult() error = %v, want nil", err)
		}
	})
	if errW != nil {
		t.Fatal(errW)
	}
	if !strings.Contains(stderr, "pager command `less 'unclosed` is invalid:") {
		t.Fatalf("stderr = %q, want invalid pager warning", stderr)
	}
	if !strings.Contains(stderr, "unclosed quote") {
		t.Fatalf("stderr = %q, want unclosed quote in warning", stderr)
	}

	stdout, errW := captureStdout(t, func() {
		err := OutputResult("direct output", true, `less \`)
		if err != nil {
			t.Fatalf("OutputResult() error = %v, want nil", err)
		}
	})
	if errW != nil {
		t.Fatal(errW)
	}
	if stdout != "direct output\n" {
		t.Fatalf("stdout = %q, want direct output", stdout)
	}
}

func TestOutputResultFakePager(t *testing.T) {
	dir := t.TempDir()
	writeFakeProgram(t, dir, "kd-testpager", "exec cat", "@echo off\r\nmore")
	prependPATH(t, dir)

	stdout, errW := captureStdout(t, func() {
		if err := OutputResult("paged line", true, "kd-testpager"); err != nil {
			t.Fatalf("OutputResult() error = %v", err)
		}
	})
	if errW != nil {
		t.Fatal(errW)
	}
	// more on Windows ends lines with CRLF and adds a blank line.
	if runtime.GOOS == "windows" {
		stdout = strings.TrimRight(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n") + "\n"
	}
	if stdout != "paged line\n" {
		t.Fatalf("stdout = %q, want paged output", stdout)
	}
}

func TestOutputResultPagerNotFound(t *testing.T) {
	stderr, errW := captureStderr(t, func() {
		err := OutputResult("plain", true, "kd-missing-pager-xyzzy")
		if err != nil {
			t.Fatalf("OutputResult() error = %v, want nil", err)
		}
	})
	if errW != nil {
		t.Fatal(errW)
	}
	if !strings.Contains(stderr, "pager command `kd-missing-pager-xyzzy` not found") {
		t.Fatalf("stderr = %q, want not found warning", stderr)
	}
	stdout, errW := captureStdout(t, func() {
		err := OutputResult("plain", true, "kd-missing-pager-xyzzy")
		if err != nil {
			t.Fatalf("OutputResult() error = %v", err)
		}
	})
	if errW != nil {
		t.Fatal(errW)
	}
	if stdout != "plain\n" {
		t.Fatalf("stdout = %q, want direct output", stdout)
	}
}

func TestAskYNInvalidThenYes(t *testing.T) {
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
	if _, err := io.WriteString(w, "maybe\nY\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	stdout, errW := captureStdout(t, func() {
		if !AskYN("continue?") {
			t.Fatal("AskYN() = false, want true after invalid then Y")
		}
	})
	if errW != nil {
		t.Fatal(errW)
	}
	if !strings.Contains(stdout, "Please input Y or n.") {
		t.Fatalf("stdout = %q, want invalid prompt message", stdout)
	}
}

func TestClearScreenFake(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ClearScreen fake clear applies on Unix only")
	}
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "cleared")
	writeFakeProgram(t, dir, "clear", "touch "+marker, "")
	prependPATH(t, dir)

	ClearScreen()
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("fake clear did not run: stat marker: %v", err)
	}
}

func TestCommandExistsOnPATH(t *testing.T) {
	dir := t.TempDir()
	writeFakeProgram(t, dir, "kd-cmdtest", ":", "@echo off\r\nrem ok")
	prependPATH(t, dir)
	if !CommandExists("kd-cmdtest") {
		t.Fatal("CommandExists(kd-cmdtest) = false, want true")
	}
	if CommandExists("kd-cmdtest-absent-xyzzy") {
		t.Fatal("CommandExists(absent) = true, want false")
	}
}

func TestGetExecutablePath(t *testing.T) {
	path, err := GetExecutablePath()
	if err != nil {
		t.Fatalf("GetExecutablePath() error = %v", err)
	}
	if path == "" {
		t.Fatal("GetExecutablePath() returned empty path")
	}
	base, err := GetExecutableBasename()
	if err != nil {
		t.Fatalf("GetExecutableBasename() error = %v", err)
	}
	if base != filepath.Base(path) {
		t.Fatalf("basename = %q, want %q", base, filepath.Base(path))
	}
}

func TestHasAnyFlag(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"kd", "word", "--theme", "nord"}
	if !HasAnyFlag("theme") {
		t.Fatal("HasAnyFlag(theme) = false, want true")
	}
	os.Args = []string{"--theme", "kd", "word"}
	if HasAnyFlag("theme") {
		t.Fatal("HasAnyFlag(theme) at index 0 = true, want false")
	}
	if HasAnyFlag("missing-flag") {
		t.Fatal("HasAnyFlag(missing) = true, want false")
	}
}

func TestGetTermSize(t *testing.T) {
	oldW, oldH := term_width, term_height
	t.Cleanup(func() {
		term_width, term_height = oldW, oldH
	})
	term_width, term_height = 120, 40
	w, h, err := GetTermSize()
	if err != nil {
		t.Fatalf("GetTermSize() error = %v", err)
	}
	if w != 120 || h != 40 {
		t.Fatalf("GetTermSize() = (%d, %d), want (120, 40)", w, h)
	}
}

func captureStderr(t *testing.T, fn func()) (string, error) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return "", err
	}
	r.Close()
	return buf.String(), nil
}

func captureStdout(t *testing.T, fn func()) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return "", err
	}
	r.Close()
	return buf.String(), nil
}
