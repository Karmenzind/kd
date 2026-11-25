package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

// Helpers for tests that run the real CLI in a child process (see
// TestQueryCommandProcess) against protocol stubs listening on 127.0.0.1:0.

type kdPaths struct {
	home, cacheRoot, config string
}

func kdPathsFor(home string) kdPaths {
	p := kdPaths{home: home}
	switch runtime.GOOS {
	case "darwin":
		p.cacheRoot = filepath.Join(home, "Library", "Caches", "kdcache")
		p.config = filepath.Join(home, ".config", "kd.toml")
	case "windows":
		p.cacheRoot = filepath.Join(home, ".cache", "kdcache")
		p.config = filepath.Join(home, "kd.toml")
	default:
		p.cacheRoot = filepath.Join(home, ".cache", "kdcache")
		p.config = filepath.Join(home, ".config", "kd.toml")
	}
	return p
}

type kdRun struct {
	stdin string
	env   []string
}

type kdResult struct {
	stdout, stderr string
	exit           int
}

// A fake systemctl that reports the unit as inactive and records every call,
// so a test can prove kd never asked systemd to do anything else.
func fakeSystemctl(t *testing.T) (dir, calls string) {
	t.Helper()
	dir = t.TempDir()
	calls = filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$@\" >> '" + calls + "'\n" +
		"if [ \"$1\" = is-active ]; then echo inactive; exit 3; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, calls
}

// runKD runs the CLI in a child process with HOME isolated and the editor,
// pager and proxy variables cleared. On Linux a fake systemctl is put first on PATH.
func runKD(t *testing.T, home, port string, opts kdRun, args ...string) kdResult {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"KD_TEST_QUERY_COMMAND=1", "KD_TEST_DAEMON_PORT=" + port,
		"HOME=" + home, "USERPROFILE=" + home, "KD_ALLOW_ROOT=1"}
	var systemctlCalls string
	if runtime.GOOS == "linux" {
		var dir string
		dir, systemctlCalls = fakeSystemctl(t)
		env = append(env, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	var base []string
	for _, kv := range os.Environ() {
		switch strings.ToUpper(strings.SplitN(kv, "=", 2)[0]) {
		case "VISUAL", "EDITOR", "PAGER", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY":
			continue
		}
		base = append(base, kv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, append([]string{"-test.run=^TestQueryCommandProcess$", "--"}, args...)...)
	cmd.Env = append(append(base, env...), opts.env...)
	cmd.Stdin = strings.NewReader(opts.stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("kd %v did not finish: %v; stdout=%q stderr=%q", args, ctx.Err(), &stdout, &stderr)
	}
	if cmd.ProcessState == nil {
		t.Fatalf("kd %v did not start: %v", args, runErr)
	}
	if systemctlCalls != "" {
		if body, err := os.ReadFile(systemctlCalls); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
				if !strings.HasPrefix(line, "is-active ") {
					t.Errorf("kd asked systemctl to %q, want only is-active", line)
				}
			}
		}
	}
	return kdResult{stdout: stdout.String(), stderr: stderr.String(), exit: cmd.ProcessState.ExitCode()}
}

func (r kdResult) check(t *testing.T, wantExit int, stdout, stderr []string) {
	t.Helper()
	if r.exit != wantExit {
		t.Errorf("exit = %d, want %d; stdout=%q stderr=%q", r.exit, wantExit, r.stdout, r.stderr)
	}
	for _, want := range stdout {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", r.stdout, want)
		}
	}
	for _, want := range stderr {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", r.stderr, want)
		}
	}
}

// daemonStubOptions describes how a protocol stub answers.
type daemonStubOptions struct {
	protocolVersion int   // 0 means the current protocol
	startTime       int64 // 0 means now
	shutdown        bool  // accept a "shutdown" request and stop listening
}

type daemonStub struct {
	port      string
	shutdowns chan struct{}
	opts      daemonStubOptions
	listener  net.Listener
	closeOnce sync.Once
}

func (s *daemonStub) closeListener() { s.closeOnce.Do(func() { s.listener.Close() }) }

