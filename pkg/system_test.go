package pkg

import (
	"runtime"
	"testing"
)

func TestGetOSInfoNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("covered by TestGetOSInfoLinux")
	}
	info, err := GetOSInfo()
	if err != nil {
		t.Fatalf("GetOSInfo() error = %v", err)
	}
	if info.OS != runtime.GOOS {
		t.Fatalf("OS = %q, want %q", info.OS, runtime.GOOS)
	}
	if info.Distro != "" {
		t.Fatalf("Distro = %q, want empty on non-linux", info.Distro)
	}
	if info.IsDebianBased {
		t.Fatal("IsDebianBased = true, want false on non-linux")
	}
}

func TestGetOSInfoLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux /etc/os-release only")
	}
	info, err := GetOSInfo()
	if err != nil {
		t.Fatalf("GetOSInfo() error = %v", err)
	}
	if info.OS != "linux" {
		t.Fatalf("OS = %q, want linux", info.OS)
	}
}
