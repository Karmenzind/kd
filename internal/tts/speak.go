package tts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"go.uber.org/zap"
)

var speakerProgram string

// TODO (k): <2025-02-02 20:23> cached files' loc
func buildTargetPath(word string) string {
	// TODO (K): <2025-06-09 22:41> and dir
	fileName := strings.ReplaceAll(word, " ", "_") + ".mp3"
	return filepath.Join(run.CACHE_AUDIO_DIR_PATH, fileName)
}

// downloadAudio checks if the audio file already exists locally, and if not, downloads it
func downloadAudio(word, filePath string) error {
	var err error
	if _, err = os.Stat(filePath); err == nil {
		zap.S().Debugw("Audio file already exists", "file", filePath)
		return err
	}

	url := buildAudioUrl(word)
	if err = pkg.DownloadFileWithTimeout(filePath, url, 5*time.Second); err != nil {
		zap.S().Warnf("Failed to download audio file: %s", err)
		return err
	}
	zap.S().Infow("Downloaded audio file", "file", filePath)
	return nil
}

// find speaker program
// return the first available program and check list
func checkSpeaker() string {
	var checkList []string
	switch runtime.GOOS {
	case "linux":
		checkList = []string{"mpv", "mpg123", "ffplay", "aplay"}
	case "darwin":
		checkList = []string{"mpv", "ffplay", "afplay"}
	case "windows":
		checkList = []string{"mpv", "ffplay"}
	}

	for _, p := range checkList {
		if _, err := exec.LookPath(p); err == nil {
			speakerProgram = p
			break
		}
	}
	if speakerProgram == "" && runtime.GOOS == "windows" {
		speakerProgram = "system"
		d.EchoWarn("推荐安装mpv或ffmpeg支持无弹窗播放声音")
	}
	if speakerProgram != "" {
		zap.S().Debugf("Will use %q as the speaker program", speakerProgram)
	}
	return speakerProgram
}

// playAudio plays the audio file using the appropriate program based on the operating system
func playAudio(filePath string) error {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		switch speakerProgram {
		case "mpv":
			cmd = exec.Command("mpv", "--no-terminal", filePath)
		case "ffplay":
			cmd = exec.Command("ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", filePath)
		case "system":
			cmd = exec.Command("powershell", "-c", fmt.Sprintf("start '%s'", filePath))
		}
	} else {
		switch speakerProgram {
		case "mpv":
			cmd = exec.Command("mpv", "--really-quiet", filePath)
		case "afplay", "mpg123", "aplay":
			cmd = exec.Command(speakerProgram, filePath)
		case "ffplay":
			cmd = exec.Command("ffplay", "-nodisp", "-autoexit", filePath)
		}
	}
	if cmd == nil {
		return fmt.Errorf("no suitable audio player found")
	}

	if output, err := cmd.CombinedOutput(); err != nil {
		zap.S().Errorf("Error playing audio %s Error: %s Output: %s", filePath, err, string(output))
		return err
	} else {
		zap.S().Infow("Audio played successfully", "file", filePath, "output", output)
	}
	return nil
}

// API
func Speak(word string) error {
	var err error
	checkSpeaker()
	if speakerProgram == "" {
		return fmt.Errorf("no speaker found")
	}
	p := buildTargetPath(word)
	if err = downloadAudio(word, p); err != nil {
		return err
	}
	return playAudio(p)
}
