package systemd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeFakeSystemctl(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := `@echo off
set "subcmd=%~1"
if /i "%subcmd%"=="is-active" (
  if "%KD_FAKE_SYSTEMCTL_ACTIVE%"=="1" (
    <nul set /p "=active"
  ) else (
    <nul set /p "=inactive"
  )
  exit /b 0
)
if /i "%subcmd%"=="stop" exit /b 0
if /i "%subcmd%"=="restart" exit /b 0
echo unknown 1>&2
exit /b 1
`
		path := filepath.Join(dir, "systemctl.bat")
		if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		return
	}
	script := `subcmd="$1"
shift
if [ "$1" = "--user" ]; then shift; fi
unit="$1"
case "$subcmd" in
is-active)
  if [ "$KD_FAKE_SYSTEMCTL_ACTIVE" = "1" ]; then
    echo active
  else
    echo inactive
  fi
  ;;
stop|restart)
  ;;
*)
  echo unknown 1>&2
  exit 1
  ;;
esac
`
	path := filepath.Join(dir, "systemctl")
	body := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func prependPATH(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestServiceIsActiveInactive(t *testing.T) {
	dir := t.TempDir()
	writeFakeSystemctl(t, dir)
	prependPATH(t, dir)
	t.Setenv("KD_FAKE_SYSTEMCTL_ACTIVE", "0")

	active, err := ServiceIsActive("kd-server", true)
	if err != nil {
		t.Fatalf("ServiceIsActive() error = %v", err)
	}
	if active {
		t.Fatal("ServiceIsActive() = true, want false for inactive")
	}
}

func TestServiceIsActiveActive(t *testing.T) {
	dir := t.TempDir()
	writeFakeSystemctl(t, dir)
	prependPATH(t, dir)
	t.Setenv("KD_FAKE_SYSTEMCTL_ACTIVE", "1")

	active, err := ServiceIsActive("kd-server", true)
	if err != nil {
		t.Fatalf("ServiceIsActive() error = %v", err)
	}
	if !active {
		t.Fatal("ServiceIsActive() = false, want true")
	}
}

func TestStopAndRestartService(t *testing.T) {
	dir := t.TempDir()
	writeFakeSystemctl(t, dir)
	prependPATH(t, dir)

	if _, err := StopService("kd-server", true); err != nil {
		t.Fatalf("StopService() error = %v", err)
	}
	if _, err := RestartService("kd-server", true); err != nil {
		t.Fatalf("RestartService() error = %v", err)
	}
}

func TestServiceIsActiveSystemScope(t *testing.T) {
	dir := t.TempDir()
	writeFakeSystemctl(t, dir)
	prependPATH(t, dir)
	t.Setenv("KD_FAKE_SYSTEMCTL_ACTIVE", "0")

	active, err := ServiceIsActive("kd-server", false)
	if err != nil {
		t.Fatalf("ServiceIsActive(system) error = %v", err)
	}
	if active {
		t.Fatal("ServiceIsActive(system) = true, want false")
	}
}
