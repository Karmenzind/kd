package daemon

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
	d "github.com/Karmenzind/kd/pkg/decorate"
)

// EnsureReady verifies the daemon through the TCP ping endpoint and starts it
// when necessary. Runtime metadata is never treated as proof of availability.
func EnsureReady() error {
	status := CheckDaemonStatus(DefaultAddress())
	if status.State != StateRunning {
		d.EchoRun("未找到守护进程，正在启动...")
		return StartDaemonProcess()
	}
	if status.Ping != nil && run.Info.Version != status.Ping.Version {
		d.EchoWarn(
			"正在运行的守护程序版本（%s）与当前程序（%s）不一致，建议执行`kd --restart`重启",
			status.Ping.Version,
			run.Info.Version,
		)
	}
	return nil
}

// QueryDaemon performs one query over the existing newline-delimited JSON TCP
// protocol and closes the connection after the response.
func QueryDaemon(addr string, result *model.Result) error {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return fmt.Errorf("连接daemon失败: %w", err)
	}
	defer conn.Close()
	request := model.TCPQuery{Action: "query", B: result.BaseResult}
	if err := model.WriteProtocolMessage(conn, request); err != nil {
		return fmt.Errorf("发送daemon请求失败: %w", err)
	}

	var response model.DaemonResponse
	if err = model.NewProtocolReader(conn).Read(&response); err != nil {
		return fmt.Errorf("解析daemon返回结果失败: %s", err)
	}
	if response.Error != "" {
		return errors.New(response.Error)
	}
	if response.R == nil || response.Base == nil {
		return errors.New("解析daemon返回结果失败: 响应缺少结果字段")
	}
	*result = *response.GetResult()
	return nil
}
