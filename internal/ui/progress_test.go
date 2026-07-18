package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

var (
	testBasicCapabilities    = TerminalCapabilities{Level: CapabilityBasic}
	testEnhancedCapabilities = TerminalCapabilities{Level: CapabilityEnhanced, ANSI: true, Unicode: true}
)

type observedWriter struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	writes chan struct{}
}

func newObservedWriter() *observedWriter {
	return &observedWriter{writes: make(chan struct{}, 128)}
}

func (w *observedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buffer.Write(p)
	w.mu.Unlock()
	select {
	case w.writes <- struct{}{}:
	default:
	}
	return n, err
}

func (w *observedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

func waitForWrite(t *testing.T, writer *observedWriter) {
	t.Helper()
	select {
	case <-writer.writes:
	case <-time.After(time.Second):
		t.Fatal("progress did not write a frame")
	}
}

func TestNewProgressDisabled(t *testing.T) {
	for _, tt := range []struct {
		name    string
		enabled bool
		caps    TerminalCapabilities
	}{
		{name: "explicitly disabled", enabled: false, caps: testBasicCapabilities},
		{name: "plain capability", enabled: true, caps: TerminalCapabilities{Level: CapabilityPlain}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			progress := NewProgress(context.Background(), Options{
				Writer:       &output,
				Enabled:      tt.enabled,
				Capabilities: tt.caps,
			})
			progress.Update(State{Query: "before", Phase: PhaseLocal})
			progress.Start(State{Query: "query", Phase: PhaseRemote})
			progress.Stop()
			progress.Stop()
			if output.Len() != 0 {
				t.Fatalf("disabled progress output = %q", output.String())
			}
		})
	}
}

func TestIsTerminalRejectsRegularFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stderr-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if IsTerminal(file) {
		t.Fatal("IsTerminal() = true for a regular file")
	}
	if got := DetectCapabilities(file); got.Level != CapabilityPlain {
		t.Fatalf("DetectCapabilities(regular file) = %+v, want Plain", got)
	}
}

func TestProgressFastCompletionHasNoOutput(t *testing.T) {
	var output bytes.Buffer
	progress := NewProgress(context.Background(), Options{
		Writer:       &output,
		Enabled:      true,
		Capabilities: testBasicCapabilities,
		Delay:        100 * time.Millisecond,
		Interval:     10 * time.Millisecond,
	})
	progress.Start(State{Query: "fast", Phase: PhaseLocal})
	progress.Stop()
	if output.Len() != 0 {
		t.Fatalf("fast progress output = %q, want empty", output.String())
	}
}

func TestProgressStopBeforeStartIsSafe(t *testing.T) {
	var output bytes.Buffer
	progress := NewProgress(context.Background(), Options{
		Writer:       &output,
		Enabled:      true,
		Capabilities: testBasicCapabilities,
	})
	progress.Update(State{Query: "before", Phase: PhaseLocal})
	progress.Stop()
	progress.Start(State{Query: "after", Phase: PhaseRemote})
	progress.Update(State{Query: "ignored", Phase: PhaseFormatting})
	progress.Stop()
	if output.Len() != 0 {
		t.Fatalf("stopped progress output = %q", output.String())
	}
}

func TestProgressDelayedRenderUpdateAndStop(t *testing.T) {
	writer := newObservedWriter()
	progress := NewProgress(context.Background(), Options{
		Writer:       writer,
		Enabled:      true,
		Capabilities: testEnhancedCapabilities,
		Delay:        5 * time.Millisecond,
		Interval:     5 * time.Millisecond,
	})
	progress.Start(State{Query: "词典", Phase: PhaseLocal})
	waitForWrite(t, writer)
	progress.Update(State{Query: "词典", Phase: PhaseRemote})
	deadline := time.After(time.Second)
	for !strings.Contains(writer.String(), "在线") {
		select {
		case <-writer.writes:
		case <-deadline:
			t.Fatalf("updated progress output = %q, missing remote phase", writer.String())
		}
	}
	progress.Stop()
	progress.Stop()
	outputAfterStop := writer.String()
	if !strings.Contains(outputAfterStop, "\r\x1b[2K") || !strings.Contains(outputAfterStop, "\x1b[0m") {
		t.Fatalf("progress output = %q, missing clear/reset sequences", outputAfterStop)
	}
	select {
	case <-time.After(30 * time.Millisecond):
	}
	if got := writer.String(); got != outputAfterStop {
		t.Fatalf("progress wrote after Stop(): before=%q after=%q", outputAfterStop, got)
	}
}

func TestProgressDoesNotPolluteResultWriter(t *testing.T) {
	status := newObservedWriter()
	var result bytes.Buffer
	progress := NewProgress(context.Background(), Options{
		Writer:       status,
		Enabled:      true,
		Capabilities: testEnhancedCapabilities,
		Delay:        time.Millisecond,
		Interval:     5 * time.Millisecond,
	})
	progress.Start(State{Query: "separate", Phase: PhaseRemote})
	waitForWrite(t, status)
	progress.Stop()
	fmt.Fprintln(&result, "final result")
	if got := result.String(); got != "final result\n" {
		t.Fatalf("result output = %q", got)
	}
	if strings.Contains(status.String(), "final result") {
		t.Fatalf("status output contains final result: %q", status.String())
	}
}

func TestProgressContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	writer := newObservedWriter()
	progress := NewProgress(ctx, Options{
		Writer:       writer,
		Enabled:      true,
		Capabilities: testBasicCapabilities,
		Delay:        time.Millisecond,
		Interval:     5 * time.Millisecond,
	})
	progress.Start(State{Query: "cancel", Phase: PhaseRemote})
	waitForWrite(t, writer)
	cancel()
	progress.Stop()
	if !strings.HasSuffix(writer.String(), "\r") {
		t.Fatalf("non-ANSI cancellation did not clear line: %q", writer.String())
	}
}

func TestNonANSIRefreshPadsPreviousFrame(t *testing.T) {
	var output bytes.Buffer
	progress := &dynamicProgress{options: Options{Writer: &output, Capabilities: testBasicCapabilities}}
	if err := progress.writeFrame("long status", 11, 0); err != nil {
		t.Fatal(err)
	}
	if err := progress.writeFrame("short", 5, 11); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\rshort"+strings.Repeat(" ", 6)) {
		t.Fatalf("non-ANSI refresh output = %q", output.String())
	}
}

func TestProgressConcurrentUpdateAndStop(t *testing.T) {
	writer := newObservedWriter()
	progress := NewProgress(context.Background(), Options{
		Writer:       writer,
		Enabled:      true,
		Capabilities: testBasicCapabilities,
		Delay:        time.Millisecond,
		Interval:     5 * time.Millisecond,
	})
	progress.Start(State{Query: "race", Phase: PhaseLocal})
	waitForWrite(t, writer)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			progress.Update(State{Query: "race", Phase: Phase(string(rune('a' + i%20)))})
		}()
	}
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			progress.Stop()
		}()
	}
	wg.Wait()
	progress.Update(State{Query: "ignored", Phase: PhaseRemote})
}

type failingWriter struct {
	wrote chan struct{}
}

func (w *failingWriter) Write([]byte) (int, error) {
	select {
	case w.wrote <- struct{}{}:
	default:
	}
	return 0, errors.New("write failed")
}

func TestProgressWriterErrorStopsRenderer(t *testing.T) {
	writer := &failingWriter{wrote: make(chan struct{}, 1)}
	progress := NewProgress(context.Background(), Options{
		Writer:       writer,
		Enabled:      true,
		Capabilities: testBasicCapabilities,
		Delay:        time.Millisecond,
		Interval:     5 * time.Millisecond,
	})
	progress.Start(State{Query: "failure", Phase: PhaseRemote})
	select {
	case <-writer.wrote:
	case <-time.After(time.Second):
		t.Fatal("renderer did not attempt a write")
	}
	progress.Stop()
}

func TestRenderFrameFallbacksAndUnicode(t *testing.T) {
	tests := []struct {
		name    string
		state   State
		caps    TerminalCapabilities
		want    []string
		notWant []string
	}{
		{
			name:    "ASCII fallback",
			state:   State{Query: "word", Phase: PhaseRemote},
			caps:    testBasicCapabilities,
			want:    []string{"| word · 在线"},
			notWant: []string{"\x1b[", "⠋"},
		},
		{
			name:  "Unicode shimmer",
			state: State{Query: "词典", Phase: PhaseFormatting},
			caps:  testEnhancedCapabilities,
			want:  []string{"词", "典", "排版", "\x1b[96;1m", "\x1b[0m", "⠋"},
		},
		{
			name:  "empty query",
			state: State{Phase: PhaseStarting},
			caps:  testEnhancedCapabilities,
			want:  []string{"启动"},
		},
		{
			name:  "unknown phase",
			state: State{Query: "word", Phase: Phase("custom")},
			caps:  testEnhancedCapabilities,
			want:  []string{"custom"},
		},
		{
			name:    "Basic wide query has no sweep",
			state:   State{Query: "中文查询", Phase: PhaseRemote},
			caps:    testBasicCapabilities,
			want:    []string{"| 中文查询 · 在线"},
			notWant: []string{"\x1b[", "⠋"},
		},
		{
			name:    "Enhanced without color keeps Unicode spinner",
			state:   State{Query: "word", Phase: PhaseRemote},
			caps:    TerminalCapabilities{Level: CapabilityEnhanced, Unicode: true},
			want:    []string{"⠋ word · 在线"},
			notWant: []string{"\x1b["},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, width := renderFrame(tt.state, 0, tt.caps)
			if !utf8.ValidString(frame) || width < 0 {
				t.Fatalf("renderFrame() = %q, width %d", frame, width)
			}
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Fatalf("renderFrame() = %q, missing %q", frame, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(frame, notWant) {
					t.Fatalf("renderFrame() = %q, contains %q", frame, notWant)
				}
			}
		})
	}
}

