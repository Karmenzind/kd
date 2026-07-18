package daemon

import (
	"errors"
	"net"
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
