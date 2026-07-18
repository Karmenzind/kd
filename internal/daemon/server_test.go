package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

func TestHandleClientMultipleMessages(t *testing.T) {
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		handleClientWithFetcher(server, func(r *model.Result) error {
			r.Found = true
			r.Keyword = "reply:" + r.Query
			return nil
		})
		close(done)
	}()
	t.Cleanup(func() { client.Close() })

	reader := model.NewProtocolReader(client)
	for _, query := range []string{"first", "第二条"} {
		request := model.TCPQuery{Action: "query", B: &model.BaseResult{Query: query}}
		if err := model.WriteProtocolMessage(client, request); err != nil {
			t.Fatalf("WriteProtocolMessage(%q) error = %v", query, err)
		}
		var response model.DaemonResponse
		if err := reader.Read(&response); err != nil {
			t.Fatalf("Read(response %q) error = %v", query, err)
		}
		if response.Error != "" || response.R == nil || response.R.Keyword != "reply:"+query {
			t.Fatalf("response for %q = %+v", query, response)
		}
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleClientWithFetcher() did not stop after connection close")
	}
}

func TestHandleClientInvalidRequestAndFetcherError(t *testing.T) {
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		handleClientWithFetcher(server, func(*model.Result) error {
			return errors.New("fetch failed")
		})
		close(done)
	}()

	reader := model.NewProtocolReader(client)
	if _, err := client.Write([]byte("not-json\n")); err != nil {
		t.Fatalf("Write(invalid JSON) error = %v", err)
	}
	var invalid model.DaemonResponse
	if err := reader.Read(&invalid); err != nil {
		t.Fatalf("Read(invalid response) error = %v", err)
	}
	if !strings.Contains(invalid.Error, "无效daemon请求") {
		t.Fatalf("invalid response = %+v", invalid)
	}

	request := model.TCPQuery{Action: "query", B: &model.BaseResult{Query: "valid"}}
	if err := model.WriteProtocolMessage(client, request); err != nil {
		t.Fatalf("WriteProtocolMessage() error = %v", err)
	}
	var failed model.DaemonResponse
	if err := reader.Read(&failed); err != nil {
		t.Fatalf("Read(fetch error response) error = %v", err)
	}
	if !strings.Contains(failed.Error, "fetch failed") {
		t.Fatalf("fetch error response = %+v", failed)
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not stop after client close")
	}
}

func TestHandleClientEarlyClose(t *testing.T) {
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		handleClientWithFetcher(server, func(*model.Result) error {
			t.Error("fetch called after client closed")
			return nil
		})
		close(done)
	}()
	client.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleClientWithFetcher() did not stop after early close")
	}
}

