package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/Karmenzind/kd/internal/model"
)

func TestWriteDaemonInfoRejectsInvalidInfoWithoutChangingFile(t *testing.T) {
	for _, tt := range []struct {
		name string
		info *model.RunInfo
	}{
		{name: "nil"},
		{name: "missing PID", info: &model.RunInfo{Port: "12345"}},
		{name: "negative PID", info: &model.RunInfo{PID: -1, Port: "12345"}},
		{name: "missing port", info: &model.RunInfo{PID: 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "daemon.json")
			if err := os.WriteFile(path, []byte("existing runtime"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := WriteDaemonInfo(path, tt.info); !errors.Is(err, ErrRuntimeCorrupt) {
				t.Fatalf("WriteDaemonInfo() = %v, want ErrRuntimeCorrupt", err)
			}
			body, err := os.ReadFile(path)
			if err != nil || string(body) != "existing runtime" {
				t.Fatalf("invalid info changed runtime: body=%q err=%v", body, err)
			}
		})
	}
}

func TestWriteDaemonInfoCreatesAndReplacesRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "run", "daemon.json")
	for _, info := range []*model.RunInfo{
		{PID: 10, Port: "12345", Version: "v1", StartTime: 1, Instance: "old"},
		{PID: 11, Port: "12346", Version: "v2", StartTime: 2, Instance: "new"},
	} {
		if err := WriteDaemonInfo(path, info); err != nil {
			t.Fatal(err)
		}
		got, err := ReadDaemonInfo(path)
		if err != nil || !reflect.DeepEqual(got, info) {
			t.Fatalf("runtime = %+v, want %+v; error=%v", got, info, err)
		}
		stat, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && stat.Mode().Perm() != 0o600 {
			t.Fatalf("runtime permissions = %o, want 600", stat.Mode().Perm())
		}
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".daemon-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary runtime files left behind: %v, %v", leftovers, err)
	}
}

func TestWriteDaemonInfoFailureCleansTemporaryFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "daemon.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	// A nonempty directory cannot be replaced or removed, including on Windows.
	sentinel := filepath.Join(path, "owned-by-test")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	info := &model.RunInfo{PID: 1, Port: "12345"}
	if err := WriteDaemonInfo(path, info); err == nil {
		t.Fatal("WriteDaemonInfo() replaced a nonempty directory")
	}
	if body, err := os.ReadFile(sentinel); err != nil || string(body) != "keep" {
		t.Fatalf("failed write changed existing path: %q, %v", body, err)
	}
	leftovers, err := filepath.Glob(filepath.Join(root, ".daemon-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("failed write leaked temporary files: %v, %v", leftovers, err)
	}
}

func TestReadDaemonInfoClassifiesCorruption(t *testing.T) {
	for _, body := range []string{"", " \n\t", "{", "null", `{}`, `{"PID":1}`, `{"PID":0,"Port":"12345"}`} {
		t.Run(body, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "daemon.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadDaemonInfo(path); !errors.Is(err, ErrRuntimeCorrupt) {
				t.Fatalf("ReadDaemonInfo(%q) = %v, want ErrRuntimeCorrupt", body, err)
			}
		})
	}
	if _, err := ReadDaemonInfo(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrRuntimeCorrupt) {
		t.Fatalf("missing runtime classified incorrectly: %v", err)
	}
}
