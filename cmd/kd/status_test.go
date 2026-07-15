package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Karmenzind/kd/internal/model"
)

func TestResolveDaemonStatus(t *testing.T) {
	tests := []struct {
		name     string
		find     func() (int, bool, error)
		loadInfo func() (*model.RunInfo, error)
		want     daemonStatus
		wantErr  string
	}{
		{
			name: "running with matching metadata",
			find: func() (int, bool, error) {
				return 123, true, nil
			},
			loadInfo: func() (*model.RunInfo, error) {
				return &model.RunInfo{PID: 123, Port: "19707"}, nil
			},
			want: daemonStatus{Running: true, PID: 123, Port: "19707"},
		},
		{
			name: "stale metadata without process",
			find: func() (int, bool, error) {
				return 0, false, nil
			},
			loadInfo: func() (*model.RunInfo, error) {
				t.Fatal("loadInfo called for a stopped daemon")
				return &model.RunInfo{PID: 456, Port: "19707"}, nil
			},
			want: daemonStatus{},
		},
		{
			name: "running with stale metadata",
			find: func() (int, bool, error) {
				return 123, true, nil
			},
			loadInfo: func() (*model.RunInfo, error) {
				return &model.RunInfo{PID: 456, Port: "19707"}, nil
			},
			want: daemonStatus{Running: true, PID: 123},
		},
		{
			name: "running without metadata",
			find: func() (int, bool, error) {
				return 123, true, nil
			},
			loadInfo: func() (*model.RunInfo, error) {
				return nil, errors.New("missing daemon info")
			},
			want: daemonStatus{Running: true, PID: 123},
		},
		{
			name: "process lookup error",
			find: func() (int, bool, error) {
				return 0, false, errors.New("process list unavailable")
			},
			loadInfo: func() (*model.RunInfo, error) {
				t.Fatal("loadInfo called after process lookup failed")
				return nil, nil
			},
			wantErr: "查询守护进程状态失败: process list unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDaemonStatus(tt.find, tt.loadInfo)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("resolveDaemonStatus() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveDaemonStatus() error = %v", err)
			}
			if got != tt.want {
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
			status:  daemonStatus{},
			want:    []string{"Daemon状态：未运行"},
			notWant: []string{"Daemon PID：", "Daemon端口："},
		},
		{
			name:   "running with validated metadata",
			status: daemonStatus{Running: true, PID: 123, Port: "19707"},
			want:   []string{"Daemon状态：运行中", "Daemon PID：123", "Daemon端口：19707"},
		},
		{
			name:    "running without validated port",
			status:  daemonStatus{Running: true, PID: 123},
			want:    []string{"Daemon状态：运行中", "Daemon PID：123"},
			notWant: []string{"Daemon端口："},
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
