package daemon

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

func TestStartDaemonAlreadyRunning(t *testing.T) {
	addr, stop := startProtocolStub(t, func(conn net.Conn) { writePing(conn, model.DaemonProtocolVersion) })
	defer stop()
	var launches atomic.Int32
	ping, err := startDaemon(addr, time.Second, func() (<-chan error, error) {
		launches.Add(1)
		return nil, errors.New("must not launch")
	})
	if err != nil || ping.PID != 321 {
		t.Fatalf("startDaemon() ping = %+v, error = %v", ping, err)
	}
	if launches.Load() != 0 {
		t.Fatalf("launches = %d, want 0", launches.Load())
	}
}

func TestStartDaemonWaitsUntilReady(t *testing.T) {
	addr := unusedAddress(t)
	stopReady := make(chan func(), 1)
	ping, err := startDaemon(addr, time.Second, func() (<-chan error, error) {
		exited := make(chan error, 1)
		go func() {
			<-time.After(2 * DaemonRetryInterval)
			listener, listenErr := net.Listen("tcp", addr)
			if listenErr != nil {
				exited <- listenErr
				return
			}
			serverDone := make(chan struct{})
			go func() {
				defer close(serverDone)
				for {
					conn, acceptErr := listener.Accept()
					if acceptErr != nil {
						return
					}
					go func(conn net.Conn) {
						defer conn.Close()
						writePing(conn, model.DaemonProtocolVersion)
					}(conn)
				}
			}()
			stopReady <- func() {
				listener.Close()
				<-serverDone
			}
		}()
		return exited, nil
	})
	if err != nil || ping == nil || ping.PID != 321 {
		t.Fatalf("startDaemon() ping = %+v, error = %v", ping, err)
	}
	stop := <-stopReady
	defer stop()
}

func TestStartDaemonFailures(t *testing.T) {
	t.Run("initialization", func(t *testing.T) {
		_, err := startDaemon(unusedAddress(t), time.Second, func() (<-chan error, error) {
			return nil, errors.New("cannot start")
		})
		if !errors.Is(err, ErrDaemonInit) {
			t.Fatalf("startDaemon() error = %v", err)
		}
	})

	t.Run("timeout includes child exit", func(t *testing.T) {
		exited := make(chan error, 1)
		exited <- errors.New("init failed")
		close(exited)
		_, err := startDaemon(unusedAddress(t), 3*DaemonRetryInterval, func() (<-chan error, error) {
			return exited, nil
		})
		if !errors.Is(err, ErrDaemonStartTimeout) {
			t.Fatalf("startDaemon() error = %v", err)
		}
	})

	t.Run("occupied port does not launch", func(t *testing.T) {
		addr, stop := startProtocolStub(t, func(conn net.Conn) { _, _ = conn.Write([]byte("other\n")) })
		defer stop()
		var launched atomic.Bool
		_, err := startDaemon(addr, time.Second, func() (<-chan error, error) {
			launched.Store(true)
			return nil, nil
		})
		if !errors.Is(err, ErrPortOccupied) || launched.Load() {
			t.Fatalf("startDaemon() error = %v, launched = %v", err, launched.Load())
		}
	})
}

func TestConcurrentStartReusesSingleDaemon(t *testing.T) {
	addr := unusedAddress(t)
	var listenerMu sync.Mutex
	var listener net.Listener
	var acceptDone chan struct{}
	launcher := func() (<-chan error, error) {
		exited := make(chan error, 1)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			exited <- err
			close(exited)
			return exited, nil
		}
		listenerMu.Lock()
		listener = l
		acceptDone = make(chan struct{})
		done := acceptDone
		listenerMu.Unlock()
		go func() {
			defer close(done)
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				go func() {
					defer conn.Close()
					writePing(conn, model.DaemonProtocolVersion)
				}()
			}
		}()
		return exited, nil
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := startDaemon(addr, time.Second, launcher)
			results <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Errorf("concurrent start error = %v", err)
		}
	}
	listenerMu.Lock()
	if listener != nil {
		listener.Close()
	}
	done := acceptDone
	listenerMu.Unlock()
	if done != nil {
		<-done
	}
}

// Lifecycle tests must never reach the user's systemctl on Linux.
func fakeSystemctl(t *testing.T, active bool, actionExit string) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		return ""
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls")
	body := `#!/bin/sh
printf '%s\n' "$*" >> "$KD_T6_SYSTEMCTL_LOG"
if [ "$1" = is-active ]; then
  printf '%s\n' "$KD_T6_SYSTEMCTL_STATE"
  exit 0
fi
exit "$KD_T6_SYSTEMCTL_EXIT"
`
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KD_T6_SYSTEMCTL_LOG", logPath)
	state := "inactive"
	if active {
		state = "active"
	}
	t.Setenv("KD_T6_SYSTEMCTL_STATE", state)
	t.Setenv("KD_T6_SYSTEMCTL_EXIT", actionExit)
	return logPath
}

