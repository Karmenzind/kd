package daemon

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close() error = %v", err)
	}
	return addr
}

func startProtocolStub(t *testing.T, respond func(net.Conn)) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	done := make(chan struct{})
	var connections sync.WaitGroup
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			go func() {
				defer connections.Done()
				defer conn.Close()
				respond(conn)
			}()
		}
	}()
	stop := func() {
		listener.Close()
		<-done
		connections.Wait()
	}
	return listener.Addr().String(), stop
}

func writePing(conn net.Conn, protocolVersion int) {
	var request model.TCPQuery
	if model.NewProtocolReader(conn).Read(&request) != nil {
		return
	}
	_ = model.WriteProtocolMessage(conn, model.DaemonResponse{Ping: &model.DaemonPing{
		Available:       true,
		PID:             321,
		Version:         "v1.2.3",
		ProtocolVersion: protocolVersion,
		StartTime:       1234,
	}})
}

func TestPingDaemon(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		addr, stop := startProtocolStub(t, func(conn net.Conn) {
			writePing(conn, model.DaemonProtocolVersion)
		})
		defer stop()
		ping, err := PingDaemon(addr)
		if err != nil {
			t.Fatalf("PingDaemon() error = %v", err)
		}
		if ping.PID != 321 || ping.Version != "v1.2.3" || !ping.Available {
			t.Fatalf("PingDaemon() = %+v", ping)
		}
	})

	tests := []struct {
		name    string
		respond func(net.Conn)
		wantErr error
	}{
		{
			name: "non kd response",
			respond: func(conn net.Conn) {
				_, _ = conn.Write([]byte("hello\n"))
			},
			wantErr: ErrNotKDDaemon,
		},
		{
			name: "invalid JSON",
			respond: func(conn net.Conn) {
				_, _ = conn.Write([]byte("not-json\n"))
			},
			wantErr: ErrNotKDDaemon,
		},
		{
			name: "incompatible protocol",
			respond: func(conn net.Conn) {
				writePing(conn, model.DaemonProtocolVersion+1)
			},
			wantErr: ErrProtocolIncompatible,
		},
		{
			name: "legacy kd response",
			respond: func(conn net.Conn) {
				var request model.TCPQuery
				_ = model.NewProtocolReader(conn).Read(&request)
				_ = model.WriteProtocolMessage(conn, model.DaemonResponse{Error: "missing query"})
			},
			wantErr: ErrProtocolIncompatible,
		},
		{
			name: "accepts but does not respond",
			respond: func(conn net.Conn) {
				var request model.TCPQuery
				_ = model.NewProtocolReader(conn).Read(&request)
				<-time.After(PingTimeout + 50*time.Millisecond)
			},
			wantErr: ErrDaemonNoResponse,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, stop := startProtocolStub(t, tt.respond)
			defer stop()
			if _, err := PingDaemon(addr); !errors.Is(err, tt.wantErr) {
				t.Fatalf("PingDaemon() error = %v, want errors.Is(%v)", err, tt.wantErr)
			}
		})
	}

	if _, err := PingDaemon(unusedAddress(t)); !errors.Is(err, ErrDaemonNotRunning) {
		t.Fatalf("PingDaemon(unused address) error = %v", err)
	}
}

