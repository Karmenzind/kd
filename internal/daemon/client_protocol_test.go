package daemon

import (
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/Karmenzind/kd/internal/model"
)

func startProtocolServer(t *testing.T, handler func(net.Conn)) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			handler(conn)
		}
	}()
	return listener.Addr().String()
}

func TestQueryDaemonRoundTrip(t *testing.T) {
	addr := startProtocolServer(t, func(conn net.Conn) {
		defer conn.Close()
		var request model.TCPQuery
		if err := model.NewProtocolReader(conn).Read(&request); err != nil {
			return
		}
		result := request.GetResult()
		result.Found = true
		result.Keyword = "reply:" + result.Query
		_ = model.WriteProtocolMessage(conn, result.ToDaemonResponse())
	})

	result := &model.Result{BaseResult: &model.BaseResult{Query: "abandon", IsEN: true}}
	if err := QueryDaemon(addr, result); err != nil {
		t.Fatalf("QueryDaemon() error = %v", err)
	}
	if !result.Found || result.Keyword != "reply:abandon" || result.Query != "abandon" {
		t.Fatalf("QueryDaemon() result = %+v", result)
	}
}

func TestQueryDaemonErrors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		handler func(net.Conn)
		want    string
	}{
		{
			name: "daemon error response",
			handler: func(conn net.Conn) {
				defer conn.Close()
				var request model.TCPQuery
				_ = model.NewProtocolReader(conn).Read(&request)
				_ = model.WriteProtocolMessage(conn, model.DaemonResponse{Error: "upstream unavailable"})
			},
			want: "upstream unavailable",
		},
		{
			name: "invalid response",
			handler: func(conn net.Conn) {
				defer conn.Close()
				_, _ = conn.Write([]byte("not-json\n"))
			},
			want: "解析daemon返回结果失败",
		},
		{
			name: "connection closed early",
			handler: func(conn net.Conn) {
				conn.Close()
			},
			want: "解析daemon返回结果失败",
		},
		{
			name: "partial response",
			handler: func(conn net.Conn) {
				defer conn.Close()
				var request model.TCPQuery
				_ = model.NewProtocolReader(conn).Read(&request)
				_ = model.WriteProtocolMessage(conn, model.DaemonResponse{})
			},
			want: "响应缺少结果字段",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			addr := startProtocolServer(t, tt.handler)
			result := &model.Result{BaseResult: &model.BaseResult{Query: "test"}}
			err := QueryDaemon(addr, result)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("QueryDaemon() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestQueryDaemonConnectionFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	result := &model.Result{BaseResult: &model.BaseResult{Query: "test"}}
	err = QueryDaemon(addr, result)
	if err == nil || !strings.Contains(err.Error(), "连接daemon失败") {
		t.Fatalf("QueryDaemon() error = %v", err)
	}
}

func TestQueryDaemonConcurrentClients(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	const clients = 8
	serverErrors := make(chan error, clients)
	go func() {
		for range clients {
			conn, err := listener.Accept()
			if err != nil {
				serverErrors <- err
				continue
			}
			go func() {
				defer conn.Close()
				var request model.TCPQuery
				if err := model.NewProtocolReader(conn).Read(&request); err != nil {
					serverErrors <- err
					return
				}
				result := request.GetResult()
				result.Found = true
				result.Keyword = result.Query
				serverErrors <- model.WriteProtocolMessage(conn, result.ToDaemonResponse())
			}()
		}
	}()

	var wg sync.WaitGroup
	clientErrors := make(chan error, clients)
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			query := string(rune('a' + i))
			result := &model.Result{BaseResult: &model.BaseResult{Query: query}}
			if err := QueryDaemon(listener.Addr().String(), result); err != nil {
				clientErrors <- err
				return
			}
			if result.Keyword != query {
				clientErrors <- errors.New("response did not match request")
			}
		}()
	}
	wg.Wait()
	close(clientErrors)
	for err := range clientErrors {
		t.Errorf("concurrent QueryDaemon() error = %v", err)
	}
	for range clients {
		if err := <-serverErrors; err != nil {
			t.Errorf("concurrent server error = %v", err)
		}
	}
}
