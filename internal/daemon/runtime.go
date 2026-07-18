package daemon

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
)

var ErrRuntimeCorrupt = errors.New("daemon runtime information is corrupt")

func GetDaemonInfoPath() string {
	return filepath.Join(run.CACHE_RUN_PATH, "daemon.json")
}

func ReadDaemonInfo(path string) (*model.RunInfo, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info model.RunInfo
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("%w: empty file", ErrRuntimeCorrupt)
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRuntimeCorrupt, err)
	}
	if info.PID <= 0 || info.Port == "" {
		return nil, fmt.Errorf("%w: missing PID or port", ErrRuntimeCorrupt)
	}
	return &info, nil
}

func GetDaemonInfoFromFile() (*model.RunInfo, error) {
	return ReadDaemonInfo(GetDaemonInfoPath())
}

func GetDaemonInfo() (*model.RunInfo, error) {
	return GetDaemonInfoFromFile()
}

func NewInstanceID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate daemon instance ID: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}

func WriteDaemonInfo(path string, info *model.RunInfo) error {
	if info == nil || info.PID <= 0 || info.Port == "" {
		return fmt.Errorf("%w: missing PID or port", ErrRuntimeCorrupt)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create daemon runtime directory: %w", err)
	}
	body, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("encode daemon runtime information: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".daemon-*.tmp")
	if err != nil {
		return fmt.Errorf("create daemon runtime temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set daemon runtime permissions: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("write daemon runtime information: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync daemon runtime information: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close daemon runtime information: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		// os.Rename may not replace an existing file on some Windows versions.
		// The listener owner is the only valid writer, so replacing a stale file
		// here cannot allow two daemons to become active.
		if runtime.GOOS != "windows" {
			return fmt.Errorf("replace daemon runtime information: %w", err)
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("remove old daemon runtime information: %w", removeErr)
		}
		if err := os.Rename(tmpPath, path); err != nil {
			return fmt.Errorf("replace daemon runtime information: %w", err)
		}
	}
	return nil
}

func daemonInfoOwnedBy(current, owner *model.RunInfo) bool {
	if current == nil || owner == nil {
		return false
	}
	if owner.Instance != "" || current.Instance != "" {
		return owner.Instance != "" && owner.Instance == current.Instance
	}
	return owner.PID == current.PID && owner.StartTime == current.StartTime
}

func RemoveDaemonInfo(path string, owner *model.RunInfo) error {
	current, err := ReadDaemonInfo(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !daemonInfoOwnedBy(current, owner) {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove daemon runtime information: %w", err)
	}
	return nil
}

func removeUnchangedRuntime(path string, original []byte) error {
	current, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
