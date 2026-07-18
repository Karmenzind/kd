package tts

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxAudioFileSize int64 = 10 << 20

type audioURLBuilder func(string) string

func downloadAudio(
	ctx context.Context,
	word, filePath string,
	client *http.Client,
	buildURL audioURLBuilder,
) error {
	if valid, err := validAudioFile(filePath); err != nil {
		return err
	} else if valid {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildURL(word), nil)
	if err != nil {
		return fmt.Errorf("create audio request: %w", err)
	}
	req.Header.Set("Accept", "audio/mpeg,audio/*;q=0.9,application/octet-stream;q=0.5")
	req.Header.Set("User-Agent", "kd")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request audio: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("audio service returned %s", resp.Status)
	}
	if resp.ContentLength == 0 {
		return errors.New("audio service returned an empty response")
	}
	if resp.ContentLength > maxAudioFileSize {
		return fmt.Errorf("audio response exceeds %d bytes", maxAudioFileSize)
	}
	if invalidAudioContentType(resp.Header.Get("Content-Type")) {
		return fmt.Errorf("audio service returned invalid content type %q", resp.Header.Get("Content-Type"))
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("create audio cache directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(filePath), ".audio-download-*.tmp")
	if err != nil {
		return fmt.Errorf("create audio temporary file: %w", err)
	}
	tempPath := temp.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			os.Remove(tempPath)
		}
	}()

	written, copyErr := io.Copy(temp, io.LimitReader(resp.Body, maxAudioFileSize+1))
	closeErr := temp.Close()
	if copyErr != nil {
		return fmt.Errorf("write audio temporary file: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close audio temporary file: %w", closeErr)
	}
	if written == 0 {
		return errors.New("audio service returned an empty response")
	}
	if written > maxAudioFileSize {
		return fmt.Errorf("audio response exceeds %d bytes", maxAudioFileSize)
	}
	if valid, validErr := validAudioFile(tempPath); validErr != nil {
		return validErr
	} else if !valid {
		return errors.New("audio service returned invalid audio data")
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		if valid, validErr := validAudioFile(filePath); validErr == nil && valid {
			return nil
		}
		return fmt.Errorf("publish audio cache file: %w", err)
	}
	keepTemp = true
	return nil
}

func validAudioFile(path string) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open audio cache file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("inspect audio cache file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > maxAudioFileSize {
		if info.Mode().IsRegular() {
			_ = file.Close()
			_ = os.Remove(path)
		}
		return false, nil
	}
	reader := bufio.NewReader(io.LimitReader(file, 512))
	prefix, err := io.ReadAll(reader)
	if err != nil {
		return false, fmt.Errorf("inspect audio cache contents: %w", err)
	}
	if looksLikeTextError(prefix) {
		_ = file.Close()
		_ = os.Remove(path)
		return false, nil
	}
	return true, nil
}

func invalidAudioContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "xml") ||
		strings.Contains(contentType, "html")
}

func looksLikeTextError(prefix []byte) bool {
	value := strings.ToLower(strings.TrimSpace(string(prefix)))
	return strings.HasPrefix(value, "<!doctype") ||
		strings.HasPrefix(value, "<html") ||
		strings.HasPrefix(value, "<?xml") ||
		strings.HasPrefix(value, "{")
}
