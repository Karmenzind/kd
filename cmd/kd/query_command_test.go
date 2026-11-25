package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/internal/update"
)

// Run the real entry point in a child so main's exit status and stderr are tested.
func TestQueryCommandProcess(t *testing.T) {
	if os.Getenv("KD_TEST_QUERY_COMMAND") != "1" {
		return
	}
	port, err := strconv.Atoi(os.Getenv("KD_TEST_DAEMON_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	run.SERVER_PORT = port
	// --update tests point the tag lookup at a local server and keep the cached
	// tag inside the temporary home.
	if u := os.Getenv("KD_TEST_TAGLIST_URL"); u != "" {
		update.TAGLIST_URL = u
		update.LATEST_TAG_FILE = os.Getenv("KD_TEST_LATEST_TAG_FILE")
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"kd"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	t.Fatal("missing CLI arguments")
}

func queryCommandFixture(t *testing.T) (home, counter string) {
	t.Helper()
	home = t.TempDir()
	cacheRoot := filepath.Join(home, ".cache", "kdcache")
	configPath := filepath.Join(home, ".config", "kd.toml")
	if runtime.GOOS == "darwin" {
		cacheRoot = filepath.Join(home, "Library", "Caches", "kdcache")
	} else if runtime.GOOS == "windows" {
		configPath = filepath.Join(home, "kd.toml")
	}
	for _, dir := range []string{cacheRoot, filepath.Dir(configPath), filepath.Join(cacheRoot, "stat")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(configPath, []byte("paging = false\nfreq_alert = true\nsplit_camelcase_and_snakecase = true\n[logging]\nenable = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	counter = filepath.Join(cacheRoot, "stat", "counter-"+time.Now().Format("200601")+".json")
	if err := os.WriteFile(counter, []byte(`{"hello":3,"getuserprofile":3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	originalRoot, originalLongText, originalDB := cache.CACHE_ROOT_PATH, cache.LONG_TEXT_CACHE_FILE, cache.LiteDB
	defer func() {
		cache.CACHE_ROOT_PATH, cache.LONG_TEXT_CACHE_FILE, cache.LiteDB = originalRoot, originalLongText, originalDB
	}()
	cache.CACHE_ROOT_PATH = cacheRoot
	cache.LONG_TEXT_CACHE_FILE = filepath.Join(cacheRoot, "long_text_results.json")
	if err := cache.InitDB(); err != nil {
		t.Fatal(err)
	}
	defer cache.LiteDB.Close()
	if err := cache.UpdateQueryCache(&model.Result{
		BaseResult: &model.BaseResult{Query: "hello", IsEN: true, Found: true},
		Keyword:    "hello", Paraphrase: []string{"你好"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := cache.UpdateLongTextCache(&model.Result{BaseResult: &model.BaseResult{
		Query: "Good morning.", IsLongText: true, MachineTrans: "早上好。",
	}}); err != nil {
		t.Fatal(err)
	}
	return home, counter
}

func readQueryCounts(t *testing.T, path string) cache.MonthCounter {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var counts cache.MonthCounter
	if err := json.Unmarshal(body, &counts); err != nil {
		t.Fatal(err)
	}
	return counts
}

func queryCommandDaemon(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
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
				if err := serveQueryCommand(conn); err != nil {
					serveErrors <- err
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		<-done
		go func() {
			connections.Wait()
			close(serveErrors)
		}()
		for err := range serveErrors {
			t.Error(err)
		}
	})
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

type queryCommandReadCounter struct {
	io.Reader
	n int
}

func (r *queryCommandReadCounter) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.n += n
	return n, err
}

func isEmptyQueryCommandProbe(n int, err error) bool {
	return n == 0 && (errors.Is(err, io.EOF) || isPeerReset(err))
}

func serveQueryCommand(conn net.Conn) error {
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set query command deadline: %w", err)
	}
	reader := &queryCommandReadCounter{Reader: conn}
	var request model.TCPQuery
	if err := model.NewProtocolReader(reader).Read(&request); err != nil {
		// A cached lookup can exit while its readiness probe is connecting.
		if isEmptyQueryCommandProbe(reader.n, err) {
			return nil
		}
		return fmt.Errorf("read query command request: %w", err)
	}
	var response *model.DaemonResponse
	if request.Action == "ping" {
		response = &model.DaemonResponse{Ping: &model.DaemonPing{
			Available: true, PID: os.Getpid(), Version: VERSION,
			ProtocolVersion: model.DaemonProtocolVersion,
			StartTime:       time.Now().Unix(),
		}}
	} else if request.B != nil {
		result := request.GetResult()
		// "foo bar" (the rewrite of fooBar) and "nothing" are the long texts the
		// stub cannot translate.
		result.Found = result.Query == "hello" ||
			(result.IsLongText && result.Query != "foo bar" && result.Query != "nothing")
		if result.IsLongText {
			result.MachineTrans = "早上好。"
		} else if result.Found {
			result.Keyword, result.Paraphrase = "hello", []string{"你好"}
		}
		response = result.ToDaemonResponse()
		if result.Query == "unavailable" {
			response = &model.DaemonResponse{Error: "在线查询失败"}
		}
	} else {
		return errors.New("query command request has no body")
	}
	if err := model.WriteProtocolMessage(conn, response); err != nil {
		if request.Action == "ping" && isPeerGoneOnWrite(err) {
			return nil
		}
		return fmt.Errorf("write query command response: %w", err)
	}
	return nil
}

func TestQueryCommands(t *testing.T) {
	home, counter := queryCommandFixture(t)
	port := queryCommandDaemon(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name      string
		args      []string
		want      string
		wantQuery string
		wantJSON  bool
		wantAlert bool
		wantError bool
	}{
		{name: "online word frequency", args: []string{"-n", "hello"}, want: "你好", wantAlert: true},
		{name: "cached word frequency", args: []string{"hello"}, want: "你好", wantAlert: true},
		{name: "online long text frequency", args: []string{"-n", "-t", "Good morning."}, want: "早上好。"},
		{name: "cached long text frequency", args: []string{"-t", "Good morning."}, want: "早上好。"},
		{name: "identifier fallback frequency", args: []string{"-n", "getUserProfile"}, want: "早上好。", wantAlert: true},
		{name: "online long text JSON", args: []string{"--json", "-n", "-t", "Good morning."}, wantJSON: true, wantQuery: "Good morning."},
		{name: "cached long text JSON", args: []string{"--json", "-t", "Good morning."}, wantJSON: true, wantQuery: "Good morning."},
		{name: "identifier fallback JSON", args: []string{"--json", "-n", "getUserProfile"}, wantJSON: true, wantQuery: "get user profile"},
		{name: "word JSON compatibility", args: []string{"--json", "-n", "hello"}, wantJSON: true},
		{name: "query error exit code", args: []string{"-n", "unavailable"}, wantError: true},
		{name: "JSON error exit code", args: []string{"--json", "-n", "unavailable"}, wantError: true},
		{name: "not found remains successful", args: []string{"-n", "nonexistent"}, want: "Not found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantCounts := readQueryCounts(t, counter)
			longText := false
			for _, arg := range tt.args {
				longText = longText || arg == "-t"
			}
			if !longText {
				wantCounts[strings.ToLower(tt.args[len(tt.args)-1])]++
			}
			t.Cleanup(func() {
				if got := readQueryCounts(t, counter); !reflect.DeepEqual(got, wantCounts) {
					t.Errorf("query counts = %v, want %v", got, wantCounts)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			args := append([]string{"-test.run=^TestQueryCommandProcess$", "--"}, tt.args...)
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Env = append(os.Environ(), "KD_TEST_QUERY_COMMAND=1", "KD_TEST_DAEMON_PORT="+port,
				"HOME="+home, "USERPROFILE="+home, "KD_ALLOW_ROOT=1")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if ctx.Err() != nil {
				t.Fatalf("query did not finish: %v; stderr=%s", ctx.Err(), &stderr)
			}
			if tt.wantError {
				if cmd.ProcessState.ExitCode() != 1 || stdout.Len() != 0 || strings.Count(stderr.String(), "在线查询失败") != 1 {
					t.Fatalf("error = %v, stdout=%q stderr=%q; want exit 1, empty stdout and one error", err, &stdout, &stderr)
				}
				return
			}
			if err != nil {
				t.Fatalf("query error = %v; stdout=%q stderr=%q", err, &stdout, &stderr)
			}
			if strings.Contains(stderr.String(), "本月第") != tt.wantAlert {
				t.Fatalf("frequency alert = %q, want alert %v", &stderr, tt.wantAlert)
			}
			if !strings.Contains(stdout.String(), tt.want) {
				t.Fatalf("stdout=%q, want %q", &stdout, tt.want)
			}
			if tt.wantJSON {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(stdout.Bytes(), &fields); err != nil || stderr.Len() != 0 {
					t.Fatalf("JSON error = %v, stdout=%q stderr=%q", err, &stdout, &stderr)
				}
				if tt.wantQuery != "" {
					var query, translation string
					if err := json.Unmarshal(fields["q"], &query); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(fields["mt"], &translation); err != nil {
						t.Fatal(err)
					}
					if query != tt.wantQuery || translation != "早上好。" {
						t.Fatalf("query=%q translation=%q", query, translation)
					}
				} else {
					want := `{"k":"hello","pron":null,"para":["你好"],"eg":null,"co":{"star":0,"rank":"","pat":"","li":null}}`
					if strings.TrimSpace(stdout.String()) != want {
						t.Fatalf("word JSON changed: %s", &stdout)
					}
				}
			}
		})
	}
}
