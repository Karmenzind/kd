package main

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

type peerClosedCase struct {
	name          string
	err           error
	wantReset     bool
	wantWrite     bool
	wantEmptyRead bool
}

func testPeerClosed(t *testing.T, cases []peerClosedCase) {
	t.Helper()
	cases = append(cases,
		peerClosedCase{name: "nil"},
		peerClosedCase{name: "EOF", err: io.EOF, wantEmptyRead: true},
		peerClosedCase{name: "timeout", err: os.ErrDeadlineExceeded},
		peerClosedCase{name: "other error", err: errors.New("unexpected failure")},
		peerClosedCase{name: "incomplete request", err: model.ErrIncompleteProtocolMessage},
	)
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			errs := []error{tt.err}
			if tt.err != nil {
				errs = append(errs, &net.OpError{
					Op: "read", Net: "tcp", Err: &os.SyscallError{Syscall: "recv", Err: tt.err},
				})
			}
			for _, err := range errs {
				if got := isPeerReset(err); got != tt.wantReset {
					t.Errorf("isPeerReset(%v) = %v, want %v", err, got, tt.wantReset)
				}
				if got := isPeerGoneOnWrite(err); got != tt.wantWrite {
					t.Errorf("isPeerGoneOnWrite(%v) = %v, want %v", err, got, tt.wantWrite)
				}
				if got := isEmptyQueryCommandProbe(0, err); got != tt.wantEmptyRead {
					t.Errorf("empty read (%v) ignored = %v, want %v", err, got, tt.wantEmptyRead)
				}
				if isEmptyQueryCommandProbe(1, err) {
					t.Errorf("read after receiving data (%v) was ignored", err)
				}
			}
		})
	}
}

// Use the same connection handler as queryCommandDaemon, returning its error to
// the test goroutine so the partial-request case can assert that it is rejected.
func queryCommandProbeClose(t *testing.T, reset bool) error {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	serveErrors := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serveErrors <- err
			return
		}
		serveErrors <- serveQueryCommand(conn)
	}()
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if reset {
		if err := conn.(*net.TCPConn).SetLinger(0); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := io.WriteString(conn, `{"action":"pi`); err != nil {
			t.Fatal(err)
		}
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serveErrors:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("fake daemon did not finish after peer close")
		return nil
	}
}

func TestQueryCommandProbeClose(t *testing.T) {
	if err := queryCommandProbeClose(t, true); err != nil {
		t.Fatalf("reset before sending request: %v", err)
	}
}

func TestQueryCommandPeerClosedPartialRequest(t *testing.T) {
	err := queryCommandProbeClose(t, false)
	if !errors.Is(err, model.ErrIncompleteProtocolMessage) {
		t.Fatalf("partial request error = %v, want %v", err, model.ErrIncompleteProtocolMessage)
	}
}
