package systemd

import (
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

func systemdAction(unit string, subcommand string, isUser bool) (string, error) {
	var cmd *exec.Cmd
	if isUser {
		cmd = exec.Command("systemctl", subcommand, "--user", unit)
	} else {
		cmd = exec.Command("systemctl", subcommand, unit)
	}
	output, err := cmd.CombinedOutput()
	outputStr := strings.Trim(string(output), "\n")
	zap.S().Infof("Executed: `%s` Result: `%s` Error: %v", cmd, outputStr, err)
	return outputStr, err
}

func ServiceIsActive(unit string, isUser bool) (bool, error) {
	out, err := systemdAction(unit, "is-active", isUser)
	if out == "active" || out == "inactive" {
		err = nil
	}
	return out == "active", err
}

func StopService(unit string, isUser bool) (string, error) {
	return systemdAction(unit, "stop", isUser)
}

func RestartService(unit string, isUser bool) (string, error) {
	return systemdAction(unit, "restart", isUser)
}
