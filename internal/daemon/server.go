package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/query"
	"github.com/Karmenzind/kd/internal/run"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"go.uber.org/zap"
)

const acceptRetryMaxDelay = time.Second

type daemonServer struct {
	listener          net.Listener
	info              *model.RunInfo
	fetch             func(*model.Result) error
	connectionTimeout time.Duration
	cancel            context.CancelFunc
	stopOnce          sync.Once
	requests          sync.WaitGroup
}

func StartServer() error {
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	return startServer(ctx, DefaultAddress(), run.Info, query.FetchOnline, true)
}

func startServer(
	parent context.Context,
	addr string,
	info *model.RunInfo,
	fetch func(*model.Result) error,
	startBackgroundTasks bool,
) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		if _, pingErr := PingDaemon(addr); pingErr == nil {
			// A concurrent caller won the bind race and is already ready.
			return nil
		} else {
			return errors.Join(ErrPortOccupied, pingErr, err)
		}
	}
	return serveListener(parent, listener, info, fetch, startBackgroundTasks, GetDaemonInfoPath())
}

func serveListener(
	parent context.Context,
	listener net.Listener,
	info *model.RunInfo,
	fetch func(*model.Result) error,
	startBackgroundTasks bool,
	runtimePath string,
) error {
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		listener.Close()
		return errors.Join(ErrDaemonInit, err)
	}
	instance, err := NewInstanceID()
	if err != nil {
		listener.Close()
		return errors.Join(ErrDaemonInit, err)
	}
	info.SetServer(true)
	info.SetPort(port)
	info.Instance = instance
	info.SetOSInfo()
	if err := WriteDaemonInfo(runtimePath, info); err != nil {
		listener.Close()
		return errors.Join(ErrDaemonInit, err)
	}

	ctx, cancel := context.WithCancel(parent)
	server := &daemonServer{
		listener:          listener,
		info:              info,
		fetch:             fetch,
		connectionTimeout: ConnectionTimeout,
		cancel:            cancel,
	}
	defer func() {
		server.Shutdown()
		server.waitRequests(server.connectionTimeout)
		if err := RemoveDaemonInfo(runtimePath, info); err != nil {
			zap.S().Warnf("Failed to remove daemon runtime information: %s", err)
		}
	}()

	d.EchoOkay("Listening on host: %s, port: %s\n", host, port)
	zap.S().Info("Started kd server")
	if startBackgroundTasks {
		InitCron(ctx, server.Shutdown)
	}
	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()
	return server.Serve(ctx)
}

func (s *daemonServer) Shutdown() {
	s.stopOnce.Do(func() {
		s.cancel()
		if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			zap.S().Warnf("Failed to close daemon listener: %s", err)
		}
	})
}

func (s *daemonServer) waitRequests(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		s.requests.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		zap.S().Warn("Timed out waiting for daemon requests to finish")
	}
}

func (s *daemonServer) Serve(ctx context.Context) error {
	var retryDelay time.Duration
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			if temporary, ok := err.(net.Error); ok && temporary.Temporary() {
				if retryDelay == 0 {
					retryDelay = 5 * time.Millisecond
				} else {
					retryDelay *= 2
				}
				if retryDelay > acceptRetryMaxDelay {
					retryDelay = acceptRetryMaxDelay
				}
				zap.S().Warnf("Temporary daemon accept error; retrying in %s: %s", retryDelay, err)
				select {
				case <-time.After(retryDelay):
					continue
				case <-ctx.Done():
					return nil
				}
			}
			return fmt.Errorf("daemon accept failed: %w", err)
		}
		retryDelay = 0
		s.requests.Add(1)
		go func() {
			defer s.requests.Done()
			handleClientConnectionWithTimeout(conn, s.info, s.fetch, s.Shutdown, s.connectionTimeout)
		}()
	}
}

func handleClient(conn net.Conn) {
	handleClientConnection(conn, run.Info, query.FetchOnline, func() {})
}

func handleClientWithFetcher(conn net.Conn, fetch func(*model.Result) error) {
	handleClientConnection(conn, run.Info, fetch, func() {})
}

func handleClientConnection(
	conn net.Conn,
	info *model.RunInfo,
	fetch func(*model.Result) error,
	shutdown func(),
) {
	handleClientConnectionWithTimeout(conn, info, fetch, shutdown, ConnectionTimeout)
}

func handleClientConnectionWithTimeout(
	conn net.Conn,
	info *model.RunInfo,
	fetch func(*model.Result) error,
	shutdown func(),
	connectionTimeout time.Duration,
) {
	defer conn.Close()
	writeResponse := func(response any) error {
		return writeDaemonResponse(conn, response, connectionTimeout)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			zap.S().Errorf("Recovered panic while handling daemon connection: %v", recovered)
			_ = writeResponse(model.DaemonResponse{Error: "daemon内部错误"})
		}
	}()

	reader := model.NewProtocolReader(conn)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(connectionTimeout)); err != nil {
			zap.S().Warnf("Failed to set daemon request deadline: %s", err)
			return
		}
		request := model.TCPQuery{}
		err := reader.Read(&request)
		if err == io.EOF {
			return
		}
		if err != nil {
			_ = writeResponse(model.DaemonResponse{Error: fmt.Sprintf("无效daemon请求：%s", err)})
			if errors.Is(err, model.ErrInvalidProtocolMessage) || errors.Is(err, model.ErrEmptyProtocolMessage) {
				continue
			}
			return
		}

		switch request.Action {
		case "ping":
			response := model.DaemonResponse{Ping: &model.DaemonPing{
				Available:       true,
				PID:             info.PID,
				Version:         info.Version,
				ProtocolVersion: model.DaemonProtocolVersion,
				StartTime:       info.StartTime,
			}}
			_ = writeResponse(response)
			return
		case "shutdown":
			if request.ProtocolVersion != model.DaemonProtocolVersion {
				_ = writeResponse(model.DaemonResponse{Error: "daemon协议版本不兼容"})
				return
			}
			if err := writeResponse(model.DaemonResponse{}); err == nil {
				shutdown()
			}
			return
		}

		if request.B == nil {
			_ = writeResponse(model.DaemonResponse{Error: "无效daemon请求：缺少查询内容"})
			continue
		}
		result := request.GetResult()
		result.Initialize()
		response := result.ToDaemonResponse()
		if err = fetch(result); err != nil {
			zap.S().Warnf("Failed to fetch online result: %s", err)
			errmsg := fmt.Sprintf("在线查询失败（%v）", err)
			if strings.Contains(err.Error(), "proxyconnect") {
				errmsg = "代理连接异常，请求失败：" + err.Error()
			}
			response = &model.DaemonResponse{Error: errmsg}
		}
		if err = writeResponse(response); err != nil {
			zap.S().Warnf("Failed to send daemon response: %s", err)
			return
		}
	}
}

func writeDaemonResponse(conn net.Conn, response any, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	return model.WriteProtocolMessage(conn, response)
}