func TestRequestShutdownResponses(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "acknowledged", body: "{}\n"},
		{name: "rejected", body: "{\"Error\":\"shutdown rejected\"}\n", want: "shutdown rejected"},
		{name: "malformed", body: "not JSON\n", want: "read daemon shutdown response"},
		{name: "closed", want: "read daemon shutdown response"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := make(chan model.TCPQuery, 1)
			addr, stop := startProtocolStub(t, func(conn net.Conn) {
				var request model.TCPQuery
				if err := model.NewProtocolReader(conn).Read(&request); err != nil {
					return
				}
				requests <- request
				_, _ = io.WriteString(conn, tt.body)
			})
			defer stop()
			err := requestShutdown(addr)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("requestShutdown() = %v, want %q", err, tt.want)
			}
			select {
			case request := <-requests:
				if request.Action != "shutdown" || request.ProtocolVersion != model.DaemonProtocolVersion {
					t.Fatalf("shutdown request = %+v", request)
				}
			case <-time.After(time.Second):
				t.Fatal("shutdown request was not received")
			}
		})
	}
	if err := requestShutdown(unusedAddress(t)); err == nil || !strings.Contains(err.Error(), "connect daemon for shutdown") {
		t.Fatalf("requestShutdown(unused address) = %v", err)
	}
}

func TestKillDaemonStopsOnlyTestServer(t *testing.T) {
	fakeSystemctl(t, false, "0")
	server := startRunningTestServer(t, func(*model.Result) error {
		t.Error("shutdown must not fetch")
		return nil
	})
	useDaemonPort(t, server.addr)
	if err := KillDaemonIfRunning(); err != nil {
		t.Fatalf("KillDaemonIfRunning() = %v", err)
	}
	select {
	case err := <-server.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("test server did not stop after shutdown")
	}
	if _, err := os.Stat(server.runtimePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime file was not removed: %v", err)
	}
}

func TestLifecycleRejectsOtherListeners(t *testing.T) {
	fakeSystemctl(t, false, "0")
	useDaemonInfoDir(t)
	for _, tt := range []struct {
		name string
		run  func() error
	}{
		{name: "stop", run: KillDaemonIfRunning},
		{name: "restart", run: RestartDaemon},
		{name: "quiet restart", run: RestartDaemonQuiet},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var shutdowns atomic.Int32
			addr, stop := startProtocolStub(t, func(conn net.Conn) {
				var request model.TCPQuery
				if model.NewProtocolReader(conn).Read(&request) == nil {
					if request.Action == "shutdown" {
						shutdowns.Add(1)
					}
					_ = model.WriteProtocolMessage(conn, model.DaemonResponse{})
				}
			})
			defer stop()
			useDaemonPort(t, addr)
			if err := tt.run(); !errors.Is(err, ErrNotKDDaemon) {
				t.Fatalf("lifecycle action = %v, want ErrNotKDDaemon", err)
			}
			if shutdowns.Load() != 0 {
				t.Fatal("sent shutdown to a non-kd listener")
			}
		})
	}
}

func TestKillDaemonPropagatesShutdownRejection(t *testing.T) {
	fakeSystemctl(t, false, "0")
	addr, stop := startProtocolStub(t, func(conn net.Conn) {
		var request model.TCPQuery
		if model.NewProtocolReader(conn).Read(&request) != nil {
			return
		}
		if request.Action == "ping" {
			_ = model.WriteProtocolMessage(conn, model.DaemonResponse{Ping: &model.DaemonPing{
				Available: true, PID: 321, Version: "v-test", ProtocolVersion: model.DaemonProtocolVersion,
			}})
		} else {
			_ = model.WriteProtocolMessage(conn, model.DaemonResponse{Error: "test shutdown denied"})
		}
	})
	defer stop()
	useDaemonPort(t, addr)
	if err := killDaemonIfRunning(false); err == nil || err.Error() != "test shutdown denied" {
		t.Fatalf("killDaemonIfRunning() = %v", err)
	}
	if _, err := PingDaemon(addr); err != nil {
		t.Fatalf("rejected shutdown closed listener: %v", err)
	}
}

func TestLifecycleUsesFakeSystemd(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd lifecycle integration is Linux-only")
	}
	for _, tt := range []struct {
		name   string
		run    func() error
		action string
	}{
		{name: "stop", run: KillDaemonIfRunning, action: "stop"},
		{name: "restart", run: RestartDaemon, action: "restart"},
		{name: "quiet restart", run: RestartDaemonQuiet, action: "restart"},
	} {
		for _, code := range []string{"0", "7"} {
			t.Run(tt.name+"/exit="+code, func(t *testing.T) {
				logPath := fakeSystemctl(t, true, code)
				// If systemd handling regresses, only this test-owned listener is reachable.
				var requests atomic.Int32
				addr, stop := startProtocolStub(t, func(conn net.Conn) {
					requests.Add(1)
					_, _ = io.WriteString(conn, "not kd\n")
				})
				defer stop()
				useDaemonPort(t, addr)
				err := tt.run()
				if (err == nil) != (code == "0") {
					t.Fatalf("systemd exit=%s, lifecycle error=%v", code, err)
				}
				body, readErr := os.ReadFile(logPath)
				want := "is-active --user " + SYSTEMD_UNIT_NAME + "\n" + tt.action + " --user " + SYSTEMD_UNIT_NAME + "\n"
				if readErr != nil || string(body) != want || requests.Load() != 0 {
					t.Fatalf("systemctl calls = %q, want %q; direct requests=%d err=%v", body, want, requests.Load(), readErr)
				}
			})
		}
	}
}
