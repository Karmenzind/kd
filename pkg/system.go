package pkg

import (
	"bufio"
	"os"
	"strings"
)

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
