//go:build e2e

package e2e

// The suite runs every case in every shell found on this machine. Windows
// PowerShell 5.1 is not part of it; only pwsh is supported.
//
//   - kd is built once with a unique version, and HOME/USERPROFILE/TMP/TEMP/
//     TMPDIR point to a temporary directory, so the user's config, cache,
//     query counters and logs are never touched.
//   - Every shell shares the fixed daemon port, so shells and cases run one
//     after another. A daemon already running on the port is stopped first;
//     kd starts a new one on the user's next query. A daemon managed by
//     systemd is left alone and the suite stops instead.
//   - Results come from the network, so checks look at structure (the query
//     kd received, exit codes, encoding, absence of ANSI codes) rather than
//     exact dictionary text.
//
// A missing shell is skipped. List shells in KD_E2E_SHELLS (for example
// "bash,zsh,fish,pwsh") to make their absence a failure instead.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/pkg/systemd"
)

const commandTimeout = 90 * time.Second

type shell struct {
	name string
	path string
	// command starts line in this shell.
	command func(ctx context.Context, sh *shell, line string) *exec.Cmd
	// quote renders one argument the way a user would type it literally.
	quote func(string) string
	// invoke renders the start of a command line that runs the program at path.
	invoke func(sh *shell, path string) string
	// pipe sends the output of line through the shell's own pipeline.
	pipe func(line string) string
	// tab, when set, returns a script that loads kd's completion script and
	// prints "TAB<tab>line<tab>candidates" for each command line, as if Tab
	// were pressed at its end. It prints "SKIP: reason" when it cannot.
	tab func(sh *shell, lines []string) string
	// noTab explains why tab is not set.
	noTab string
}

// tabCases are command lines completed by pressing Tab at their end.
var tabCases = []struct {
	line string
	want []string
}{
	{"kd --th", []string{"--theme"}},
	{"kd --theme ", []string{"temp", "wudao", "canvas"}},
	{"kd --theme w", []string{"wudao"}},
	{"kd -T c", []string{"canvas"}},
	{"kd --completion ", []string{"bash", "zsh", "fish", "pwsh"}},
	{"kd --theme wudao --j", []string{"--json"}},
	{"kd hello -", nil},
}

type kdCase struct {
	name string
	// desc is the label shown in the progress output.
	desc string
	args []string
	// pipe sends kd's output through the shell's own pipeline before it
	// reaches the test, so the shell has to decode and re-encode it.
	pipe  bool
	env   map[string]string
	exit  int
	check func(stdout, stderr string) error
}

type kdRun struct {
	kd      string
	version string
	env     []string
}

