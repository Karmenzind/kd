package cache

import (
	"bufio"
	"os"
	"path/filepath"

	"github.com/Karmenzind/kd/pkg"
	"go.uber.org/zap"
)

func getNotFoundFilePath() string {
	return filepath.Join(CACHE_ROOT_PATH, "online_not_found")
}

func CheckNotFound(query string) (int, error) {
    p := getNotFoundFilePath()
    if !pkg.IsPathExists(p) {
        return 0, nil
    }

	f, err := os.Open(getNotFoundFilePath())
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// Splits on newlines by default.
	scanner := bufio.NewScanner(f)

	line := 1
	// https://golang.org/pkg/bufio/#Scanner.Scan
	for scanner.Scan() {
		if scanner.Text() == query {
			return line, nil
		}
		line++
	}

	if err := scanner.Err(); err != nil {
		return 0, err
		// Handle the error
	}
	return 0, nil
}

func AppendNotFound(query string) (err error) {
    p := getNotFoundFilePath()
	file, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.ModeAppend|os.ModePerm)

	if err != nil {
        zap.S().Warnf("Failed to open %s: %s", p, err)
		return
	}

	defer file.Close()

	_, err = file.WriteString(query + "\n")

	if err != nil {
        zap.S().Warnf("Failed to write to %s: %s", p, err)
		return
	}
	return
}

func RemoveNotFound(query string) error {
	return nil
}

