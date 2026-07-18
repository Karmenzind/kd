package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

type cliObservation struct {
	args        []string
	bools       map[string]bool
	theme       string
	themeIsSet  bool
	query       string
	rootCalls   int
	flagActions []string
	forceAtFlag bool
	contextErr  error
}

func observeCLI(t *testing.T, ctx context.Context, args ...string) (cliObservation, string, string, error) {
	t.Helper()
	observation := cliObservation{bools: make(map[string]bool)}
	flagAction := func(name string) boolFlagAction {
		return func(_ context.Context, cmd *cli.Command, _ bool) error {
			observation.flagActions = append(observation.flagActions, name)
			if name == "update" {
				observation.forceAtFlag = cmd.Bool("force")
			}
			return nil
		}
	}
	actions := cliActions{
		server:         flagAction("server"),
		daemon:         flagAction("daemon"),
		stop:           flagAction("stop"),
		restart:        flagAction("restart"),
		update:         flagAction("update"),
		generateConfig: flagAction("generate-config"),
		editConfig:     flagAction("edit-config"),
		status:         flagAction("status"),
		root: func(ctx context.Context, cmd *cli.Command) error {
			observation.rootCalls++
			observation.args = append([]string(nil), cmd.Args().Slice()...)
			for _, name := range []string{"text", "json", "nocache", "force", "speak", "brief", "no-brief", "log-to-stream"} {
				observation.bools[name] = cmd.Bool(name)
			}
			observation.theme = cmd.String("theme")
			observation.themeIsSet = cmd.IsSet("theme")
			observation.query = queryFromCommand(cmd)
			observation.contextErr = ctx.Err()
			return nil
		},
	}
	command := newCLICommand(actions)
	var stdout, stderr bytes.Buffer
	command.Writer = &stdout
	command.ErrWriter = &stderr
	err := command.Run(ctx, append([]string{"kd"}, args...))
	return observation, stdout.String(), stderr.String(), err
}

func TestCLIArgumentAndFlagBaseline(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantArgs   []string
		wantBools  map[string]bool
		wantTheme  string
		themeIsSet bool
		wantQuery  string
	}{
		{name: "single word", args: []string{"word"}, wantArgs: []string{"word"}, wantQuery: "word"},
		{name: "multiple words", args: []string{"leave", "me", "alone"}, wantArgs: []string{"leave", "me", "alone"}, wantQuery: "leave me alone"},
		{name: "quoted phrase from shell", args: []string{"leave  me alone"}, wantArgs: []string{"leave  me alone"}, wantQuery: "leave  me alone"},
		{name: "Unicode", args: []string{"词典", "かな"}, wantArgs: []string{"词典", "かな"}, wantQuery: "词典 かな"},
		{name: "flag before word", args: []string{"--brief", "word"}, wantArgs: []string{"word"}, wantBools: map[string]bool{"brief": true}, wantQuery: "word"},
		{name: "alias flags", args: []string{"-t", "-n", "-b", "-s", "-T", "canvas", "word"}, wantArgs: []string{"word"}, wantBools: map[string]bool{"text": true, "nocache": true, "brief": true, "speak": true}, wantTheme: "canvas", themeIsSet: true, wantQuery: "word"},
		{name: "long query flags", args: []string{"--json", "--text", "--nocache", "--speak", "--brief", "--no-brief", "--log-to-stream", "word"}, wantArgs: []string{"word"}, wantBools: map[string]bool{"json": true, "text": true, "nocache": true, "speak": true, "brief": true, "no-brief": true, "log-to-stream": true}, wantQuery: "word"},
		{name: "unvalidated theme remains accepted", args: []string{"--theme", "custom", "word"}, wantArgs: []string{"word"}, wantTheme: "custom", themeIsSet: true, wantQuery: "word"},
		{name: "both brief flags", args: []string{"--brief", "--no-brief", "word"}, wantArgs: []string{"word"}, wantBools: map[string]bool{"brief": true, "no-brief": true}, wantQuery: "word"},
		{name: "flag after word remains argument", args: []string{"word", "--brief"}, wantArgs: []string{"word", "--brief"}, wantQuery: "word --brief"},
		{name: "string flag after word remains argument", args: []string{"word", "--theme", "canvas"}, wantArgs: []string{"word", "--theme", "canvas"}, wantQuery: "word --theme canvas"},
		{name: "double dash", args: []string{"--", "word"}, wantArgs: []string{"word"}, wantQuery: "word"},
		{name: "hyphen query after double dash", args: []string{"--", "--word"}, wantArgs: []string{"--word"}, wantQuery: "--word"},
		{name: "empty query"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _, err := observeCLI(t, context.Background(), tt.args...)
			if err != nil {
				t.Fatalf("RunContext() error = %v", err)
			}
			if got.rootCalls != 1 {
				t.Fatalf("root action calls = %d, want 1", got.rootCalls)
			}
			if !reflect.DeepEqual(got.args, tt.wantArgs) {
				t.Fatalf("args = %#v, want %#v", got.args, tt.wantArgs)
			}
			if got.query != tt.wantQuery {
				t.Fatalf("query = %q, want %q", got.query, tt.wantQuery)
			}
			for name, want := range tt.wantBools {
				if got.bools[name] != want {
					t.Fatalf("Bool(%q) = %v, want %v", name, got.bools[name], want)
				}
			}
			if got.theme != tt.wantTheme || got.themeIsSet != tt.themeIsSet {
				t.Fatalf("theme = %q (set %v), want %q (set %v)", got.theme, got.themeIsSet, tt.wantTheme, tt.themeIsSet)
			}
		})
	}
}