func TestShells(t *testing.T) {
	started := time.Now()
	out := newReporter(t)

	root := t.TempDir()
	home := filepath.Join(root, "home")
	tmp := filepath.Join(root, "tmp")
	for _, dir := range []string{home, tmp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Set for this process too, so daemon status checks and the daemon that
	// kd starts use the same isolated directories.
	for k, v := range map[string]string{"HOME": home, "USERPROFILE": home, "TMP": tmp, "TEMP": tmp, "TMPDIR": tmp} {
		t.Setenv(k, v)
	}
	for _, k := range []string{"NO_COLOR", "PAGER", "KD_DEBUG_PROGRESS"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}

	r := &kdRun{version: fmt.Sprintf("v0.0.0-e2e.%d", time.Now().Unix())}
	out.printf("编译 kd（版本 %s）……", r.version)
	buildStarted := time.Now()
	r.kd = buildKD(t, root, r.version)
	out.printf("完成，%s\n", seconds(time.Since(buildStarted)))
	// Completion scripts call kd by name, so the built kd must come first.
	t.Setenv("PATH", filepath.Dir(r.kd)+string(os.PathListSeparator)+os.Getenv("PATH"))
	r.env = os.Environ()
	out.printf("临时 home 目录：%s\n", home)

	if stopExistingDaemon(t, r) {
		out.printf("已停止 %d 端口上原有的守护进程，你下次查词时 kd 会自动重新启动\n", run.SERVER_PORT)
	}
	t.Cleanup(func() { r.stopDaemon(t) })

	shells, missing := findShells(t)
	if len(shells) == 0 {
		t.Fatal("no shell found")
	}
	if len(missing) > 0 {
		out.printf("没找到、跳过的 shell：%s\n", strings.Join(missing, ", "))
	}

	cases := shellCases(t, root)
	for _, sh := range shells {
		t.Run(sh.name, func(t *testing.T) {
			out.printf("\n[%s] %s\n", sh.name, sh.path)
			// Each shell starts kd's daemon itself, which is the part most
			// sensitive to how the shell launches processes.
			r.stopDaemon(t)
			daemonCase := kdCase{
				name: "starts the daemon and returns",
				desc: "停掉守护进程后查词，kd 自己拉起守护进程并返回",
				args: []string{"--nocache", "abandon"},
				check: func(stdout, _ string) error {
					if !strings.Contains(stdout, "abandon") {
						return errors.New("output lacks the word")
					}
					status := daemon.CheckDaemonStatus(daemon.DefaultAddress())
					if status.State != daemon.StateRunning || status.Ping == nil || status.Ping.Version != r.version {
						return fmt.Errorf("daemon status = %+v, want running version %s", status, r.version)
					}
					return nil
				},
			}
			for _, c := range append([]kdCase{daemonCase}, cases...) {
				line := sh.invoke(sh, r.kd) + " " + quoteArgs(sh, c.args...)
				if c.pipe {
					line = sh.pipe(line)
				}
				caseStarted := time.Now()
				var failure string
				passed := t.Run(c.name, func(t *testing.T) {
					stdout, stderr, code, err := r.runInShell(sh, line, c.env)
					if err == nil && code != c.exit {
						err = fmt.Errorf("exit %d, want %d", code, c.exit)
					}
					if err == nil {
						err = c.check(stdout, stderr)
					}
					if err != nil {
						failure = err.Error()
						t.Fatalf("%v\nline: %s\nstdout:\n%s\nstderr:\n%s", err, line, stdout, stderr)
					}
				})
				out.caseResult(c.desc, strings.Replace(line, sh.invoke(sh, r.kd), "kd", 1), passed, failure, time.Since(caseStarted))
			}
			if sh.tab != nil {
				r.runTabCompletion(t, sh, out)
			} else {
				out.printf("  跳过          按 Tab 补全：%s\n", sh.noTab)
			}
		})
	}
	out.summary(time.Since(started))
}

// runTabCompletion loads kd's completion script in sh and presses Tab at the
// end of each tab case.
func (r *kdRun) runTabCompletion(t *testing.T, sh *shell, out *reporter) {
	lines := make([]string, len(tabCases))
	for i, c := range tabCases {
		lines[i] = c.line
	}
	started := time.Now()
	got := map[string]string{}
	var failure, skipped string
	passed := t.Run("tab completion", func(t *testing.T) {
		script := sh.tab(sh, lines)
		stdout, stderr, code, err := r.runInShell(sh, script, nil)
		if err == nil && code != 0 {
			err = fmt.Errorf("exit %d", code)
		}
		if err != nil {
			failure = err.Error()
			t.Fatalf("%v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		if reason, ok := strings.CutPrefix(strings.TrimSpace(stdout), "SKIP: "); ok {
			skipped = reason
			t.Skip(reason)
		}
		for _, row := range strings.Split(stdout, "\n") {
			if parts := strings.SplitN(strings.TrimRight(row, "\r"), "\t", 3); len(parts) == 3 && parts[0] == "TAB" {
				got[parts[1]] = strings.TrimSpace(parts[2])
			}
		}
		for _, c := range tabCases {
			want := append([]string{}, c.want...)
			have := strings.Fields(got[c.line])
			slices.Sort(want)
			slices.Sort(have)
			if !slices.Equal(want, have) {
				failure = fmt.Sprintf("%q<Tab> gave %q, want %q", c.line, have, want)
				t.Errorf("%s\nstdout:\n%s\nstderr:\n%s", failure, stdout, stderr)
			}
		}
	})
	if skipped != "" {
		out.printf("  跳过          按 Tab 补全：%s\n", skipped)
		return
	}
	out.caseResult("按 Tab 补全参数、主题名和 shell 名", "kd --completion "+sh.name+" 加载后按 Tab", passed, failure, time.Since(started))
	for _, c := range tabCases {
		out.printf("                 %-22s ⇥ %s\n", c.line, got[c.line])
	}
}

// reporter shows progress while the suite runs. go test prints nothing but
// the final "ok" line for a passing package, so without -v the progress goes
// straight to the terminal. Without a terminal (CI) it is dropped; use -v.
type reporter struct {
	w            io.Writer
	passed, fail int
}

func newReporter(t *testing.T) *reporter {
	if testing.Verbose() {
		return &reporter{w: os.Stdout}
	}
	f, err := openTerminal()
	if err != nil {
		return &reporter{w: io.Discard}
	}
	t.Cleanup(func() { f.Close() })
	return &reporter{w: f}
}

func (o *reporter) printf(format string, a ...any) {
	fmt.Fprintf(o.w, format, a...)
}

func (o *reporter) caseResult(desc, line string, passed bool, failure string, d time.Duration) {
	status := "通过"
	if passed {
		o.passed++
	} else {
		o.fail++
		status = "失败"
	}
	o.printf("  %s  %6s  %s\n", status, seconds(d), desc)
	o.printf("                 $ %s\n", line)
	if failure != "" {
		o.printf("                 原因：%s\n", strings.SplitN(failure, "\n", 2)[0])
	}
}

func (o *reporter) summary(d time.Duration) {
	o.printf("\n共 %d 条：%d 条通过，%d 条失败，用时 %s\n", o.passed+o.fail, o.passed, o.fail, seconds(d))
	if o.fail > 0 {
		o.printf("失败详情见下方 go test 的输出\n")
	}
}

func seconds(d time.Duration) string {
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func shellCases(t *testing.T, root string) []kdCase {
	t.Helper()

	// A separate home whose config fails validation; kd exits before it
	// touches the daemon.
	badHome := filepath.Join(root, "bad-home")
	badConfig := "[logging]\nlevel = \"loud\"\n"
	for _, p := range []string{filepath.Join(badHome, "kd.toml"), filepath.Join(badHome, ".config", "kd.toml")} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(badConfig), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	plainWord := func(stdout, stderr string) error {
		if !strings.Contains(stdout, "abandon") || !strings.Contains(stdout, "放弃") {
			return errors.New("output lacks the word or its Chinese paraphrase")
		}
		if strings.Contains(stdout, "\x1b[") || strings.Contains(stderr, "\x1b[") {
			return errors.New("piped output contains ANSI escape codes")
		}
		return nil
	}

	cases := []kdCase{
		{name: "word output is plain text", desc: "查词结果是纯文本：有中文释义，没有颜色码", args: []string{"abandon"}, check: plainWord},
		{name: "word output survives the shell pipeline", desc: "经过 shell 自己的管道后中文不乱码", args: []string{"abandon"}, pipe: true, check: plainWord},
		{
			name: "not found exits 0",
			desc: "查不到的词：输出 Not found，退出码 0",
			args: []string{"zzqxvjkwpq"},
			check: func(stdout, _ string) error {
				if !strings.Contains(stdout, "Not found") {
					return errors.New(`stdout lacks "Not found"`)
				}
				return nil
			},
		},
		{
			// The legacy page finds nothing for aren't; kd must reach it
			// through jsonapi_s, and the quote must survive every shell.
			name: "word with an apostrophe is found",
			desc: "带撇号的词（aren't）能查到中文释义",
			args: []string{"aren't"},
			check: func(stdout, _ string) error {
				if strings.Contains(stdout, "Not found") {
					return errors.New(`stdout says "Not found"`)
				}
				if !strings.Contains(stdout, "aren't") || !strings.ContainsFunc(stdout, isHan) {
					return errors.New("output lacks the word or a Chinese paraphrase")
				}
				return nil
			},
		},
		{
			name: "invalid config exits 1",
			desc: "配置文件写错：报出出错的配置项，退出码 1",
			args: []string{"abandon"},
			env:  map[string]string{"HOME": badHome, "USERPROFILE": badHome},
			exit: 1,
			check: func(_, stderr string) error {
				if !strings.Contains(stderr, "logging.level") {
					return errors.New("stderr does not name the invalid setting")
				}
				return nil
			},
		},
	}

	// Long text JSON echoes the query kd received in "q", which shows
	// whether the shell passed the argument through unchanged.
	for _, tc := range []struct{ name, desc, text string }{
		{name: "embedded quotes", desc: "内嵌引号", text: `it's "fine" today`},
		{name: "percent signs", desc: "百分号", text: "100% sure, 50%d off"},
		{name: "Chinese", desc: "中文", text: "你好，世界"},
		{name: "shell metacharacters", desc: "shell 特殊字符", text: "cost $5 & a|b !x ^y"},
	} {
		want := tc.text
		cases = append(cases, kdCase{
			name: "argument keeps " + tc.name,
			desc: "长句参数原样到达 kd（" + tc.desc + "）",
			args: []string{"-t", "--json", tc.text},
			check: func(stdout, _ string) error {
				var got struct {
					Q  string `json:"q"`
					MT string `json:"mt"`
				}
				if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &got); err != nil {
					return fmt.Errorf("parse JSON: %w", err)
				}
				if got.Q != want {
					return fmt.Errorf("kd received %q, want %q", got.Q, want)
				}
				if got.MT == "" {
					return errors.New("empty machine translation")
				}
				return nil
			},
		})
	}
	return cases
}

func buildKD(t *testing.T, root, version string) string {
	t.Helper()
	out := filepath.Join(root, "bin", "kd")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "-ldflags", "-X main.VERSION="+version, "./cmd/kd")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build kd: %v\n%s", err, b)
	}
	return out
}

