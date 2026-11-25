package pkg

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func prependPATH(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeFakeProgram(t *testing.T, dir, name, unixBody, winBody string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, name+".bat")
		if err := os.WriteFile(path, []byte(winBody), 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
		return
	}
	path := filepath.Join(dir, name)
	body := "#!/bin/sh\n" + unixBody + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
