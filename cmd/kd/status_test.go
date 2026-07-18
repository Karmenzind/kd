package main

import (
	"strings"
	"testing"

	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/model"
)

func TestResolveDaemonStatus(t *testing.T) {
	tests := []struct {
		name string
		in   daemon.Status
		want daemonStatus
	}{
		{
			name: "running with matching metadata",
			in: daemon.Status{
				State:   daemon.StateRunning,
				Ping:    &model.DaemonPing{PID: 123},
				Runtime: &model.RunInfo{PID: 123, Port: "19707"},
			},
			want: daemonStatus{Running: true, PID: 123, Port: "19707", State: daemon.StateRunning},
		},
		{
			name: "running ignores stale metadata",
			in: daemon.Status{
				State:   daemon.StateRunning,
				Ping:    &model.DaemonPing{PID: 123},
				Runtime: &model.RunInfo{PID: 456, Port: "19707"},
			},
			want: daemonStatus{Running: true, PID: 123, State: daemon.StateRunning},
		},
		{
			name: "stale runtime is not running",
			in:   daemon.Status{State: daemon.StateStaleRuntime, Runtime: &model.RunInfo{PID: 456}},
			want: daemonStatus{State: daemon.StateStaleRuntime},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveDaemonStatus(tt.in); got != tt.want {
				t.Fatalf("resolveDaemonStatus() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWriteStatusDaemonFields(t *testing.T) {
	tests := []struct {
		name    string
		status  daemonStatus
		want    []string
		notWant []string
	}{
		{
			name:    "stopped",
			status:  daemonStatus{State: daemon.StateNotRunning},
			want:    []string{"Daemon状态：未运行"},
			notWant: []string{"Daemon PID：", "Daemon端口："},
		},
		{
			name:   "running with validated metadata",
			status: daemonStatus{Running: true, PID: 123, Port: "19707", State: daemon.StateRunning},
			want:   []string{"Daemon状态：运行中", "Daemon PID：123", "Daemon端口：19707"},
		},
		{
			name:   "occupied",
			status: daemonStatus{State: daemon.StatePortOccupied},
			want:   []string{"Daemon状态：不可用（端口被其他程序占用）"},
		},
		{
			name:   "incompatible",
			status: daemonStatus{State: daemon.StateIncompatible},
			want:   []string{"Daemon状态：不可用（协议版本不兼容）"},
		},
		{
			name:   "unresponsive",
			status: daemonStatus{State: daemon.StateUnresponsive},
			want:   []string{"Daemon状态：不可用（无响应）"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			if err := writeStatus(&out, tt.status); err != nil {
				t.Fatalf("writeStatus() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("writeStatus() output = %q, missing %q", out.String(), want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(out.String(), notWant) {
					t.Fatalf("writeStatus() output = %q, unexpectedly contains %q", out.String(), notWant)
				}
			}
		})
	}
}
