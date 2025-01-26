package pkg

import (
	"encoding/json"
	"errors"
	"os"

	"go.uber.org/zap"
)

func IsPathExists(p string) bool {
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

// marshal struct and write to file
func SaveJson(fpath string, v any) (err error) {
	j, err := json.Marshal(v)
	if err != nil {
		zap.S().Debugf("Failed to marshal: %+v %s", v, err)
	}
	err = os.WriteFile(fpath, j, os.ModePerm)
	zap.S().Debugf("Updated json file %s. Error: %v. BodyLength: %d", fpath, err, len(j))
	return
}

func LoadJson(fpath string, v any) (err error) {
	if _, err = os.Stat(fpath); errors.Is(err, os.ErrNotExist) {
		zap.S().Debugf("Json file for '%s' doesn't exist.", fpath)
		return err
	}
	j, err := os.ReadFile(fpath)
	if err != nil {
		zap.S().Debugf("Failed to read file for %s: %s", fpath, err)
		return err
	}
	if len(j) > 0 {
		if err = json.Unmarshal(j, v); err != nil {
			zap.S().Debugf("Failed to unmarshal : %s", fpath, err)
			return err
		}
	}
	return
}

func AddExecutablePermission(filePath string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	originalMode := fileInfo.Mode()
	newMode := originalMode | 0111
	if err := os.Chmod(filePath, newMode); err != nil {
		return err
	}
	zap.S().Infof("Executable permission added to %s", filePath)
	return nil
}
