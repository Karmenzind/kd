package ui

import (
	"os"
	"runtime"
	"strings"

	"golang.org/x/term"
)

type Capability uint8

const (
	CapabilityPlain Capability = iota
	CapabilityBasic
	CapabilityEnhanced
)

type TerminalCapabilities struct {
	Level   Capability
	ANSI    bool
	Unicode bool
}

type capabilityInput struct {
	GOOS      string
	Terminal  bool
	Getenv    func(string) string
	VTKnown   bool
	VTEnabled bool
}

type vtProbe struct {
	Known   bool
	Enabled bool
}

func IsTerminal(file *os.File) bool {
	return file != nil && term.IsTerminal(int(file.Fd()))
}

func DetectCapabilities(file *os.File) TerminalCapabilities {
	probe := probeVirtualTerminal(file)
	return detectCapabilities(capabilityInput{
		GOOS:      runtime.GOOS,
		Terminal:  IsTerminal(file),
		Getenv:    os.Getenv,
		VTKnown:   probe.Known,
		VTEnabled: probe.Enabled,
	})
}

func detectCapabilities(input capabilityInput) TerminalCapabilities {
	getenv := input.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if !input.Terminal || strings.EqualFold(getenv("TERM"), "dumb") {
		return TerminalCapabilities{Level: CapabilityPlain}
	}
	if getenv("CI") != "" {
		return basicCapabilities()
	}

	if input.GOOS == "windows" {
		return detectWindowsCapabilities(input, getenv)
	}
	return detectUnixCapabilities(getenv)
}

func detectWindowsCapabilities(input capabilityInput, getenv func(string) string) TerminalCapabilities {
	if !input.VTKnown || !input.VTEnabled {
		return basicCapabilities()
	}

	term := strings.ToLower(getenv("TERM"))
	termProgram := strings.ToLower(getenv("TERM_PROGRAM"))
	modernTerminal := getenv("WT_SESSION") != "" ||
		strings.Contains(term, "xterm") ||
		strings.Contains(term, "screen") ||
		strings.Contains(term, "tmux") ||
		strings.Contains(termProgram, "windows terminal") ||
		strings.Contains(termProgram, "wezterm") ||
		strings.Contains(termProgram, "vscode")
	if !modernTerminal {
		return basicCapabilities()
	}

	return enhancedCapabilities(getenv("NO_COLOR") == "")
}

func detectUnixCapabilities(getenv func(string) string) TerminalCapabilities {
	term := strings.ToLower(getenv("TERM"))
	termProgram := strings.ToLower(getenv("TERM_PROGRAM"))
	modernTerminal := strings.Contains(term, "xterm") ||
		strings.Contains(term, "screen") ||
		strings.Contains(term, "tmux") ||
		strings.Contains(term, "rxvt") ||
		strings.Contains(term, "alacritty") ||
		strings.Contains(term, "kitty") ||
		strings.Contains(term, "wezterm") ||
		strings.Contains(term, "foot") ||
		strings.Contains(termProgram, "apple_terminal") ||
		strings.Contains(termProgram, "iterm") ||
		strings.Contains(termProgram, "wezterm") ||
		strings.Contains(termProgram, "vscode")
	if !modernTerminal || !unicodeLocale(getenv) {
		return basicCapabilities()
	}
	return enhancedCapabilities(getenv("NO_COLOR") == "")
}

func unicodeLocale(getenv func(string) string) bool {
	locale := getenv("LC_ALL")
	if locale == "" {
		locale = getenv("LC_CTYPE")
	}
	if locale == "" {
		locale = getenv("LANG")
	}
	locale = strings.ToLower(locale)
	return strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8")
}

func basicCapabilities() TerminalCapabilities {
	return TerminalCapabilities{Level: CapabilityBasic}
}

func enhancedCapabilities(color bool) TerminalCapabilities {
	return TerminalCapabilities{
		Level:   CapabilityEnhanced,
		ANSI:    color,
		Unicode: true,
	}
}
