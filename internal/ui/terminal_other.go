//go:build !windows

package ui

import "os"

func probeVirtualTerminal(*os.File) vtProbe {
	return vtProbe{}
}
