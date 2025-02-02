package tts

import (
	"os/exec"
	"runtime"
)

var presetSpeaker string

// find speaker program
// return the first available program and check list
func CheckSpeaker() (string, []string) {
	var checkList []string
	switch runtime.GOOS {
	case "linux":
		checkList = []string{"mpg123", "ffplay", "aplay"}
	case "darwin":
		checkList = []string{"aplay"}
	case "windows":
        // TODO (k): <2025-02-02 20:47> 
	}

	for _, p := range []string{"mpg123", "ffplay", "aplay"} {
		if _, err := exec.LookPath("mpg123"); err == nil {
			presetSpeaker = p
			break
		}
	}
	return presetSpeaker, checkList
}
