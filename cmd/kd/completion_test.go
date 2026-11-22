package main

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

// completionTokensOf runs a completion request for the words before the
// cursor and returns the candidate tokens, without their descriptions.
func completionTokensOf(t *testing.T, words ...string) ([]string, cliObservation) {
	t.Helper()
	args := append(append([]string{}, words...), completionRequestFlag)
	// completeRoot and urfave/cli's flag completion read os.Args, as they do
	// when the shell runs kd.
	originalArgs := os.Args
	os.Args = append([]string{"kd"}, args...)
	t.Cleanup(func() { os.Args = originalArgs })

	observation, stdout, stderr, err := observeCLI(t, context.Background(), args...)
	if err != nil {
		t.Fatalf("completion %q: error = %v, stderr = %q", words, err, stderr)
	}
	var tokens []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if token, _, _ := strings.Cut(line, ":"); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens, observation
}

func TestCompletionCandidates(t *testing.T) {
	themes := []string{"temp", "wudao", "canvas"}
	tests := []struct {
		name  string
		words []string
		want  []string
	}{
		{name: "flag prefix", words: []string{"--th"}, want: []string{"--theme"}},
		{name: "theme values", words: []string{"--theme"}, want: themes},
		{name: "theme values after short alias", words: []string{"-T"}, want: themes},
		{name: "completion shells", words: []string{"--completion"}, want: []string{"bash", "zsh", "fish", "pwsh"}},
		{name: "flags after a flag value", words: []string{"--theme", "wudao", "--j"}, want: []string{"--json"}},
		{name: "nothing after a complete flag value", words: []string{"--theme", "wudao"}},
		{name: "no flags after the query starts", words: []string{"hello", "-"}},
		{name: "query words are left for later", words: []string{"hel"}},
		{name: "empty command line", words: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, observation := completionTokensOf(t, tt.words...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("candidates = %q, want %q", got, tt.want)
			}
			if observation.rootCalls != 0 {
				t.Fatalf("root action ran %d times during completion", observation.rootCalls)
			}
		})
	}
}

func TestCompletionListsVisibleFlagsOnly(t *testing.T) {
	got, _ := completionTokensOf(t, "-")
	for _, want := range []string{"--text", "--theme", "--completion", "--update"} {
		if !contains(got, want) {
			t.Fatalf("candidates %q lack %s", got, want)
		}
	}
	for _, hidden := range []string{"--server", "--stop", "--restart", "--status", "--log-to-stream"} {
		if contains(got, hidden) {
			t.Fatalf("candidates %q include hidden flag %s", got, hidden)
		}
	}
}

// Pressing Tab after an action flag must not run that action.
func TestCompletionDoesNotRunFlagActions(t *testing.T) {
	for _, flag := range []string{"--update", "--stop", "--daemon", "--generate-config"} {
		t.Run(flag, func(t *testing.T) {
			_, observation := completionTokensOf(t, flag)
			if len(observation.flagActions) != 0 || observation.rootCalls != 0 {
				t.Fatalf("completion ran actions %q (root calls %d)", observation.flagActions, observation.rootCalls)
			}
		})
	}
}

// "completion" is an English word; enabling shell completion must not turn
// it into a subcommand.
func TestCompletionWordIsStillLookedUp(t *testing.T) {
	observation, _, _, err := observeCLI(t, context.Background(), "completion")
	if err != nil {
		t.Fatal(err)
	}
	if observation.rootCalls != 1 || observation.query != "completion" {
		t.Fatalf("query = %q (root calls %d), want the word looked up", observation.query, observation.rootCalls)
	}
}

func TestCompletionScripts(t *testing.T) {
	tests := []struct {
		args []string
		want []string
	}{
		{args: []string{"--completion", "bash"}, want: []string{"complete -o bashdefault -o default -F __kd_bash_autocomplete kd"}},
		{args: []string{"--completion=zsh"}, want: []string{"#compdef kd", "compdef _kd kd"}},
		{args: []string{"--completion", "fish"}, want: []string{"complete -c kd -f -a '(__kd_perform_completion)'"}},
		{args: []string{"--completion", "pwsh"}, want: []string{"Register-ArgumentCompleter -Native -CommandName kd, kd.exe", completionRequestFlag}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			_, stdout, stderr, err := observeCLI(t, context.Background(), tt.args...)
			if err != nil {
				t.Fatalf("error = %v, stderr = %q", err, stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(stdout, want) {
					t.Fatalf("script lacks %q:\n%s", want, stdout)
				}
			}
		})
	}

	_, _, _, err := observeCLI(t, context.Background(), "--completion", "tcsh")
	if err == nil || !strings.Contains(err.Error(), "不支持的shell") {
		t.Fatalf("unsupported shell error = %v", err)
	}
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
