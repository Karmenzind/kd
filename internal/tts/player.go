package tts

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const (
	systemPlayer      = "system"
	playerOutputLimit = 4 << 10
)

type lookPathFunc func(string) (string, error)

func findSpeaker(goos string, lookPath lookPathFunc) string {
	var candidates []string
	switch goos {
	case "linux":
		candidates = []string{"ffplay", "mpg123", "mpv"}
	case "darwin":
		candidates = []string{"afplay", "ffplay", "mpv"}
	case "windows":
		candidates = []string{"ffplay", "mpv"}
	}
	for _, candidate := range candidates {
		if _, err := lookPath(candidate); err == nil {
			return candidate
		}
	}
	if goos == "windows" {
		return systemPlayer
	}
	return ""
}

func playerCommand(goos, player, filePath string) (string, []string, error) {
	switch player {
	case "mpv":
		return "mpv", []string{"--no-terminal", "--really-quiet", filePath}, nil
	case "ffplay":
		return "ffplay", []string{"-nodisp", "-autoexit", "-loglevel", "quiet", filePath}, nil
	case "mpg123":
		return "mpg123", []string{"-q", filePath}, nil
	case "afplay":
		return "afplay", []string{filePath}, nil
	case systemPlayer:
		if goos == "windows" {
			return "explorer.exe", []string{filePath}, nil
		}
	}
	return "", nil, ErrNoSpeaker
}

func playAudio(ctx context.Context, goos, player, filePath string) error {
	program, args, err := playerCommand(goos, player, filePath)
	if err != nil {
		return err
	}
	playbackCtx, cancel := context.WithTimeout(ctx, audioPlaybackTimeout)
	defer cancel()
	cmd := exec.CommandContext(playbackCtx, program, args...)
	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if playbackCtx.Err() != nil {
			return fmt.Errorf("player %s timed out or was cancelled: %w", player, playbackCtx.Err())
		}
		detail := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, " "))
		if detail != "" {
			return fmt.Errorf("player %s failed: %w (%s)", player, err, detail)
		}
		return fmt.Errorf("player %s failed: %w", player, err)
	}
	return nil
}

type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	originalLength := len(value)
	remaining := playerOutputLimit - b.buf.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.buf.Write(value)
	}
	return originalLength, nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}