func TestHandleClientPing(t *testing.T) {
	client, server := net.Pipe()
	info := &model.RunInfo{PID: 42, Version: "v1.2.3", StartTime: 99}
	done := make(chan struct{})
	go func() {
		handleClientConnection(server, info, func(*model.Result) error {
			t.Error("fetch called for ping")
			return nil
		}, func() {
			t.Error("shutdown called for ping")
		})
		close(done)
	}()
	if err := model.WriteProtocolMessage(client, model.TCPQuery{Action: "ping", ProtocolVersion: model.DaemonProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	var response model.DaemonResponse
	if err := model.NewProtocolReader(client).Read(&response); err != nil {
		t.Fatal(err)
	}
	if response.Ping == nil || response.Ping.PID != info.PID || response.Ping.Version != info.Version || response.Ping.StartTime != info.StartTime {
		t.Fatalf("ping response = %+v", response.Ping)
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ping connection did not close")
	}
}

func TestIncompleteRequestTimesOut(t *testing.T) {
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		handleClientConnectionWithTimeout(server, &model.RunInfo{}, func(*model.Result) error { return nil }, func() {}, 50*time.Millisecond)
		close(done)
	}()
	if _, err := client.Write([]byte(`{"Action":"query"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("incomplete request did not time out")
	}
	client.Close()
}

type runningTestServer struct {
	addr        string
	runtimePath string
	info        *model.RunInfo
	cancel      context.CancelFunc
	done        <-chan error
}

func startRunningTestServer(t *testing.T, fetch func(*model.Result) error) *runningTestServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	info := &model.RunInfo{PID: os.Getpid(), Version: "v-test", StartTime: time.Now().Unix()}
	runtimePath := filepath.Join(t.TempDir(), "daemon.json")
	done := make(chan error, 1)
	go func() {
		done <- serveListener(ctx, listener, info, fetch, false, runtimePath)
		close(done)
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := PingDaemon(listener.Addr().String()); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("test daemon did not become ready")
		}
		<-time.After(10 * time.Millisecond)
	}
	server := &runningTestServer{addr: listener.Addr().String(), runtimePath: runtimePath, info: info, cancel: cancel, done: done}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("test daemon did not stop")
		}
	})
	return server
}

func sendQuery(t *testing.T, addr string, request model.TCPQuery) model.DaemonResponse {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := model.WriteProtocolMessage(conn, request); err != nil {
		t.Fatal(err)
	}
	var response model.DaemonResponse
	if err := model.NewProtocolReader(conn).Read(&response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestConnectionFailureAndPanicAreIsolated(t *testing.T) {
	server := startRunningTestServer(t, func(result *model.Result) error {
		if result.Query == "panic" {
			panic("test panic")
		}
		result.Found = true
		result.Keyword = result.Query
		return nil
	})

	conn, err := net.Dial("tcp", server.addr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("not-json\n"))
	conn.Close()

	panicResponse := sendQuery(t, server.addr, model.TCPQuery{Action: "query", B: &model.BaseResult{Query: "panic"}})
	if !strings.Contains(panicResponse.Error, "内部错误") {
		t.Fatalf("panic response = %+v", panicResponse)
	}
	if _, err := PingDaemon(server.addr); err != nil {
		t.Fatalf("daemon unavailable after bad connections: %v", err)
	}
	normal := sendQuery(t, server.addr, model.TCPQuery{Action: "query", B: &model.BaseResult{Query: "ok"}})
	if normal.R == nil || normal.R.Keyword != "ok" {
		t.Fatalf("normal response after panic = %+v", normal)
	}
}

func TestConcurrentPing(t *testing.T) {
	server := startRunningTestServer(t, func(*model.Result) error { return nil })
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := PingDaemon(server.addr)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("PingDaemon() error = %v", err)
		}
	}
}

func TestShutdownClosesListenerAndCleansOwnedRuntime(t *testing.T) {
	server := startRunningTestServer(t, func(*model.Result) error { return nil })
	response := sendQuery(t, server.addr, model.TCPQuery{Action: "shutdown", ProtocolVersion: model.DaemonProtocolVersion})
	if response.Error != "" {
		t.Fatalf("shutdown response = %+v", response)
	}
	server.cancel()
	server.cancel()
	select {
	case err := <-server.done:
		if err != nil {
			t.Fatalf("serveListener() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop")
	}
	if _, err := os.Stat(server.runtimePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime file still exists: %v", err)
	}
	if conn, err := net.DialTimeout("tcp", server.addr, 100*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("listener still accepts connections")
	}
}

func TestOldServerDoesNotRemoveNewRuntime(t *testing.T) {
	server := startRunningTestServer(t, func(*model.Result) error { return nil })
	replacement := &model.RunInfo{PID: server.info.PID + 1, Port: server.info.Port, Version: "v-new", StartTime: server.info.StartTime + 1, Instance: "replacement"}
	if err := WriteDaemonInfo(server.runtimePath, replacement); err != nil {
		t.Fatal(err)
	}
	server.cancel()
	select {
	case <-server.done:
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop")
	}
	got, err := ReadDaemonInfo(server.runtimePath)
	if err != nil {
		t.Fatalf("ReadDaemonInfo() error = %v", err)
	}
	if got.Instance != replacement.Instance {
		t.Fatalf("runtime instance = %q, want %q", got.Instance, replacement.Instance)
	}
}

func TestServerInitializationFailureLeavesNoRuntime(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(parentFile, "daemon.json")
	info := &model.RunInfo{PID: os.Getpid(), Version: "v-test", StartTime: 1}
	err = serveListener(context.Background(), listener, info, func(*model.Result) error { return nil }, false, runtimePath)
	if !errors.Is(err, ErrDaemonInit) {
		t.Fatalf("serveListener() error = %v", err)
	}
	if _, statErr := os.Stat(runtimePath); !errors.Is(statErr, os.ErrNotExist) && !errors.Is(statErr, syscall.ENOTDIR) {
		t.Fatalf("runtime file exists after init failure: %v", statErr)
	}
}
