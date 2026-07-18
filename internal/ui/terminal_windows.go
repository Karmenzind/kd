//go:build windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

var (
	kernel32Console = syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode  = kernel32Console.NewProc("GetConsoleMode")
	setConsoleMode  = kernel32Console.NewProc("SetConsoleMode")
)

func probeVirtualTerminal(file *os.File) vtProbe {
	if file == nil {
		return vtProbe{}
	}

	var mode uint32
	ok, _, _ := getConsoleMode.Call(file.Fd(), uintptr(unsafe.Pointer(&mode)))
	if ok == 0 {
		return vtProbe{}
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return vtProbe{Known: true, Enabled: true}
	}

	ok, _, _ = setConsoleMode.Call(file.Fd(), uintptr(mode|enableVirtualTerminalProcessing))
	return vtProbe{Known: true, Enabled: ok != 0}
}