// startDaemonStub answers ping (and optionally shutdown) like a kd daemon.
// Any other request is a test failure.
func startDaemonStub(t *testing.T, opts daemonStubOptions) *daemonStub {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	s := &daemonStub{port: port, shutdowns: make(chan struct{}, 4), opts: opts, listener: listener}
	done := make(chan struct{})
	serveErrors := make(chan error, 16)
	var connections sync.WaitGroup
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			go func() {
				defer connections.Done()
				if err := s.serve(conn); err != nil {
					serveErrors <- err
				}
			}()
		}
	}()
	t.Cleanup(func() {
		s.closeListener()
		<-done
		connections.Wait()
		close(serveErrors)
		for err := range serveErrors {
			t.Error(err)
		}
	})
	return s
}

func (s *daemonStub) serve(conn net.Conn) error {
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set stub deadline: %w", err)
	}
	reader := &queryCommandReadCounter{Reader: conn}
	var request model.TCPQuery
	if err := model.NewProtocolReader(reader).Read(&request); err != nil {
		if isEmptyQueryCommandProbe(reader.n, err) {
			return nil
		}
		return fmt.Errorf("read stub request: %w", err)
	}
	switch {
	case request.Action == "ping":
		version := s.opts.protocolVersion
		if version == 0 {
			version = model.DaemonProtocolVersion
		}
		start := s.opts.startTime
		if start == 0 {
			start = time.Now().Unix()
		}
		err := model.WriteProtocolMessage(conn, &model.DaemonResponse{Ping: &model.DaemonPing{
			Available: true, PID: os.Getpid(), Version: VERSION,
			ProtocolVersion: version, StartTime: start,
		}})
		if err != nil && !isPeerGoneOnWrite(err) {
			return fmt.Errorf("write stub ping response: %w", err)
		}
	case request.Action == "shutdown" && s.opts.shutdown:
		if err := model.WriteProtocolMessage(conn, &model.DaemonResponse{}); err != nil {
			return fmt.Errorf("write stub shutdown response: %w", err)
		}
		s.shutdowns <- struct{}{}
		s.closeListener()
	default:
		return fmt.Errorf("stub got unexpected request %+v", request)
	}
	return nil
}

func (s *daemonStub) wasShutDown() bool {
	select {
	case <-s.shutdowns:
		return true
	default:
		return false
	}
}

// startClosingListener accepts each connection, reads the request and closes
// the connection without answering: something that is not a kd daemon.
func startClosingListener(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			_, _ = conn.Read(make([]byte, 4096))
			conn.Close()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		<-done
	})
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// unusedPort returns a loopback port with nothing listening on it.
func unusedPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	listener.Close()
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestStatusCommand(t *testing.T) {
	home, _ := queryCommandFixture(t)
	paths := kdPathsFor(home)
	running := startDaemonStub(t, daemonStubOptions{})
	incompatible := startDaemonStub(t, daemonStubOptions{protocolVersion: model.DaemonProtocolVersion + 1})
	for _, tt := range []struct {
		name string
		port string
		want []string
	}{
		{"running", running.port, []string{"Daemon状态：运行中", "Daemon PID：" + strconv.Itoa(os.Getpid()), "版本：" + VERSION}},
		{"not running", unusedPort(t), []string{"Daemon状态：未运行"}},
		{"not a kd daemon", startClosingListener(t), []string{"Daemon状态：不可用"}},
		{"incompatible protocol", incompatible.port, []string{"Daemon状态：不可用（协议版本不兼容）"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := runKD(t, home, tt.port, kdRun{}, "--status")
			r.check(t, 0, append(tt.want, "配置文件地址："+paths.config, "数据文件目录："+paths.cacheRoot), nil)
		})
	}
}

