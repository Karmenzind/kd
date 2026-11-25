package update

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Every test that can reach moveFile's fallback installs this before calling it.
// The shim only records arguments and fails; it never invokes sudo or mv.
func installFakeSudo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "sudo-args")
	name := "sudo"
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$KD_TEST_SUDO_ARGS\"\nexit 23\n"
	if runtime.GOOS == "windows" {
		name = "sudo.bat"
		script = "@echo off\r\necho %* > \"%KD_TEST_SUDO_ARGS%\"\r\nexit /b 23\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KD_TEST_SUDO_ARGS", argsPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsPath
}

func assertFakeSudoFailure(t *testing.T, err error, argsPath, src, tgt string) {
	t.Helper()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("error = %v, want fake sudo exit status 23", err)
	}
	assertTestFile(t, argsPath, strings.Join([]string{"mv", src, tgt, ""}, "\n"))
}

func TestUpdateBinaryAt(t *testing.T) {
	argsPath := installFakeSudo(t)
	root := t.TempDir()
	exePath, tmpPath := filepath.Join(root, "kd"), filepath.Join(root, "kd.temp")
	writeTestFile(t, exePath, "old binary")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/binary" {
			t.Errorf("request = %s %s, want GET /binary", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, "new binary")
	}))
	defer server.Close()

	if err := updateBinaryAt(exePath, server.URL+"/binary", tmpPath); err != nil {
		t.Fatal(err)
	}
	assertTestFile(t, exePath, "new binary")
	assertTestMissing(t, tmpPath)
	assertTestMissing(t, argsPath)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0111 != 0111 {
			t.Fatalf("binary mode = %o, want all executable bits set", info.Mode().Perm())
		}
	}
}

func TestUpdateBinaryAtMoveFailure(t *testing.T) {
	argsPath := installFakeSudo(t)
	root := t.TempDir()
	exePath, tmpPath := filepath.Join(root, "missing", "kd"), filepath.Join(root, "kd.temp")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "new binary")
	}))
	defer server.Close()

	err := updateBinaryAt(exePath, server.URL, tmpPath)
	if err == nil {
		t.Fatal("updateBinaryAt succeeded with a missing destination directory")
	}
	assertTestFile(t, tmpPath, "new binary")
	assertTestMissing(t, exePath)
	if runtime.GOOS == "windows" {
		assertTestMissing(t, argsPath)
	} else {
		assertFakeSudoFailure(t, err, argsPath, tmpPath, exePath)
	}
}

func TestUpdateBinaryAtDownloadFailure(t *testing.T) {
	argsPath := installFakeSudo(t)
	root := t.TempDir()
	exePath := filepath.Join(root, "kd")
	tmpPath := filepath.Join(root, "kd.temp")
	writeTestFile(t, exePath, "old binary")
	// The body is shorter than the announced length, so the download fails
	// after the temporary file has been created.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, "partial")
	}))
	defer server.Close()

	// A download error stops the update before moveFile, so sudo never runs.
	err := updateBinaryAt(exePath, server.URL, tmpPath)
	if err == nil || !strings.Contains(err.Error(), "下载新版本失败") {
		t.Fatalf("updateBinaryAt() error = %v, want download failure", err)
	}
	if _, statErr := os.Stat(argsPath); !os.IsNotExist(statErr) {
		t.Fatalf("fake sudo ran after a failed download: %v", statErr)
	}
	assertTestFile(t, exePath, "old binary")
	assertTestMissing(t, tmpPath)
}

func TestMoveFile(t *testing.T) {
	for _, missingParent := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing parent=%v", missingParent), func(t *testing.T) {
			argsPath := installFakeSudo(t)
			root := t.TempDir()
			src, tgt := filepath.Join(root, "source"), filepath.Join(root, "target")
			writeTestFile(t, src, "replacement")
			if missingParent {
				tgt = filepath.Join(root, "missing", "target")
			} else {
				writeTestFile(t, tgt, "old contents")
			}

			err := moveFile(src, tgt)
			if !missingParent {
				if err != nil {
					t.Fatal(err)
				}
				assertTestFile(t, tgt, "replacement")
				assertTestMissing(t, src)
				assertTestMissing(t, argsPath)
				return
			}
			if err == nil {
				t.Fatal("moveFile succeeded with a missing destination directory")
			}
			assertTestFile(t, src, "replacement")
			assertTestMissing(t, tgt)
			if runtime.GOOS == "windows" {
				assertTestMissing(t, argsPath)
			} else {
				assertFakeSudoFailure(t, err, argsPath, src, tgt)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	t.Run("overwrites and truncates destination", func(t *testing.T) {
		root := t.TempDir()
		src, dest := filepath.Join(root, "source"), filepath.Join(root, "destination")
		writeTestFile(t, src, "new")
		writeTestFile(t, dest, "long old contents")
		if err := copyFile(src, dest); err != nil {
			t.Fatal(err)
		}
		assertTestFile(t, src, "new")
		assertTestFile(t, dest, "new")
	})
	t.Run("missing source preserves destination", func(t *testing.T) {
		root := t.TempDir()
		src, dest := filepath.Join(root, "missing"), filepath.Join(root, "destination")
		writeTestFile(t, dest, "old contents")
		if err := copyFile(src, dest); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("copyFile error = %v, want not exist", err)
		}
		assertTestFile(t, dest, "old contents")
	})
	t.Run("missing destination parent preserves source", func(t *testing.T) {
		root := t.TempDir()
		src, dest := filepath.Join(root, "source"), filepath.Join(root, "missing", "destination")
		writeTestFile(t, src, "new contents")
		if err := copyFile(src, dest); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("copyFile error = %v, want not exist", err)
		}
		assertTestFile(t, src, "new contents")
		assertTestMissing(t, dest)
	})
}

func TestReplaceExecutable(t *testing.T) {
	t.Run("replaces binary and removes backup", func(t *testing.T) {
		root := t.TempDir()
		oldPath, newPath := filepath.Join(root, "kd"), filepath.Join(root, "replacement")
		writeTestFile(t, oldPath, "old binary")
		writeTestFile(t, newPath, "new binary")
		if err := replaceExecutable(oldPath, newPath); err != nil {
			t.Fatal(err)
		}
		assertTestFile(t, oldPath, "new binary")
		assertTestFile(t, newPath, "new binary")
		assertTestMissing(t, oldPath+".update_backup")
	})
	t.Run("missing old binary leaves replacement intact", func(t *testing.T) {
		root := t.TempDir()
		oldPath, newPath := filepath.Join(root, "missing", "kd"), filepath.Join(root, "replacement")
		writeTestFile(t, newPath, "new binary")
		if err := replaceExecutable(oldPath, newPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("replaceExecutable error = %v, want not exist", err)
		}
		assertTestFile(t, newPath, "new binary")
		assertTestMissing(t, oldPath)
		assertTestMissing(t, oldPath+".update_backup")
	})
	t.Run("failed copy retains old binary in backup", func(t *testing.T) {
		root := t.TempDir()
		oldPath, newPath := filepath.Join(root, "kd"), filepath.Join(root, "missing", "replacement")
		writeTestFile(t, oldPath, "old binary")
		if err := replaceExecutable(oldPath, newPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("replaceExecutable error = %v, want not exist", err)
		}
		assertTestFile(t, oldPath+".update_backup", "old binary")
		assertTestMissing(t, oldPath)
	})
}