// stopExistingDaemon reports whether it stopped a daemon that was running
// before the suite started.
func stopExistingDaemon(t *testing.T, r *kdRun) bool {
	t.Helper()
	switch daemon.CheckDaemonStatus(daemon.DefaultAddress()).State {
	case daemon.StateNotRunning, daemon.StateStaleRuntime:
		return false
	}
	if runtime.GOOS == "linux" {
		if active, _ := systemd.ServiceIsActive(daemon.SYSTEMD_UNIT_NAME, true); active {
			t.Fatalf("kd daemon runs as systemd unit %s; stop it first with `systemctl --user stop %[1]s`", daemon.SYSTEMD_UNIT_NAME)
		}
	}
	r.stopDaemon(t)
	return true
}

func (r *kdRun) stopDaemon(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.kd, "--stop")
	cmd.Env = r.env
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("kd --stop: %v\n%s", err, b)
	}
}

// runInShell returns an error only when the shell could not run line to
// completion; a non-zero exit is reported through the exit code.
func (r *kdRun) runInShell(sh *shell, line string, env map[string]string) (string, string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := sh.command(ctx, sh, line)
	cmd.Env = append([]string{}, r.env...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// A daemon that kept the output pipe open would block Wait forever.
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return stdout.String(), stderr.String(), 0, nil
	case errors.As(err, &exitErr) && ctx.Err() == nil && !errors.Is(err, exec.ErrWaitDelay):
		return stdout.String(), stderr.String(), exitErr.ExitCode(), nil
	case ctx.Err() != nil:
		return stdout.String(), stderr.String(), -1, fmt.Errorf("no result within %s", commandTimeout)
	default:
		return stdout.String(), stderr.String(), -1, fmt.Errorf("run %s: %w", sh.name, err)
	}
}

