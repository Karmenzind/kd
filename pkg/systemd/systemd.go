package systemd

import (
	"bytes"
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
	zap.S().Infof("Executed: `%s` Result: `%s` Error: %v", cmd, output, err)
	return outputStr, err
}

func ServiceIsActive(unit string, isUser bool) (bool, error) {
	out, err := systemdAction(unit, "is-active", isUser)
	return out == "active", err
}

func ServiceIsEnabled(unit string, isUser bool) (bool, error) {
	out, err := systemdAction(unit, "is-enabled", isUser)
	return out == "enabled", err
}

func ServiceIsActiveOrEnabled(unit string, isUser bool) bool {
	out1, _ := systemdAction(unit, "is-active", isUser)
	out2, _ := systemdAction(unit, "is-enabled", isUser)
	return out1 == "active" || out2 == "enabled"
}

func UnitExists(unitName string, isUser bool) (bool, error) {
	var cmd *exec.Cmd
	if isUser {
		cmd = exec.Command("systemctl", "list-unit-files", "--full", "--no-pager", "--user")
	} else {
		cmd = exec.Command("systemctl", "list-unit-files", "--full", "--no-pager")
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
        zap.S().Warnf("Failed to fetch unit-files. Error: %s", err)
		return false, err
	}
	output := out.String()
    
    zap.S().Debugf("Fetched unit-files: %s", output)
	return strings.Contains(output, unitName), err
}

func StopService(unit string, isUser bool) (string, error) {
	return systemdAction(unit, "stop", isUser)
}

func StartService(unit string, isUser bool) (string, error) {
	return systemdAction(unit, "start", isUser)
}

func RestartService(unit string, isUser bool) (string, error) {
	return systemdAction(unit, "restart", isUser)
}
