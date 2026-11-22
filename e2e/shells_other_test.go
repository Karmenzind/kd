//go:build e2e && !windows

package e2e

import "os"

// openTerminal opens the terminal the tests were started from.
func openTerminal() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_WRONLY, 0)
}

func candidateShells() []*shell {
	return []*shell{
		posixShell("bash", lookPath("bash")),
		posixShell("zsh", lookPath("zsh")),
		fishShell(lookPath("fish")),
		powerShell("pwsh", lookPath("pwsh")),
	}
}
