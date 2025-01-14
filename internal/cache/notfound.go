package cache

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

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
	p := getNotFoundFilePath()
	if !pkg.IsPathExists(p) {
		return nil
	}
	f, err := os.Open(getNotFoundFilePath())
	if err != nil {
		return err
	}
	newfile := p + ".new"
	defer func() {
		f.Close()
		if pkg.IsPathExists(newfile) {
			os.Rename(newfile, p)
			zap.S().Infof("Replaced not found list. Removed: %s", query)
		}
	}()

	scanner := bufio.NewScanner(f)

	newLines := []string{}
	// https://golang.org/pkg/bufio/#Scanner.Scan
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == query {
			continue
		}
		newLines = append(newLines, line)
	}

	if err = scanner.Err(); err != nil {
		return err
		// Handle the error
	}

	content := strings.Join(newLines, "\n")
	err = os.WriteFile(newfile, []byte(content), os.ModePerm)
	zap.S().Debugf("Updated json file %s. Error: %v. BodyLength: %d", newfile, err, len(content))
	if err != nil {
		return err
	}

	return nil
}