func TestCLIParseErrorsBaseline(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"--definitely-invalid"}, want: "definitely-invalid"},
		{name: "missing string value", args: []string{"--theme"}, want: "theme"},
		{name: "hyphen query without separator", args: []string{"--word"}, want: "word"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, stdout, stderr, err := observeCLI(t, context.Background(), tt.args...)
			if err == nil {
				t.Fatalf("RunContext() error = nil, stdout=%q stderr=%q", stdout, stderr)
			}
			if got.rootCalls != 0 {
				t.Fatalf("root action calls = %d, want 0", got.rootCalls)
			}
			if !strings.Contains(err.Error()+stdout+stderr, tt.want) {
				t.Fatalf("error/output = %q/%q/%q, missing %q", err, stdout, stderr, tt.want)
			}
		})
	}
}

func TestCLIHelpVersionAndActionErrorsBaseline(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		got, stdout, stderr, err := observeCLI(t, context.Background(), "--help")
		if err != nil || got.rootCalls != 0 || stderr != "" {
			t.Fatalf("help: calls=%d stdout=%q stderr=%q err=%v", got.rootCalls, stdout, stderr, err)
		}
		for _, want := range []string{"NAME:", "kd", "GLOBAL OPTIONS:", "--version", "--brief", "--theme string"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("help output missing %q: %q", want, stdout)
			}
		}
		if strings.Contains(stdout, "completion") || strings.Contains(stdout, "--server") {
			t.Fatalf("help exposes unsupported or hidden behavior: %q", stdout)
		}
	})

	t.Run("version", func(t *testing.T) {
		got, stdout, stderr, err := observeCLI(t, context.Background(), "--version")
		if err != nil || got.rootCalls != 0 || stderr != "" {
			t.Fatalf("version: calls=%d stdout=%q stderr=%q err=%v", got.rootCalls, stdout, stderr, err)
		}
		if !strings.Contains(stdout, "kd version "+VERSION) {
			t.Fatalf("version output = %q", stdout)
		}
	})

	t.Run("root action error", func(t *testing.T) {
		wantErr := errors.New("root failed")
		command := newCLICommand(cliActions{root: func(context.Context, *cli.Command) error { return wantErr }})
		if err := command.Run(context.Background(), []string{"kd", "word"}); !errors.Is(err, wantErr) {
			t.Fatalf("RunContext() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("cancelled context reaches action", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got, _, _, err := observeCLI(t, ctx, "word")
		if err != nil {
			t.Fatalf("RunContext() error = %v", err)
		}
		if !errors.Is(got.contextErr, context.Canceled) {
			t.Fatalf("action context error = %v, want context.Canceled", got.contextErr)
		}
	})
}

func TestCLIManagementFlagActionsBaseline(t *testing.T) {
	for _, name := range []string{"server", "daemon", "stop", "restart", "update", "generate-config", "edit-config", "status"} {
		t.Run(name, func(t *testing.T) {
			got, _, _, err := observeCLI(t, context.Background(), "--"+name)
			if err != nil {
				t.Fatalf("RunContext() error = %v", err)
			}
			if !reflect.DeepEqual(got.flagActions, []string{name}) {
				t.Fatalf("flag actions = %#v, want [%q]", got.flagActions, name)
			}
			if got.rootCalls != 1 {
				t.Fatalf("root action calls = %d, want 1", got.rootCalls)
			}
		})
	}

	for _, args := range [][]string{{"--force", "--update"}, {"--update", "--force"}, {"-f", "--update"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			got, _, _, err := observeCLI(t, context.Background(), args...)
			if err != nil {
				t.Fatal(err)
			}
			if !got.forceAtFlag {
				t.Fatalf("update action did not observe force for %v", args)
			}
		})
	}

	t.Run("flag action error prevents root action", func(t *testing.T) {
		wantErr := errors.New("daemon failed")
		rootCalls := 0
		command := newCLICommand(cliActions{
			daemon: func(context.Context, *cli.Command, bool) error { return wantErr },
			root: func(context.Context, *cli.Command) error {
				rootCalls++
				return nil
			},
		})
		if err := command.Run(context.Background(), []string{"kd", "--daemon"}); !errors.Is(err, wantErr) {
			t.Fatalf("Run() error = %v, want %v", err, wantErr)
		}
		if rootCalls != 0 {
			t.Fatalf("root action calls = %d, want 0", rootCalls)
		}
	})
}

func TestCLIFlagDefaults(t *testing.T) {
	got, _, _, err := observeCLI(t, context.Background(), "word")
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range got.bools {
		if value {
			t.Fatalf("default Bool(%q) = true, want false", name)
		}
	}
	if got.theme != "" || got.themeIsSet {
		t.Fatalf("default theme = %q (set %v), want empty and unset", got.theme, got.themeIsSet)
	}
}

func TestCLICommandConstructionIsIndependent(t *testing.T) {
	first, _, _, err := observeCLI(t, context.Background(), "--brief", "word")
	if err != nil {
		t.Fatal(err)
	}
	second, _, _, err := observeCLI(t, context.Background(), "word")
	if err != nil {
		t.Fatal(err)
	}
	if !first.bools["brief"] || second.bools["brief"] {
		t.Fatalf("flag state leaked between commands: first=%v second=%v", first.bools["brief"], second.bools["brief"])
	}
}
