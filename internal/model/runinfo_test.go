package model

import (
	"os"
	"testing"
)

func TestRunInfoServerFlagsAndPort(t *testing.T) {
	r := &RunInfo{}
	if r.IsServer() {
		t.Fatal("new RunInfo reports server before SetServer(true)")
	}
	r.SetServer(true)
	if !r.IsServer() {
		t.Fatal("SetServer(true) was not remembered")
	}
	r.SetPort("19707")
	if r.Port != "19707" {
		t.Fatalf("Port = %q, want 19707", r.Port)
	}
}

func TestRunInfoOSInfo(t *testing.T) {
	r := &RunInfo{}
	osInfo := r.GetOSInfo()
	if osInfo == nil {
		t.Fatal("GetOSInfo() returned nil")
	}
	if r.OS != osInfo {
		t.Fatal("GetOSInfo() did not cache OS on RunInfo")
	}
}

func TestRunInfoGetTermSizeCaches(t *testing.T) {
	if _, err := os.Stat("/dev/tty"); err != nil {
		t.Skip("no controlling terminal")
	}
	r := &RunInfo{}
	w1, h1, err := r.GetTermSize()
	if err != nil {
		t.Skipf("GetTermSize() needs a TTY: %v", err)
	}
	if w1 <= 0 || h1 <= 0 {
		t.Fatalf("terminal size = %dx%d, want positive dimensions", w1, h1)
	}
	w2, h2, err := r.GetTermSize()
	if err != nil {
		t.Fatalf("second GetTermSize() error = %v", err)
	}
	if w1 != w2 || h1 != h2 {
		t.Fatalf("cached size = %dx%d, want %dx%d", w2, h2, w1, h1)
	}
}

func TestRunInfoGetTermSizeUsesPreset(t *testing.T) {
	r := &RunInfo{termWidth: 120, termHeight: 40}
	w, h, err := r.GetTermSize()
	if err != nil {
		t.Fatalf("GetTermSize() error = %v", err)
	}
	if w != 120 || h != 40 {
		t.Fatalf("size = %dx%d, want 120x40", w, h)
	}
}

func TestResultInitialize(t *testing.T) {
	tests := []struct {
		query   string
		wantEN  bool
		wantPhr bool
	}{
		{"hello", true, false},
		{"look forward", true, true},
		{"你好", false, false},
		{"hello-world", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r := &Result{BaseResult: &BaseResult{Query: tt.query}}
			r.Initialize()
			if r.IsEN != tt.wantEN || r.IsPhrase != tt.wantPhr {
				t.Fatalf("Initialize(%q) = (en=%v phrase=%v), want (en=%v phrase=%v)",
					tt.query, r.IsEN, r.IsPhrase, tt.wantEN, tt.wantPhr)
			}
		})
	}
}

func TestToDaemonResponseAndGetResult(t *testing.T) {
	base := &BaseResult{Query: "test", IsEN: true, Found: true}
	source := &Result{BaseResult: base, Keyword: "test"}
	resp := source.ToDaemonResponse()
	if resp.R != source || resp.Base != base {
		t.Fatalf("ToDaemonResponse() = %+v", resp)
	}

	resp.R = &Result{Keyword: "payload"}
	resp.Base = &BaseResult{Query: "payload", Found: true}
	got := resp.GetResult()
	if got.Keyword != "payload" || got.BaseResult != resp.Base {
		t.Fatalf("GetResult() = %+v", got)
	}

	q := &TCPQuery{B: &BaseResult{Query: "ping"}}
	qr := q.GetResult()
	if qr.BaseResult != q.B || qr.Query != "ping" {
		t.Fatalf("TCPQuery.GetResult() = %+v", qr)
	}
}
