package daemon

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/pkg"
)

func useDaemonInfoDir(t *testing.T) {
	t.Helper()
	originalPath := run.CACHE_RUN_PATH
	originalInfo := DaemonInfo
	run.CACHE_RUN_PATH = t.TempDir()
	DaemonInfo = &model.RunInfo{}
	t.Cleanup(func() {
		run.CACHE_RUN_PATH = originalPath
		DaemonInfo = originalInfo
	})
}

func TestGetDaemonInfo(t *testing.T) {
	useDaemonInfoDir(t)

	if _, err := GetDaemonInfoFromFile(); err == nil {
		t.Fatal("GetDaemonInfoFromFile() returned nil error for missing file")
	}

	want := &model.RunInfo{PID: 123, Port: "19707", Version: "v1.2.3"}
	if err := pkg.SaveJson(GetDaemonInfoPath(), want); err != nil {
		t.Fatalf("SaveJson() error = %v", err)
	}
	got, err := GetDaemonInfoFromFile()
	if err != nil {
		t.Fatalf("GetDaemonInfoFromFile() error = %v", err)
	}
	if got.PID != want.PID || got.Port != want.Port || got.Version != want.Version {
		t.Fatalf("daemon info = %+v, want %+v", got, want)
	}

	if err := os.WriteFile(GetDaemonInfoPath(), []byte("not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	DaemonInfo = &model.RunInfo{}
	if _, err := GetDaemonInfoFromFile(); err == nil {
		t.Fatal("GetDaemonInfoFromFile() returned nil error for invalid JSON")
	}
}

type zipEntry struct {
	name string
	body string
	dir  bool
}

func writeTestZIP(t *testing.T, path string, entries []zipEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	w := zip.NewWriter(f)
	for _, entry := range entries {
		name := entry.name
		if entry.dir && name[len(name)-1] != '/' {
			name += "/"
		}
		part, err := w.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if !entry.dir {
			if _, err := part.Write([]byte(entry.body)); err != nil {
				t.Fatalf("Write(%q) error = %v", name, err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}
}

func TestDecompressDBZIP(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "data.zip")
	tempDBPath := filepath.Join(dir, "database.temp")
	writeTestZIP(t, zipPath, []zipEntry{
		{name: "data", dir: true},
		{name: "data/kd_data.db", body: "database contents"},
	})

	if err := decompressDBZip(tempDBPath, zipPath); err != nil {
		t.Fatalf("decompressDBZip() error = %v", err)
	}
	body, err := os.ReadFile(tempDBPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(body), "database contents"; got != want {
		t.Fatalf("decompressed body = %q, want %q", got, want)
	}
}

func TestDecompressDBZIPFailuresPreserveCurrentDatabase(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string)
	}{
		{
			name: "invalid archive",
			prepare: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("not-a-zip"), 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			},
		},
		{
			name: "empty archive",
			prepare: func(t *testing.T, path string) {
				writeTestZIP(t, path, nil)
			},
		},
		{
			name: "directory only archive",
			prepare: func(t *testing.T, path string) {
				writeTestZIP(t, path, []zipEntry{{name: "data", dir: true}})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			zipPath := filepath.Join(dir, "data.zip")
			tempDBPath := filepath.Join(dir, "database.temp")
			currentDBPath := filepath.Join(dir, "kd_data.db")
			if err := os.WriteFile(currentDBPath, []byte("current database"), 0o600); err != nil {
				t.Fatalf("WriteFile(current DB) error = %v", err)
			}
			tt.prepare(t, zipPath)

			if err := decompressDBZip(tempDBPath, zipPath); err == nil {
				t.Fatal("decompressDBZip() returned nil error")
			}
			body, err := os.ReadFile(currentDBPath)
			if err != nil {
				t.Fatalf("ReadFile(current DB) error = %v", err)
			}
			if got, want := string(body), "current database"; got != want {
				t.Fatalf("current database = %q, want unchanged %q", got, want)
			}
		})
	}
}
