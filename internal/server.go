package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	d "github.com/Karmenzind/kd/pkg/decorate"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/query"
	"github.com/Karmenzind/kd/pkg"
	"go.uber.org/zap"
)

// TODO  支持自定义
var SERVER_PORT = 19707

type DaemonInfoType struct {
	StartTime int64
	Port      string
	PID       int
}

var DaemonInfo = &DaemonInfoType{}

func recordRunningInfo(port string) {
	DaemonInfo.StartTime = time.Now().Unix()
	DaemonInfo.PID = os.Getpid()
	DaemonInfo.Port = port
	pkg.SaveJson(
		filepath.Join(cache.CACHE_RUN_PATH, "daemon.json"),
		DaemonInfo,
	)
	zap.S().Infof("Recorded running information of daemon %+v", DaemonInfo)
}

func GetDaemonInfo() *DaemonInfoType {
	if *DaemonInfo == (DaemonInfoType{}) {
		err := pkg.LoadJson(filepath.Join(cache.CACHE_RUN_PATH, "daemon.json"), DaemonInfo)
		if err != nil {
			d.EchoFatal("获取守护进程信息失败，请执行`kd --stop && kd --daemon`")
		}
	}
	return DaemonInfo
}

func StartServer() (err error) {
	IS_SERVER = true
	addr := fmt.Sprintf("localhost:%d", SERVER_PORT)
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
	daemon.InitCron()
	go recordRunningInfo(port)

	fmt.Printf("Listening on host: %s, port: %s\n", host, port)

	for {
		// Listen for an incoming connection
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
        fmt.Println("Connection closed by client")
		return
	} else if err != nil {
		fmt.Printf("Error reading: %#v\n", err)
		zap.S().Errorf("Error reading: %#v\n", err)
		// FIXME (k): <2024-01-02> reply
        return
	}

	fmt.Printf("Received: %s\n", recv)
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

    fmt.Printf("Sending to client: %s \n", reply)
	conn.Write(append(reply, '\n'))
	conn.Close()

}
