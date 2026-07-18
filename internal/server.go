package internal

import "github.com/Karmenzind/kd/internal/daemon"

// StartServer preserves the package-level entry point used by the CLI while
// keeping daemon lifecycle and TCP serving code in internal/daemon.
func StartServer() error {
	return daemon.StartServer()
}
