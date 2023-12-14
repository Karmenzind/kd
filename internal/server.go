package internal

import (
	"encoding/json"
	"fmt"
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

		// Handle connections in a new goroutine
		go func(conn net.Conn) {
			buf := make([]byte, 1024)
			len, err := conn.Read(buf)
			if err != nil {
				fmt.Printf("Error reading: %#v\n", err)
				zap.S().Errorf("Error reading: %#v\n", err)
			}
			recv := string(buf[:len])
			fmt.Printf("Received: %s\n", recv)

			r := &model.Result{Query: recv}
			r.Initialize()

			query.FetchOnline(r)
			reply, err := json.Marshal(model.DaemonResponse{R: r, Error: "", Found: r.Found})

			if err != nil {
				zap.S().Errorf("[daemon] Failed to marshal response:", err)
				reply, _ = json.Marshal(model.DaemonResponse{R: nil, Error: fmt.Sprintf("序列化查询结果失败：%s", err)})
			}

			conn.Write([]byte(reply))
			conn.Close()
		}(conn)
	}
}
