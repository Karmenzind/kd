package pkg

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

func IsPortOpen(port int) bool {
	if runtime.GOOS == "windows" {
		return !IsPortInUseOnWindows(port)
	}
	address := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	defer listener.Close()
	return true
}

func IsPortInUseOnWindows(port int) bool {
	output, err := exec.Command("netstat", "-ano").CombinedOutput()
	if err != nil {
		fmt.Println("Error running netstat command:", err)
		return false
	}

	outputString := string(output)
	return strings.Contains(outputString, fmt.Sprintf(":%d", port))
}
