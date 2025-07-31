package pkg

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

type OSInfo struct {
	OS            string
	Distro        string
	IsDebianBased bool
}

// Deprecated: use GetOSInfo
func GetLinuxDistro() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			splited := strings.SplitN(line, "=", 2)
			return splited[1]
		}
	}
	return ""
}

func GetOSInfo() (*OSInfo, error) {
	var ret = &OSInfo{OS: runtime.GOOS}
	if ret.OS != "linux" {
		return ret, nil
	}

	// Open the os-release file
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return ret, err
	}
	defer file.Close()

	// Read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check for ID or ID_LIKE that indicates Debian
		if strings.HasPrefix(line, "ID=") || strings.HasPrefix(line, "ID_LIKE=") {
			if strings.HasPrefix(line, "ID=") {
				splited := strings.SplitN(line, "=", 2)
				ret.Distro = splited[1]
			}

			if strings.Contains(line, "debian") {
				ret.IsDebianBased = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return ret, err
	}

	return ret, nil
}

func GetCurUsername() string {
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("LOGNAME")
	}
	return username
}