func TestBrokenConfig(t *testing.T) {
	home, _ := queryCommandFixture(t)
	paths := kdPathsFor(home)
	if err := os.WriteFile(paths.config, []byte("paging = [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	port := unusedPort(t)
	// A broken config must not lock the user out of the commands that repair it.
	runKD(t, home, port, kdRun{}, "--status").check(t, 0, []string{"配置文件地址：" + paths.config}, []string{"解析配置文件失败"})
	// A query cannot run without a usable configuration.
	runKD(t, home, port, kdRun{}, "hello").check(t, 1, nil, []string{"解析配置文件失败"})
}

func TestStopCommand(t *testing.T) {
	home, _ := queryCommandFixture(t)
	t.Run("running daemon is asked to shut down", func(t *testing.T) {
		stub := startDaemonStub(t, daemonStubOptions{shutdown: true})
		runKD(t, home, stub.port, kdRun{}, "--stop").check(t, 0, []string{"守护进程已经停止"}, nil)
		if !stub.wasShutDown() {
			t.Error("daemon stub never got a shutdown request")
		}
	})
	t.Run("nothing running", func(t *testing.T) {
		runKD(t, home, unusedPort(t), kdRun{}, "--stop").check(t, 0, []string{"未发现守护进程，无需停止"}, nil)
	})
	t.Run("port held by another program", func(t *testing.T) {
		runKD(t, home, startClosingListener(t), kdRun{}, "--stop").check(t, 1, nil, []string{"daemon端口由其他程序占用，未执行进程终止操作"})
	})
	t.Run("old daemon without safe shutdown", func(t *testing.T) {
		stub := startDaemonStub(t, daemonStubOptions{protocolVersion: model.DaemonProtocolVersion + 1})
		runKD(t, home, stub.port, kdRun{}, "--stop").check(t, 1, nil, []string{"旧版daemon不支持安全停止"})
		if stub.wasShutDown() {
			t.Error("an incompatible daemon must not be sent a shutdown request")
		}
	})
}

// After the stub shuts down nothing listens, so the restart has to launch a new
// daemon. The launched "kd --server" is this test binary, which exits at once on
// the unknown flag; the point is that the failure is reported and exits 1.
func TestRestartCommandReportsStartFailure(t *testing.T) {
	home, _ := queryCommandFixture(t)
	stub := startDaemonStub(t, daemonStubOptions{shutdown: true})
	r := runKD(t, home, stub.port, kdRun{}, "--restart")
	r.check(t, 1, []string{"守护进程已经停止"}, []string{"daemon startup timed out", "daemon process exited"})
	if !stub.wasShutDown() {
		t.Error("daemon stub never got a shutdown request")
	}
}

func TestRestartCommandRefusesForeignPort(t *testing.T) {
	home, _ := queryCommandFixture(t)
	runKD(t, home, startClosingListener(t), kdRun{}, "--restart").check(t, 1, nil, []string{"daemon端口由其他程序占用"})
}

func TestDaemonCommand(t *testing.T) {
	home, _ := queryCommandFixture(t)
	t.Run("reuses running daemon", func(t *testing.T) {
		stub := startDaemonStub(t, daemonStubOptions{})
		runKD(t, home, stub.port, kdRun{}, "--daemon").check(t, 0, []string{"守护进程已就绪，PID：" + strconv.Itoa(os.Getpid())}, nil)
	})
	t.Run("port held by another program", func(t *testing.T) {
		runKD(t, home, startClosingListener(t), kdRun{}, "--daemon").check(t, 1, nil, []string{"daemon端口已被其他程序占用"})
	})
	t.Run("incompatible daemon", func(t *testing.T) {
		stub := startDaemonStub(t, daemonStubOptions{protocolVersion: model.DaemonProtocolVersion + 1})
		runKD(t, home, stub.port, kdRun{}, "--daemon").check(t, 1, nil, []string{"已有daemon协议版本不兼容"})
	})
}

// --server cannot bind a port the stub already holds; it must report that
// instead of starting a second daemon.
func TestServerCommandRefusesForeignPort(t *testing.T) {
	home, _ := queryCommandFixture(t)
	runKD(t, home, startClosingListener(t), kdRun{}, "--server").check(t, 1, nil, []string{"daemon端口已被其他程序占用"})
}

func TestGenerateConfigCommand(t *testing.T) {
	read := func(t *testing.T, path string) string {
		t.Helper()
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	t.Run("overwrite existing and reload", func(t *testing.T) {
		home, _ := queryCommandFixture(t)
		paths := kdPathsFor(home)
		before := read(t, paths.config)
		port := unusedPort(t)
		runKD(t, home, port, kdRun{stdin: "y\ny\n"}, "--generate-config").check(t, 0, []string{"已经存在，是否覆盖", "已经写入配置文件"}, nil)
		after := read(t, paths.config)
		if after == before || !strings.Contains(after, "split_camelcase_and_snakecase") {
			t.Fatalf("config was not replaced by the generated sample:\n%s", after)
		}
		// The generated file must load without a warning.
		if r := runKD(t, home, port, kdRun{}, "--status"); r.exit != 0 || r.stderr != "" {
			t.Errorf("generated config did not load cleanly: stderr=%q", r.stderr)
		}
	})
	for _, tt := range []struct {
		name  string
		stdin string
	}{
		{"decline overwrite", "n\n"},
		{"decline after preview", "y\nn\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home, _ := queryCommandFixture(t)
			paths := kdPathsFor(home)
			before := read(t, paths.config)
			r := runKD(t, home, unusedPort(t), kdRun{stdin: tt.stdin}, "--generate-config")
			r.check(t, 0, []string{"已取消"}, nil)
			if strings.Contains(r.stdout, "已经写入配置文件") {
				t.Errorf("declined run still reported writing: %q", r.stdout)
			}
			if got := read(t, paths.config); got != before {
				t.Errorf("config changed after declining:\n%s", got)
			}
		})
	}
	t.Run("new file after an invalid answer", func(t *testing.T) {
		home, _ := queryCommandFixture(t)
		paths := kdPathsFor(home)
		if err := os.Remove(paths.config); err != nil {
			t.Fatal(err)
		}
		// The invalid answer is followed by an explicit one: AskYN keeps its
		// previous answer on empty input and would otherwise never end.
		r := runKD(t, home, unusedPort(t), kdRun{stdin: "x\nY\n"}, "--generate-config")
		r.check(t, 0, []string{"Please input Y or n.", "已经写入配置文件"}, nil)
		if strings.Contains(r.stdout, "已经存在") {
			t.Errorf("asked to overwrite a file that does not exist: %q", r.stdout)
		}
		if got := read(t, paths.config); !strings.Contains(got, "split_camelcase_and_snakecase") {
			t.Errorf("generated config is missing keys:\n%s", got)
		}
	})
}

func writeFakeEditor(t *testing.T, output string, exit int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-editor")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > '" + output + "'\nexit " + strconv.Itoa(exit) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEditConfigCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("--edit-config always starts notepad on Windows")
	}
	readArg := func(t *testing.T, path string) string {
		t.Helper()
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("editor was not started: %v", err)
		}
		return string(body)
	}
	t.Run("VISUAL wins over EDITOR", func(t *testing.T) {
		home, _ := queryCommandFixture(t)
		paths := kdPathsFor(home)
		visualOut := filepath.Join(t.TempDir(), "visual")
		editorOut := filepath.Join(t.TempDir(), "editor")
		env := []string{"VISUAL=" + writeFakeEditor(t, visualOut, 0), "EDITOR=" + writeFakeEditor(t, editorOut, 0)}
		runKD(t, home, unusedPort(t), kdRun{env: env}, "--edit-config").check(t, 0, []string{"找到预设VISUAL"}, nil)
		if got := readArg(t, visualOut); got != paths.config {
			t.Errorf("editor opened %q, want %q", got, paths.config)
		}
		if _, err := os.Stat(editorOut); err == nil {
			t.Error("EDITOR ran although VISUAL was set")
		}
	})
	t.Run("EDITOR and missing config", func(t *testing.T) {
		home, _ := queryCommandFixture(t)
		paths := kdPathsFor(home)
		if err := os.Remove(paths.config); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "editor")
		env := []string{"EDITOR=" + writeFakeEditor(t, out, 0)}
		runKD(t, home, unusedPort(t), kdRun{env: env}, "--edit-config").check(t, 0, []string{"配置文件不存在，已生成默认配置", "找到预设EDITOR"}, nil)
		if got := readArg(t, out); got != paths.config {
			t.Errorf("editor opened %q, want %q", got, paths.config)
		}
		if _, err := os.Stat(paths.config); err != nil {
			t.Errorf("default config was not created: %v", err)
		}
	})
	t.Run("editor failure passes its exit status through", func(t *testing.T) {
		home, _ := queryCommandFixture(t)
		env := []string{"VISUAL=" + writeFakeEditor(t, filepath.Join(t.TempDir(), "out"), 3)}
		runKD(t, home, unusedPort(t), kdRun{env: env}, "--edit-config").check(t, 3, nil, []string{"exit status 3"})
	})
	t.Run("editor that does not exist", func(t *testing.T) {
		home, _ := queryCommandFixture(t)
		env := []string{"VISUAL=" + filepath.Join(t.TempDir(), "no-such-editor")}
		runKD(t, home, unusedPort(t), kdRun{env: env}, "--edit-config").check(t, 1, nil, []string{"no-such-editor"})
	})
}