func quoteArgs(sh *shell, args ...string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = sh.quote(a)
	}
	return strings.Join(quoted, " ")
}

// findShells returns the shells available here and the names of those that
// are missing. Shells named in KD_E2E_SHELLS must be present.
func findShells(t *testing.T) (shells []*shell, missing []string) {
	t.Helper()
	found := map[string]*shell{}
	for _, sh := range candidateShells() {
		if sh.path != "" {
			found[sh.name] = sh
		}
	}
	for _, name := range strings.Split(os.Getenv("KD_E2E_SHELLS"), ",") {
		if name = strings.TrimSpace(name); name != "" && found[name] == nil {
			t.Fatalf("shell %s is listed in KD_E2E_SHELLS but was not found", name)
		}
	}
	for _, sh := range candidateShells() {
		if s := found[sh.name]; s != nil {
			shells = append(shells, s)
		} else {
			missing = append(missing, sh.name)
		}
	}
	return shells, missing
}

func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// bashTab fills COMP_WORDS the way bash does and calls the completion
// function. urfave/cli's bash script needs bash-completion (or the copy of
// its helpers that Git for Windows ships with git-completion.bash).
func bashTab(sh *shell, lines []string) string {
	var b strings.Builder
	b.WriteString(`if ! type _comp_initialize >/dev/null 2>&1 && ! type _get_comp_words_by_ref >/dev/null 2>&1; then
  for f in /usr/share/bash-completion/bash_completion /opt/homebrew/etc/profile.d/bash_completion.sh /usr/local/etc/profile.d/bash_completion.sh /mingw64/share/git/completion/git-completion.bash; do
    [ -f "$f" ] && . "$f"
  done
fi
if ! type _comp_initialize >/dev/null 2>&1 && ! type _get_comp_words_by_ref >/dev/null 2>&1; then
  echo "SKIP: bash-completion is not installed"
  exit 0
fi
source <(kd --completion bash)
tab() {
  local line="$1"
  read -ra COMP_WORDS <<< "$line"
  [[ "$line" == *" " ]] && COMP_WORDS+=("")
  COMP_CWORD=$(( ${#COMP_WORDS[@]} - 1 )) COMP_LINE="$line" COMP_POINT=${#line} COMP_TYPE=9
  COMPREPLY=()
  __kd_bash_autocomplete
  printf 'TAB\t%s\t%s\n' "$line" "${COMPREPLY[*]}"
}
`)
	for _, line := range lines {
		b.WriteString("tab " + sh.quote(line) + "\n")
	}
	return b.String()
}