func TestDisplayQuerySanitizesAndTruncates(t *testing.T) {
	query := strings.Repeat("界", 60) + "\nnext"
	got := displayQuery(query)
	if strings.ContainsAny(got, "\r\n\t") {
		t.Fatalf("displayQuery() = %q, contains control whitespace", got)
	}
	if runes := []rune(got); len(runes) != 48 || runes[len(runes)-1] != '…' {
		t.Fatalf("displayQuery() rune count = %d, value = %q", len(runes), got)
	}
}

func TestTerminalCapabilities(t *testing.T) {
	tests := []struct {
		name  string
		input capabilityInput
		env   map[string]string
		want  TerminalCapabilities
	}{
		{name: "non TTY", input: capabilityInput{GOOS: "linux"}, want: TerminalCapabilities{Level: CapabilityPlain}},
		{name: "dumb terminal", input: capabilityInput{GOOS: "linux", Terminal: true}, env: map[string]string{"TERM": "dumb"}, want: TerminalCapabilities{Level: CapabilityPlain}},
		{name: "Unix missing TERM", input: capabilityInput{GOOS: "linux", Terminal: true}, want: testBasicCapabilities},
		{name: "Unix unknown terminal", input: capabilityInput{GOOS: "linux", Terminal: true}, env: map[string]string{"TERM": "unknown", "LANG": "en_US.UTF-8"}, want: testBasicCapabilities},
		{name: "Unix Unicode unclear", input: capabilityInput{GOOS: "linux", Terminal: true}, env: map[string]string{"TERM": "xterm-256color"}, want: testBasicCapabilities},
		{name: "Unix modern terminal", input: capabilityInput{GOOS: "linux", Terminal: true}, env: map[string]string{"TERM": "xterm-256color", "LANG": "en_US.UTF-8"}, want: testEnhancedCapabilities},
		{name: "Unix NO_COLOR", input: capabilityInput{GOOS: "darwin", Terminal: true}, env: map[string]string{"TERM_PROGRAM": "Apple_Terminal", "LANG": "en_US.UTF-8", "NO_COLOR": "1"}, want: TerminalCapabilities{Level: CapabilityEnhanced, Unicode: true}},
		{name: "CI is conservative", input: capabilityInput{GOOS: "linux", Terminal: true}, env: map[string]string{"TERM": "xterm-256color", "LANG": "en_US.UTF-8", "CI": "1"}, want: testBasicCapabilities},
		{name: "Windows capability unknown", input: capabilityInput{GOOS: "windows", Terminal: true}, want: testBasicCapabilities},
		{name: "Windows VT unavailable", input: capabilityInput{GOOS: "windows", Terminal: true, VTKnown: true}, env: map[string]string{"WT_SESSION": "1"}, want: testBasicCapabilities},
		{name: "Windows Terminal", input: capabilityInput{GOOS: "windows", Terminal: true, VTKnown: true, VTEnabled: true}, env: map[string]string{"WT_SESSION": "1"}, want: testEnhancedCapabilities},
		{name: "Windows xterm with VT", input: capabilityInput{GOOS: "windows", Terminal: true, VTKnown: true, VTEnabled: true}, env: map[string]string{"TERM": "xterm-256color"}, want: testEnhancedCapabilities},
		{name: "cmd style environment", input: capabilityInput{GOOS: "windows", Terminal: true, VTKnown: true, VTEnabled: true}, env: map[string]string{"ComSpec": `C:\Windows\System32\cmd.exe`}, want: testBasicCapabilities},
		{name: "PowerShell version is ignored", input: capabilityInput{GOOS: "windows", Terminal: true, VTKnown: true, VTEnabled: true}, env: map[string]string{"PSModulePath": "PowerShell\\7"}, want: testBasicCapabilities},
		{name: "ANSICON alone is conservative", input: capabilityInput{GOOS: "windows", Terminal: true, VTKnown: true, VTEnabled: true}, env: map[string]string{"ANSICON": "1"}, want: testBasicCapabilities},
		{name: "Windows Terminal NO_COLOR", input: capabilityInput{GOOS: "windows", Terminal: true, VTKnown: true, VTEnabled: true}, env: map[string]string{"WT_SESSION": "1", "NO_COLOR": "1"}, want: TerminalCapabilities{Level: CapabilityEnhanced, Unicode: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			tt.input.Getenv = getenv
			got := detectCapabilities(tt.input)
			if got != tt.want {
				t.Fatalf("detectCapabilities() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNopProgressWriterContract(t *testing.T) {
	progress := NopProgress()
	progress.Start(State{})
	progress.Update(State{})
	progress.Stop()
	if _, ok := any(progress).(io.Writer); ok {
		t.Fatal("Progress unexpectedly exposes writer behavior")
	}
}