func TestQueryCommandBranches(t *testing.T) {
	home, _ := queryCommandFixture(t)
	paths := kdPathsFor(home)
	port := queryCommandDaemon(t)
	if err := os.WriteFile(filepath.Join(paths.cacheRoot, "online_not_found"), []byte("zzzz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout []string
		wantStderr []string
	}{
		{name: "word starting with punctuation", args: []string{".hello"}, wantStderr: []string{"请以字母、数字或汉字开头"}},
		{name: "long text starting with punctuation", args: []string{"-t", "@hello"}, wantStderr: []string{"请以字母、数字、汉字或标点符号开头"}},
		{name: "long text may start with a full-width mark", args: []string{"-t", "。Good morning."}, wantStdout: []string{"早上好。"}},
		{name: "word in the not-found list stays local", args: []string{"zzzz"}, wantStdout: []string{"Not found"}},
		{name: "not-found list is bypassed by -n", args: []string{"-n", "zzzz"}, wantStdout: []string{"Not found"}},
		{name: "long text falls back to the original after its identifier rewrite fails", args: []string{"-n", "-t", "fooBar"}, wantStdout: []string{"早上好。"}},
		{name: "long text not found after the fallback", args: []string{"-n", "-t", "nothing"}, wantStdout: []string{"Not found"}},
		{name: "brief", args: []string{"--brief", "hello"}, wantStdout: []string{"你好"}},
		{name: "no brief", args: []string{"--no-brief", "hello"}, wantStdout: []string{"你好"}},
		{name: "theme", args: []string{"-T", "wudao", "hello"}, wantStdout: []string{"你好"}},
		{name: "speak is rejected for long text", args: []string{"-s", "-t", "Good morning."}, wantStdout: []string{"早上好。"}, wantStderr: []string{"读音功能暂不支持长文本模式"}},
		{name: "no arguments prints the prompt", args: nil, wantStdout: []string{"<text>", "查看详细帮助"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runKD(t, home, port, kdRun{}, tt.args...).check(t, tt.wantExit, tt.wantStdout, tt.wantStderr)
		})
	}

	t.Run("update notice follows the result", func(t *testing.T) {
		notice := filepath.Join(paths.cacheRoot, "latest_tag")
		if err := os.WriteFile(notice, []byte("v99.0.0"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(notice) })
		runKD(t, home, port, kdRun{}, "hello").check(t, 0, []string{"你好", "发现新版本v99.0.0，请执行`kd --update`更新"}, nil)
		// JSON output is machine-readable, so no notice may follow it.
		r := runKD(t, home, port, kdRun{}, "--json", "hello")
		r.check(t, 0, []string{`"k":"hello"`}, nil)
		if strings.Contains(r.stdout, "发现新版本") {
			t.Errorf("JSON output carries an update notice: %q", r.stdout)
		}
	})
}

// A listener that is not a kd daemon: the cache still answers, but a lookup that
// needs the daemon fails with exit 1 instead of waiting for a daemon that cannot start.
func TestQueryCommandUnreachableDaemon(t *testing.T) {
	home, _ := queryCommandFixture(t)
	port := startClosingListener(t)
	t.Run("cache hit does not need the daemon", func(t *testing.T) {
		runKD(t, home, port, kdRun{}, "hello").check(t, 0, []string{"你好"}, nil)
	})
	t.Run("online lookup fails", func(t *testing.T) {
		r := runKD(t, home, port, kdRun{}, "-n", "hello")
		r.check(t, 1, nil, []string{"守护进程启动失败"})
		if r.stdout != "" {
			t.Errorf("stdout = %q, want empty", r.stdout)
		}
	})
	t.Run("JSON lookup fails", func(t *testing.T) {
		r := runKD(t, home, port, kdRun{}, "--json", "-n", "hello")
		r.check(t, 1, nil, []string{"守护进程启动失败"})
		if r.stdout != "" {
			t.Errorf("stdout = %q, want empty", r.stdout)
		}
	})
}

// A daemon that started before the config file was last changed is restarted by
// the next query. Here the restart cannot bring a daemon back (see
// TestRestartCommandReportsStartFailure), and the cached answer still comes out.
func TestQueryCommandRestartsDaemonAfterConfigChange(t *testing.T) {
	home, _ := queryCommandFixture(t)
	t.Run("announced", func(t *testing.T) {
		stub := startDaemonStub(t, daemonStubOptions{startTime: 1, shutdown: true})
		runKD(t, home, stub.port, kdRun{}, "hello").check(t, 0, []string{"你好"}, []string{"检测到配置文件发生修改，正在重启守护进程"})
		if !stub.wasShutDown() {
			t.Error("old daemon was not asked to shut down")
		}
	})
	t.Run("quiet with --json", func(t *testing.T) {
		stub := startDaemonStub(t, daemonStubOptions{startTime: 1, shutdown: true})
		r := runKD(t, home, stub.port, kdRun{}, "--json", "hello")
		r.check(t, 0, []string{`"k":"hello"`}, nil)
		if strings.Contains(r.stderr, "检测到配置文件发生修改") {
			t.Errorf("JSON run announced the restart: %q", r.stderr)
		}
		if !stub.wasShutDown() {
			t.Error("old daemon was not asked to shut down")
		}
	})
	t.Run("newer daemon is left alone", func(t *testing.T) {
		stub := startDaemonStub(t, daemonStubOptions{startTime: time.Now().Add(time.Hour).Unix()})
		runKD(t, home, stub.port, kdRun{}, "hello").check(t, 0, []string{"你好"}, nil)
	})
}

func TestLogComponentForClient(t *testing.T) {
	home, _ := queryCommandFixture(t)
	paths := kdPathsFor(home)
	logFile := filepath.Join(t.TempDir(), "kd.log")
	config := "paging = false\n[logging]\nenable = true\nlevel = \"debug\"\npath = '" + logFile + "'\n"
	if err := os.WriteFile(paths.config, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	port := startDaemonStub(t, daemonStubOptions{}).port
	runKD(t, home, port, kdRun{}, "--status").check(t, 0, []string{"Log地址：" + logFile}, nil)
	body, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("log file was not written: %v", err)
	}
	if !strings.Contains(string(body), `"component": "client"`) {
		t.Errorf("log lacks the client component field:\n%s", body)
	}
}

// --update is only exercised up to the version lookup: the tag list comes from a
// local server and every answer stops before any binary is downloaded or replaced.
func TestUpdateCommandVersionLookup(t *testing.T) {
	for _, tt := range []struct {
		name       string
		status     int
		body       string
		stdin      string
		wantExit   int
		wantStdout []string
		wantStderr []string
		wantTag    string
	}{
		{name: "newer version declined", body: `[{"name":"v99.0.0"}]`, stdin: "n\n", wantStdout: []string{"Found new version (v99.0.0)", "Canceled."}, wantTag: "v99.0.0"},
		{name: "already latest", body: `[{"name":"` + VERSION + `"}]`, wantStdout: []string{"You're using the latest version."}, wantTag: VERSION},
		{name: "lookup fails", status: http.StatusForbidden, body: `{"message":"rate limited"}`, wantExit: 1, wantStderr: []string{"ERROR"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home, _ := queryCommandFixture(t)
			paths := kdPathsFor(home)
			requests := make(chan string, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.Method + " " + r.URL.Path
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			tagFile := filepath.Join(paths.cacheRoot, "latest_tag")
			env := []string{"KD_TEST_TAGLIST_URL=" + server.URL + "/tags", "KD_TEST_LATEST_TAG_FILE=" + tagFile}
			r := runKD(t, home, unusedPort(t), kdRun{stdin: tt.stdin, env: env}, "--update")
			r.check(t, tt.wantExit, tt.wantStdout, tt.wantStderr)
			if strings.Contains(r.stdout, "Start downloading") {
				t.Errorf("update started a download: %q", r.stdout)
			}
			if got := len(requests); got != 1 {
				t.Errorf("server saw %d requests, want 1", got)
			} else if req := <-requests; req != "GET /tags" {
				t.Errorf("request = %q, want GET /tags", req)
			}
			body, err := os.ReadFile(tagFile)
			if tt.wantTag == "" {
				if err == nil {
					t.Errorf("cached tag %q written after a failed lookup", body)
				}
			} else if err != nil || string(body) != tt.wantTag {
				t.Errorf("cached tag = %q, %v; want %q", body, err, tt.wantTag)
			}
		})
	}
}
