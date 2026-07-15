package internal

import (
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"

	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/query"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"go.uber.org/zap"
)

func StartServer() (err error) {
	if !pkg.IsPortOpen(run.SERVER_PORT) {
		d.EchoFatal("端口%d已经被占用，请停止占用端口的程序后重试", run.SERVER_PORT)
	}

	run.Info.SetServer(true)
	addr := fmt.Sprintf("localhost:%d", run.SERVER_PORT)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		zap.S().Errorf("Failed to start server:", err)
		return err
	}
	defer l.Close()
	host, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		zap.S().Errorf("Failed to SplitHostPort:", err)
		return err
	}
	run.Info.SetPort(port)
	run.Info.SetOSInfo()

	go run.Info.SaveToFile(filepath.Join(run.CACHE_RUN_PATH, "daemon.json"))

	d.EchoOkay("Listening on host: %s, port: %s\n", host, port)
	zap.S().Info("Started kd server")

	daemon.InitCron()

	for {
		conn, err := l.Accept()
		if err != nil {
			zap.S().Errorf("Failed to accept connection:", err)
		}

		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	handleClientWithFetcher(conn, query.FetchOnline)
}

func handleClientWithFetcher(conn net.Conn, fetch func(*model.Result) error) {
	defer conn.Close()
	reader := model.NewProtocolReader(conn)
	for {
		q := model.TCPQuery{}
		err := reader.Read(&q)
		if err == io.EOF {
			return
		}
		if err != nil {
			_ = model.WriteProtocolMessage(conn, model.DaemonResponse{Error: fmt.Sprintf("无效daemon请求：%s", err)})
			if strings.Contains(err.Error(), "decode protocol message") || strings.Contains(err.Error(), "empty protocol message") {
				continue
			}
			return
		}
		if q.B == nil {
			_ = model.WriteProtocolMessage(conn, model.DaemonResponse{Error: "无效daemon请求：缺少查询内容"})
			continue
		}

		r := q.GetResult()
		r.Initialize()
		response := r.ToDaemonResponse()
		if err = fetch(r); err != nil {
			zap.S().Warnf("Failed to fetch online result: %s", err)
			errmsg := fmt.Sprintf("在线查询失败（%v）", err)
			if strings.Contains(err.Error(), "proxyconnect") {
				errmsg = "代理连接异常，请求失败：" + err.Error()
			}
			response = &model.DaemonResponse{Error: errmsg}
		}
		if err = model.WriteProtocolMessage(conn, response); err != nil {
			zap.S().Warnf("Failed to send daemon response: %s", err)
			return
		}
	}
}
