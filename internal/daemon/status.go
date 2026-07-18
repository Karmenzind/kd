package daemon

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	PingTimeout         = 500 * time.Millisecond
	DaemonStartTimeout  = 3 * time.Second
	DaemonRetryInterval = 75 * time.Millisecond
	ConnectionTimeout   = 5 * time.Second
)

var (
	ErrDaemonNotRunning     = errors.New("daemon is not running")
	ErrDaemonStartTimeout   = errors.New("daemon startup timed out")
	ErrPortOccupied         = errors.New("daemon port is occupied")
	ErrNotKDDaemon          = errors.New("listener is not a kd daemon")
	ErrProtocolIncompatible = errors.New("daemon protocol is incompatible")
	ErrDaemonNoResponse     = errors.New("daemon did not respond")
	ErrDaemonInit           = errors.New("daemon initialization failed")
)

type State string

const (
	StateRunning      State = "running"
	StateNotRunning   State = "not-running"
	StateStaleRuntime State = "stale-runtime"
	StatePortOccupied State = "port-occupied"
	StateIncompatible State = "protocol-incompatible"
	StateUnresponsive State = "unresponsive"
	StateRuntimeError State = "runtime-corrupt"
)

type Status struct {
	State      State
	Ping       *model.DaemonPing
	Runtime    *model.RunInfo
	RuntimeErr error
	Err        error
}

func DefaultAddress() string {
	return net.JoinHostPort("localhost", strconv.Itoa(run.SERVER_PORT))
}

func PingDaemon(addr string) (*model.DaemonPing, error) {
	conn, err := net.DialTimeout("tcp", addr, PingTimeout)
	if err != nil {
		if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
			return nil, fmt.Errorf("%w: %v", ErrDaemonNoResponse, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrDaemonNotRunning, err)
	}
	defer conn.Close()
	deadline := time.Now().Add(PingTimeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set daemon ping deadline: %w", err)
	}
	request := model.TCPQuery{Action: "ping", ProtocolVersion: model.DaemonProtocolVersion}
	if err := model.WriteProtocolMessage(conn, request); err != nil {
		return nil, fmt.Errorf("%w: write ping: %v", ErrDaemonNoResponse, err)
	}
	var response model.DaemonResponse
	if err := model.NewProtocolReader(conn).Read(&response); err != nil {
		if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
			return nil, fmt.Errorf("%w: %v", ErrDaemonNoResponse, err)
		}
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: connection closed", ErrNotKDDaemon)
		}
		return nil, fmt.Errorf("%w: %v", ErrNotKDDaemon, err)
	}
	if response.Ping == nil {
		if response.Error != "" {
			// Older kd daemons return their normal structured error response for
			// the unknown ping action. They are kd, but not ping-compatible.
			return nil, ErrProtocolIncompatible
		}
		return nil, ErrNotKDDaemon
	}
	if !response.Ping.Available || response.Ping.PID <= 0 || response.Ping.Version == "" {
		return nil, ErrNotKDDaemon
	}
	if response.Ping.ProtocolVersion != model.DaemonProtocolVersion {
		return response.Ping, fmt.Errorf("%w: daemon=%d client=%d", ErrProtocolIncompatible, response.Ping.ProtocolVersion, model.DaemonProtocolVersion)
	}
	return response.Ping, nil
}

func CheckDaemonStatus(addr string) Status {
	return checkDaemonStatus(addr, GetDaemonInfoPath(), process.PidExists)
}

func checkDaemonStatus(addr, runtimePath string, pidExists func(int32) (bool, error)) Status {
	raw, rawErr := os.ReadFile(runtimePath)
	info, runtimeErr := ReadDaemonInfo(runtimePath)
	status := Status{Runtime: info, RuntimeErr: runtimeErr}
	ping, pingErr := PingDaemon(addr)
	if pingErr == nil {
		status.State = StateRunning
		status.Ping = ping
		return status
	}
	status.Err = pingErr
	switch {
	case errors.Is(pingErr, ErrProtocolIncompatible):
		status.State = StateIncompatible
		status.Ping = ping
	case errors.Is(pingErr, ErrNotKDDaemon):
		status.State = StatePortOccupied
		status.Err = errors.Join(ErrPortOccupied, pingErr)
	case errors.Is(pingErr, ErrDaemonNoResponse):
		status.State = StateUnresponsive
	case runtimeErr != nil && !errors.Is(runtimeErr, os.ErrNotExist):
		status.State = StateRuntimeError
		status.Err = errors.Join(ErrRuntimeCorrupt, pingErr)
		if rawErr == nil {
			_ = removeUnchangedRuntime(runtimePath, raw)
		}
	case info != nil:
		status.State = StateStaleRuntime
		if exists, err := pidExists(int32(info.PID)); err == nil && !exists && rawErr == nil {
			_ = removeUnchangedRuntime(runtimePath, raw)
		}
	default:
		status.State = StateNotRunning
	}
	return status
}
