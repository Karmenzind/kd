package tts

import (
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
)

// TODO (k): <2025-02-02 20:23> cached files' loc

// downloadAudio checks if the audio file already exists locally, and if not, downloads it
func downloadAudio(word string) (string, error) {
	// URL-encode the word to handle special characters
	encodedWord := url.QueryEscape(word)
	fileName := encodedWord + ".mp3"

	if _, err := os.Stat(fileName); err == nil {
		zap.S().Infow("Audio file already exists", "file", fileName)
		return fileName, nil
	}

	url := fmt.Sprintf("https://translate.google.com/translate_tts?ie=UTF-8&q=%s&tl=en&client=tw-ob", encodedWord)
	resp, err := http.Get(url)
	if err != nil {
		zap.S().Errorw("Failed to fetch audio", "word", word, "error", err)
		return "", err
	}
	defer resp.Body.Close()

	// Create the file to save the audio
	out, err := os.Create(fileName)
	if err != nil {
		zap.S().Errorw("Failed to create file", "file", fileName, "error", err)
		return "", err
	}
	defer out.Close()

	// Save the audio content into the file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		zap.S().Errorw("Failed to save audio file", "file", fileName, "error", err)
		return "", err
	}

	zap.S().Infow("Downloaded audio file", "file", fileName)
	return fileName, nil
}

// playAudio plays the audio file using the appropriate program based on the operating system
func playAudio(fileName string) error {
	var cmd *exec.Cmd

	// Check the operating system and select the appropriate audio player
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("powershell", "-c", fmt.Sprintf("Start-Process wmplayer -ArgumentList '%s' -NoNewWindow", fileName))
	case "darwin": // macOS
		cmd = exec.Command("afplay", fileName)
	case "linux":
		if _, err := exec.LookPath("mpg123"); err == nil {
			cmd = exec.Command("mpg123", fileName)
		} else if _, err := exec.LookPath("ffplay"); err == nil {
			cmd = exec.Command("ffplay", "-nodisp", "-autoexit", fileName)
		} else if _, err := exec.LookPath("aplay"); err == nil {
			cmd = exec.Command("aplay", fileName)
		} else {
			zap.S().Error("No suitable audio player found")
			return fmt.Errorf("no suitable audio player found")
		}
	default:
		zap.S().Errorw("Unsupported OS", "os", runtime.GOOS)
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	err := cmd.Run()
	if err != nil {
		zap.S().Errorw("Error playing audio", "file", fileName, "error", err)
		return err
	}

	zap.S().Infow("Audio played successfully", "file", fileName)
	return nil
}
