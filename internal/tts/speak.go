package tts

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Karmenzind/kd/internal/run"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"go.uber.org/zap"
)

const (
	audioDownloadTimeout = 5 * time.Second
	audioPlaybackTimeout = 30 * time.Second
)

var ErrNoSpeaker = errors.New("no suitable audio player found")

// Speak downloads and plays a word pronunciation. cacheLimitMB is expressed in
// MiB; zero keeps no audio files after playback.
func Speak(ctx context.Context, word string, cacheLimitMB uint64) error {
	if strings.TrimSpace(word) == "" {
		return errors.New("empty TTS query")
	}
	cacheLimit, err := cacheLimitBytes(cacheLimitMB)
	if err != nil {
		return err
	}

	player := findSpeaker(runtime.GOOS, exec.LookPath)
	if player == "" {
		return ErrNoSpeaker
	}
	if player == systemPlayer {
		d.EchoWarn("推荐安装mpv或ffmpeg支持无弹窗播放声音")
	}
	zap.S().Debugw("Selected TTS player", "player", player)

	defer cleanupAudioCacheAndLog(cacheLimit, time.Now())

	filePath, cached, err := resolveCachedAudio(word)
	if err != nil {
		return fmt.Errorf("prepare audio cache: %w", err)
	}
	if cacheLimit == 0 {
		file, createErr := os.CreateTemp(run.CACHE_AUDIO_DIR_PATH, ".audio-uncached-*.mp3")
		if createErr != nil {
			return fmt.Errorf("create temporary audio path: %w", createErr)
		}
		filePath = file.Name()
		if closeErr := file.Close(); closeErr != nil {
			os.Remove(filePath)
			return fmt.Errorf("close temporary audio path: %w", closeErr)
		}
		if removeErr := os.Remove(filePath); removeErr != nil {
			return fmt.Errorf("prepare temporary audio path: %w", removeErr)
		}
		defer os.Remove(filePath)
		cached = false
	}

	if !cached {
		client := &http.Client{Timeout: audioDownloadTimeout}
		if err := downloadAudio(ctx, word, filePath, client, buildAudioUrl); err != nil {
			zap.S().Warnw("TTS audio download failed", "audio_id", audioID(word), "error", err)
			return fmt.Errorf("download audio: %w", err)
		}
		zap.S().Infow("Downloaded TTS audio", "audio_id", audioID(word))
	}

	if err := playAudio(ctx, runtime.GOOS, player, filePath); err != nil {
		zap.S().Warnw("TTS playback failed", "audio_id", audioID(word), "player", player, "error", err)
		return fmt.Errorf("play audio: %w", err)
	}
	if cacheLimit > 0 {
		now := time.Now()
		if err := os.Chtimes(filePath, now, now); err != nil {
			zap.S().Debugw("Failed to update TTS cache access time", "audio_id", audioID(word), "error", err)
		}
	}
	zap.S().Infow("Played TTS audio", "audio_id", audioID(word), "player", player)
	return nil
}

func cacheLimitBytes(limitMB uint64) (int64, error) {
	const bytesPerMiB = uint64(1 << 20)
	if limitMB > uint64(math.MaxInt64)/bytesPerMiB {
		return 0, fmt.Errorf("audio cache limit is too large: %d MiB", limitMB)
	}
	return int64(limitMB * bytesPerMiB), nil
}
