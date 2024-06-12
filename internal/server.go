package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"

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
	defer conn.Close()

	recv, err := bufio.NewReader(conn).ReadBytes('\n')
	if err == io.EOF {
		zap.S().Debugf("Connection closed by client.")
		d.EchoWarn("Connection closed by client")
		return
	} else if err != nil {
		d.EchoWrong(fmt.Sprintf("Error reading: %#v\n", err))
		zap.S().Errorf("Error reading: %#v\n", err)
		// FIXME (k): <2024-01-02> reply
		return
	}

	d.EchoRun("Received: %s\n", recv)
	q := model.TCPQuery{}
	err = json.Unmarshal(recv, &q)
	if err != nil {
		zap.S().Errorf("[daemon] Failed to marshal request:", err)

	}
	r := q.GetResult()
	r.Initialize()

	query.FetchOnline(r)
	reply, err := json.Marshal(r.ToDaemonResponse())

	if err != nil {
		zap.S().Errorf("[daemon] Failed to marshal response:", err)
		reply, _ = json.Marshal(model.DaemonResponse{Error: fmt.Sprintf("序列化查询结果失败：%s", err)})
	}

	d.EchoRun("Sending to client: %s \n", reply)
	conn.Write(append(reply, '\n'))
	conn.Close()
}
