package tts

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Karmenzind/kd/internal/run"
	"go.uber.org/zap"
)

const (
	audioCleanupInterval = 24 * time.Hour
	staleAudioTempAge    = time.Hour
	cleanupMarkerName    = "last_audio_cache_cleanup"
)

type cleanupResult struct {
	Removed  int
	Freed    int64
	Failures []error
}

type audioCacheFile struct {
	path    string
	size    int64
	modTime time.Time
}

func audioID(word string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(word)))
}

func buildTargetPath(word string) string {
	return filepath.Join(run.CACHE_AUDIO_DIR_PATH, audioID(word)+".mp3")
}

func resolveCachedAudio(word string) (string, bool, error) {
	target := buildTargetPath(word)
	if valid, err := validAudioFile(target); err != nil || valid {
		return target, valid, err
	}

	legacyName := strings.ReplaceAll(word, " ", "_") + ".mp3"
	if !safeLegacyAudioName(legacyName) {
		return target, false, nil
	}
	legacyPath := filepath.Join(run.CACHE_AUDIO_DIR_PATH, legacyName)
	valid, err := validAudioFile(legacyPath)
	if err != nil || !valid {
		return target, false, err
	}
	if err := os.Rename(legacyPath, target); err == nil {
		return target, true, nil
	}
	if valid, err := validAudioFile(target); err == nil && valid {
		return target, true, nil
	}
	return legacyPath, true, nil
}

func safeLegacyAudioName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		filepath.Base(name) == name &&
		!strings.ContainsAny(name, `/\\<>:"|?*`)
}

func cleanupAudioCacheAndLog(limit int64, now time.Time) {
	markerPath := filepath.Join(run.CACHE_RUN_PATH, cleanupMarkerName)
	runCleanup, err := audioCleanupDue(markerPath, limit, now)
	if err != nil {
		zap.S().Debugw("Failed to inspect TTS cleanup marker; cleanup will run", "error", err)
		runCleanup = true
	}
	if !runCleanup {
		return
	}
	result, err := trimAudioCache(run.CACHE_AUDIO_DIR_PATH, limit, now)
	if err != nil {
		zap.S().Warnw("Failed to inspect TTS audio cache", "error", err)
		return
	}
	for _, failure := range result.Failures {
		zap.S().Warnw("Failed to remove TTS audio cache file", "error", failure)
	}
	if limit > 0 {
		if err := writeCleanupMarker(markerPath, limit, now); err != nil {
			zap.S().Warnw("Failed to update TTS cleanup marker", "error", err)
		}
	}
	zap.S().Debugw("Checked TTS audio cache", "removed", result.Removed, "freed_bytes", result.Freed)
}

func audioCleanupDue(markerPath string, limit int64, now time.Time) (bool, error) {
	if limit == 0 {
		return true, nil
	}
	content, err := os.ReadFile(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	info, err := os.Stat(markerPath)
	if err != nil {
		return true, err
	}
	if strings.TrimSpace(string(content)) != strconv.FormatInt(limit, 10) {
		return true, nil
	}
	if info.ModTime().After(now) {
		return true, nil
	}
	return now.Sub(info.ModTime()) >= audioCleanupInterval, nil
}

func writeCleanupMarker(path string, limit int64, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(strconv.FormatInt(limit, 10)), 0o600); err != nil {
		return err
	}
	return os.Chtimes(path, now, now)
}

func trimAudioCache(directory string, limit int64, now time.Time) (cleanupResult, error) {
	return trimAudioCacheWithRemove(directory, limit, now, os.Remove)
}

func trimAudioCacheWithRemove(
	directory string,
	limit int64,
	now time.Time,
	remove func(string) error,
) (cleanupResult, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return cleanupResult{}, err
	}
	var result cleanupResult
	var total int64
	files := make([]audioCacheFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if strings.HasPrefix(entry.Name(), ".audio-") && strings.HasSuffix(entry.Name(), ".tmp") {
			if now.Sub(info.ModTime()) >= staleAudioTempAge {
				removeAudioCacheFile(path, info.Size(), &result, remove)
			}
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".mp3") {
			continue
		}
		total += info.Size()
		files = append(files, audioCacheFile{path: path, size: info.Size(), modTime: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path < files[j].path
		}
		return files[i].modTime.Before(files[j].modTime)
	})
	for _, file := range files {
		if total <= limit {
			break
		}
		if removeAudioCacheFile(file.path, file.size, &result, remove) {
			total -= file.size
		}
	}
	return result, nil
}

func removeAudioCacheFile(path string, size int64, result *cleanupResult, remove func(string) error) bool {
	if err := remove(path); err != nil {
		result.Failures = append(result.Failures, fmt.Errorf("remove %s: %w", filepath.Base(path), err))
		return false
	}
	result.Removed++
	result.Freed += size
	return true
}
