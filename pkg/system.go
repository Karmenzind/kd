package pkg

import (
	"bufio"
	"os"
	"strings"
)

type OSInfo struct {
	Distro        string
	IsDebianBased bool
}

func GetLinuxDistro() string {
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
	var ret = &OSInfo{}
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
