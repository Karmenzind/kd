package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/term"
)

type Phase string

const (
	PhaseStarting   Phase = "starting"
	PhaseLocal      Phase = "local"
	PhaseRemote     Phase = "remote"
	PhaseFormatting Phase = "format"
)

type State struct {
	Query string
	Phase Phase
	Text  string
}

type Progress interface {
	Start(State)
	Update(State)
	Stop()
}

type Options struct {
	Writer   io.Writer
	Enabled  bool
	Terminal bool
	ANSI     bool
	Unicode  bool
	Delay    time.Duration
	Interval time.Duration
}

type nopProgress struct{}

func (nopProgress) Start(State)  {}
func (nopProgress) Update(State) {}
func (nopProgress) Stop()        {}

func NopProgress() Progress {
	return nopProgress{}
}

func NewProgress(parent context.Context, options Options) Progress {
	if !options.Enabled || !options.Terminal || options.Writer == nil {
		return NopProgress()
	}
	if parent == nil {
		parent = context.Background()
	}
	if options.Delay <= 0 {
		options.Delay = 150 * time.Millisecond
	}
	if options.Interval <= 0 {
		options.Interval = 90 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(parent)
	return &dynamicProgress{
		ctx:      ctx,
		cancel:   cancel,
		options:  options,
		finished: make(chan struct{}),
	}
}

func IsTerminal(file *os.File) bool {
	return file != nil && term.IsTerminal(int(file.Fd()))
}

func TerminalCapabilities() (ansi, unicodeOutput bool) {
	return terminalCapabilities(runtime.GOOS, os.Getenv)
}

func terminalCapabilities(goos string, getenv func(string) string) (ansi, unicodeOutput bool) {
	if strings.EqualFold(getenv("TERM"), "dumb") {
		return false, false
	}
	if getenv("NO_COLOR") != "" {
		return false, goos != "windows"
	}
	if goos != "windows" {
		return true, true
	}
	modernTerminal := getenv("WT_SESSION") != "" ||
		getenv("TERM_PROGRAM") != "" ||
		getenv("ANSICON") != "" ||
		strings.EqualFold(getenv("ConEmuANSI"), "ON") ||
		strings.Contains(strings.ToLower(getenv("TERM")), "xterm")
	return modernTerminal, modernTerminal
}

type dynamicProgress struct {
	ctx     context.Context
	cancel  context.CancelFunc
	options Options

	mu      sync.Mutex
	state   State
	started bool
	stopped bool

	stopOnce sync.Once
	finished chan struct{}
}

func (p *dynamicProgress) Start(state State) {
	p.mu.Lock()
	if p.started || p.stopped {
		p.mu.Unlock()
		return
	}
	p.state = state
	p.started = true
	p.mu.Unlock()
	go p.run()
}

func (p *dynamicProgress) Update(state State) {
	p.mu.Lock()
	if !p.stopped {
		p.state = state
	}
	p.mu.Unlock()
}

func (p *dynamicProgress) Stop() {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.stopped = true
		started := p.started
		p.mu.Unlock()
		p.cancel()
		if !started {
			close(p.finished)
		}
	})
	<-p.finished
}

func (p *dynamicProgress) run() {
	defer close(p.finished)
	timer := time.NewTimer(p.options.Delay)
	defer timer.Stop()
	select {
	case <-p.ctx.Done():
		return
	case <-timer.C:
	}

	ticker := time.NewTicker(p.options.Interval)
	defer ticker.Stop()
	displayed := false
	lastWidth := 0
	frame := 0
	for {
		state := p.currentState()
		text, width := renderFrame(state, frame, p.options.ANSI, p.options.Unicode)
		if err := p.writeFrame(text, width, lastWidth); err != nil {
			return
		}
		displayed = true
		lastWidth = width
		frame++
		select {
		case <-p.ctx.Done():
			if displayed {
				_ = p.clearLine(lastWidth)
			}
			return
		case <-ticker.C:
		}
	}
}

func (p *dynamicProgress) currentState() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *dynamicProgress) writeFrame(frame string, width, previousWidth int) error {
	if p.options.ANSI {
		_, err := fmt.Fprintf(p.options.Writer, "\r\x1b[2K%s", frame)
		return err
	}
	padding := previousWidth - width
	if padding < 0 {
		padding = 0
	}
	_, err := fmt.Fprintf(p.options.Writer, "\r%s%s", frame, strings.Repeat(" ", padding))
	return err
}

func (p *dynamicProgress) clearLine(previousWidth int) error {
	if p.options.ANSI {
		_, err := io.WriteString(p.options.Writer, "\r\x1b[2K")
		return err
	}
	_, err := fmt.Fprintf(p.options.Writer, "\r%s\r", strings.Repeat(" ", previousWidth))
	return err
}

const unicodeSpinner = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
const asciiSpinner = "|/-\\"

func renderFrame(state State, frame int, ansi, unicodeOutput bool) (string, int) {
	query := displayQuery(state.Query)
	phase := phaseText(state)
	frames := []rune(asciiSpinner)
	if unicodeOutput {
		frames = []rune(unicodeSpinner)
	}
	marker := string(frames[frame%len(frames)])
	plain := marker + " " + query
	if phase != "" {
		plain += " · " + phase
	}
	if !ansi {
		return plain, displayWidth(plain)
	}

	runes := []rune(query)
	highlight := -1
	if len(runes) > 0 {
		highlight = frame % len(runes)
	}
	var styled strings.Builder
	styled.WriteString("\x1b[36m")
	styled.WriteString(marker)
	styled.WriteString("\x1b[0m ")
	for i, r := range runes {
		if i == highlight {
			styled.WriteString("\x1b[96;1m")
			styled.WriteRune(r)
			styled.WriteString("\x1b[0m")
		} else {
			styled.WriteRune(r)
		}
	}
	if phase != "" {
		styled.WriteString(" \x1b[2m· ")
		styled.WriteString(phase)
		styled.WriteString("\x1b[0m")
	}
	styled.WriteString("\x1b[0m")
	return styled.String(), displayWidth(plain)
}

func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		switch {
		case unicode.IsControl(r) || unicode.Is(unicode.Mn, r):
			continue
		case r >= 0x1100:
			width += 2
		default:
			width++
		}
	}
	return width
}

func displayQuery(query string) string {
	runes := []rune(strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, query)))
	const maxRunes = 48
	if len(runes) > maxRunes {
		runes = append(runes[:maxRunes-1], '…')
	}
	return string(runes)
}

func phaseText(state State) string {
	if state.Text != "" {
		return state.Text
	}
	switch state.Phase {
	case PhaseStarting:
		return "启动"
	case PhaseLocal:
		return "本地"
	case PhaseRemote:
		return "在线"
	case PhaseFormatting:
		return "排版"
	default:
		return string(state.Phase)
	}
}
