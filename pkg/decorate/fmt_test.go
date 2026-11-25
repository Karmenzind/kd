package decorate

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdIO(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stdout, os.Stderr = outW, errW
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origOut, origErr
		outW.Close()
		errW.Close()
	})

	done := make(chan struct{})
	go func() {
		var outBuf, errBuf bytes.Buffer
		_, _ = io.Copy(&outBuf, outR)
		_, _ = io.Copy(&errBuf, errR)
		stdout, stderr = outBuf.String(), errBuf.String()
		close(done)
	}()

	fn()
	outW.Close()
	errW.Close()
	<-done
	return stdout, stderr
}

func TestEchoFunctionsWriteToStreams(t *testing.T) {
	tests := []struct {
		name     string
		echo     func()
		wantStd  bool
		contains string
	}{
		{"warn", func() { EchoWarn("disk %s", "full") }, false, "WARNING"},
		{"error", func() { EchoError("failed") }, false, "ERROR"},
		{"wrong", func() { EchoWrong("nope") }, false, "nope"},
		{"run", func() { EchoRun("starting") }, true, "starting"},
		{"okay", func() { EchoOkay("done") }, true, "done"},
		{"fine", func() { EchoFine("nice") }, true, "nice"},
		{"weak", func() { EchoWeakNotice("hint") }, true, "hint"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := captureStdIO(t, tt.echo)
			stream := stderr
			if tt.wantStd {
				stream = stdout
			}
			if !strings.Contains(stream, tt.contains) {
				t.Fatalf("output = stdout(%q) stderr(%q), want %q in the chosen stream",
					stdout, stderr, tt.contains)
			}
		})
	}
}
