package pkg

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestOutputResultInvalidPagerCommand(t *testing.T) {
	stderr, errW := captureStderr(t, func() {
		err := OutputResult("hello", true, `less 'unclosed`)
		if err != nil {
			t.Fatalf("OutputResult() error = %v, want nil", err)
		}
	})
	if errW != nil {
		t.Fatal(errW)
	}
	if !strings.Contains(stderr, "pager command `less 'unclosed` is invalid:") {
		t.Fatalf("stderr = %q, want invalid pager warning", stderr)
	}
	if !strings.Contains(stderr, "unclosed quote") {
		t.Fatalf("stderr = %q, want unclosed quote in warning", stderr)
	}

	stdout, errW := captureStdout(t, func() {
		err := OutputResult("direct output", true, `less \`)
		if err != nil {
			t.Fatalf("OutputResult() error = %v, want nil", err)
		}
	})
	if errW != nil {
		t.Fatal(errW)
	}
	if stdout != "direct output\n" {
		t.Fatalf("stdout = %q, want direct output", stdout)
	}
}

func captureStderr(t *testing.T, fn func()) (string, error) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return "", err
	}
	r.Close()
	return buf.String(), nil
}

func captureStdout(t *testing.T, fn func()) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return "", err
	}
	r.Close()
	return buf.String(), nil
}