func fishTab(sh *shell, lines []string) string {
	var b strings.Builder
	b.WriteString("kd --completion fish | source\n")
	for _, line := range lines {
		q := sh.quote(line)
		b.WriteString(`printf 'TAB\t%s\t%s\n' ` + q + ` (string join -- ' ' (complete -C ` + q + ` | string replace -r '\t.*$' ''))` + "\n")
	}
	return b.String()
}

func pwshTab(sh *shell, lines []string) string {
	var b strings.Builder
	b.WriteString("kd --completion pwsh | Out-String | Invoke-Expression\n")
	for _, line := range lines {
		q := sh.quote(line)
		b.WriteString(`"TAB` + "`t" + `{0}` + "`t" + `{1}" -f ` + q + `, ((TabExpansion2 ` + q + ` ` + fmt.Sprint(len(line)) + `).CompletionMatches.CompletionText -join ' ')` + "\n")
	}
	return b.String()
}

func posixShell(name, path string) *shell {
	var tab func(*shell, []string) string
	var noTab string
	if name == "bash" {
		tab = bashTab
	} else {
		noTab = name + " 无法在测试里模拟按 Tab，请手动检查"
	}
	return &shell{
		tab:   tab,
		noTab: noTab,
		name:  name,
		path:  path,
		command: func(ctx context.Context, sh *shell, line string) *exec.Cmd {
			return exec.CommandContext(ctx, sh.path, "-c", line)
		},
		quote:  func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" },
		invoke: func(sh *shell, path string) string { return sh.quote(filepath.ToSlash(path)) },
		pipe:   func(line string) string { return line + " | cat" },
	}
}

func fishShell(path string) *shell {
	sh := posixShell("fish", path)
	sh.tab = fishTab
	sh.quote = func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
	}
	return sh
}

func powerShell(name, path string) *shell {
	return &shell{
		name: name,
		path: path,
		tab:  pwshTab,
		command: func(ctx context.Context, sh *shell, line string) *exec.Cmd {
			// -EncodedCommand keeps the host's own command-line parsing out of
			// the way, so only PowerShell's script parsing is tested.
			script := line + "\nexit $LASTEXITCODE"
			units := utf16.Encode([]rune(script))
			b := make([]byte, 0, len(units)*2)
			for _, u := range units {
				b = append(b, byte(u), byte(u>>8))
			}
			return exec.CommandContext(ctx, sh.path, "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(b))
		},
		quote:  func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" },
		invoke: func(sh *shell, path string) string { return "& " + sh.quote(path) },
		pipe:   func(line string) string { return line + " | ForEach-Object { $_ }" },
	}
}

func isHan(r rune) bool { return unicode.Is(unicode.Han, r) }