func TestCheckDaemonStatusAndStaleRuntime(t *testing.T) {
	dir := t.TempDir()
	runtimePath := filepath.Join(dir, "daemon.json")
	addr := unusedAddress(t)

	t.Run("missing runtime and listener", func(t *testing.T) {
		status := checkDaemonStatus(addr, runtimePath, func(int32) (bool, error) { return false, nil })
		if status.State != StateNotRunning || !errors.Is(status.Err, ErrDaemonNotRunning) {
			t.Fatalf("status = %+v", status)
		}
	})

	t.Run("stale dead PID is cleaned", func(t *testing.T) {
		info := &model.RunInfo{PID: 999999, Port: "19707", StartTime: 1, Instance: "stale"}
		if err := WriteDaemonInfo(runtimePath, info); err != nil {
			t.Fatal(err)
		}
		status := checkDaemonStatus(addr, runtimePath, func(pid int32) (bool, error) {
			if pid != int32(info.PID) {
				t.Fatalf("PidExists(%d), want %d", pid, info.PID)
			}
			return false, nil
		})
		if status.State != StateStaleRuntime {
			t.Fatalf("status.State = %q", status.State)
		}
		if _, err := os.Stat(runtimePath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("runtime file still exists: %v", err)
		}
	})

	t.Run("live PID is not killed or cleaned", func(t *testing.T) {
		info := &model.RunInfo{PID: os.Getpid(), Port: "19707", StartTime: 1, Instance: "reused"}
		if err := WriteDaemonInfo(runtimePath, info); err != nil {
			t.Fatal(err)
		}
		var checked atomic.Bool
		status := checkDaemonStatus(addr, runtimePath, func(int32) (bool, error) {
			checked.Store(true)
			return true, nil
		})
		if status.State != StateStaleRuntime || !checked.Load() {
			t.Fatalf("status = %+v, checked = %v", status, checked.Load())
		}
		if _, err := os.Stat(runtimePath); err != nil {
			t.Fatalf("runtime file was removed: %v", err)
		}
	})

	t.Run("corrupt runtime is recoverable", func(t *testing.T) {
		if err := os.WriteFile(runtimePath, []byte("not-json"), 0o600); err != nil {
			t.Fatal(err)
		}
		status := checkDaemonStatus(addr, runtimePath, func(int32) (bool, error) {
			t.Fatal("PidExists called for corrupt runtime")
			return false, nil
		})
		if status.State != StateRuntimeError || !errors.Is(status.Err, ErrRuntimeCorrupt) {
			t.Fatalf("status = %+v", status)
		}
		if _, err := os.Stat(runtimePath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("corrupt runtime was not cleaned: %v", err)
		}
	})
}

func TestCheckDaemonStatusRunning(t *testing.T) {
	addr, stop := startProtocolStub(t, func(conn net.Conn) {
		writePing(conn, model.DaemonProtocolVersion)
	})
	defer stop()
	runtimePath := filepath.Join(t.TempDir(), "daemon.json")
	if err := WriteDaemonInfo(runtimePath, &model.RunInfo{PID: 321, Port: "19707", Version: "v1.2.3", StartTime: 1234}); err != nil {
		t.Fatal(err)
	}
	status := checkDaemonStatus(addr, runtimePath, func(int32) (bool, error) {
		t.Fatal("PidExists called for a running daemon")
		return false, nil
	})
	if status.State != StateRunning || status.Ping == nil || status.Ping.PID != 321 || status.Runtime == nil {
		t.Fatalf("status = %+v", status)
	}
}

func TestCheckDaemonStatusListenerKinds(t *testing.T) {
	tests := []struct {
		name    string
		respond func(net.Conn)
		want    State
	}{
		{name: "other program", respond: func(conn net.Conn) { _, _ = conn.Write([]byte("other\n")) }, want: StatePortOccupied},
		{name: "incompatible daemon", respond: func(conn net.Conn) { writePing(conn, model.DaemonProtocolVersion+1) }, want: StateIncompatible},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, stop := startProtocolStub(t, tt.respond)
			defer stop()
			status := checkDaemonStatus(addr, filepath.Join(t.TempDir(), "missing.json"), func(int32) (bool, error) { return false, nil })
			if status.State != tt.want {
				t.Fatalf("status.State = %q, want %q (err=%v)", status.State, tt.want, status.Err)
			}
		})
	}
}

func TestRuntimeOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.json")
	old := &model.RunInfo{PID: 1, Port: "19707", StartTime: 1, Instance: "old"}
	current := &model.RunInfo{PID: 2, Port: "19707", StartTime: 2, Instance: "new"}
	if err := WriteDaemonInfo(path, old); err != nil {
		t.Fatal(err)
	}
	if err := WriteDaemonInfo(path, current); err != nil {
		t.Fatal(err)
	}
	if err := RemoveDaemonInfo(path, old); err != nil {
		t.Fatalf("RemoveDaemonInfo(old) error = %v", err)
	}
	got, err := ReadDaemonInfo(path)
	if err != nil {
		t.Fatalf("ReadDaemonInfo() error = %v", err)
	}
	if got.Instance != current.Instance {
		t.Fatalf("runtime instance = %q, want %q", got.Instance, current.Instance)
	}
	if err := RemoveDaemonInfo(path, current); err != nil {
		t.Fatalf("RemoveDaemonInfo(current) error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned runtime still exists: %v", err)
	}
}
