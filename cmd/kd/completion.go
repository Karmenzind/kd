package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	d "github.com/Karmenzind/kd/pkg/decorate"
	"github.com/urfave/cli/v3"
)

// Shell completion uses urfave/cli's built-in support: the completion
// scripts call kd with completionRequestFlag appended, and completeRoot
// prints the candidates. Only flags and flag values are completed for now;
// completing query words is left for later.

// completionCommandName names the hidden urfave/cli command that prints the
// completion scripts. Its default name, "completion", is an English word that
// `kd completion` must keep looking up, so it gets a name no query can have
// (queries start with a letter, digit or Chinese character). Users reach the
// scripts through --completion instead.
const completionCommandName = "_completion"

const completionRequestFlag = "--generate-shell-completion"

var completionShells = []string{"bash", "zsh", "fish", "pwsh"}

// The pwsh script shipped with urfave/cli only works when saved as kd.ps1 and
// sends the partly typed word along, so kd ships its own that makes the same
// request as the bash, zsh and fish scripts.
//
//go:embed completion.ps1
var pwshCompletionScript string

// configureCompletionCommand replaces urfave/cli's pwsh script with kd's.
func configureCompletionCommand(c *cli.Command) {
	if pwsh := c.Command("pwsh"); pwsh != nil {
		pwsh.Action = func(_ context.Context, cmd *cli.Command) error {
			_, err := io.WriteString(cmd.Root().Writer, pwshCompletionScript)
			return err
		}
	}
}

// flagCompletion prints the completion script for shell.
func flagCompletion(ctx context.Context, cmd *cli.Command, shell string) error {
	if !slices.Contains(completionShells, shell) {
		return fmt.Errorf("不支持的shell：%s，可选：%s", shell, strings.Join(completionShells, "/"))
	}
	completion := cmd.Root().Command(completionCommandName)
	if completion == nil {
		return fmt.Errorf("shell completion is not enabled")
	}
	script := completion.Command(shell)
	return script.Action(ctx, script)
}

// completionValues returns the candidates for a flag that takes a value, or
// nil when token is not such a flag.
func completionValues(token string) []string {
	if !strings.HasPrefix(token, "-") {
		return nil
	}
	switch strings.TrimLeft(token, "-") {
	case "theme", "T":
		return d.Themes
	case "completion":
		return completionShells
	}
	return nil
}

// completeRoot prints completion candidates for the words before the cursor.
// The scripts pass the word being typed only when it starts with "-"; for
// anything else they filter the candidates by that word themselves.
func completeRoot(ctx context.Context, cmd *cli.Command) {
	tokens := completionTokens(os.Args)
	for i := 0; i < len(tokens); i++ {
		if !strings.HasPrefix(tokens[i], "-") {
			// The first query word ends flag parsing (StopOnNthArg), so
			// nothing after it is a flag.
			return
		}
		if completionValues(tokens[i]) != nil {
			i++ // skip the flag's value
		}
	}
	if len(tokens) == 0 {
		return
	}
	last := tokens[len(tokens)-1]
	if values := completionValues(last); values != nil {
		for _, v := range values {
			fmt.Fprintln(cmd.Root().Writer, v)
		}
		return
	}
	if strings.HasPrefix(last, "-") {
		cli.DefaultCompleteWithFlags(ctx, cmd)
	}
}

// completionTokens returns the arguments between the program name and the
// completion request flag.
func completionTokens(args []string) []string {
	if len(args) < 2 {
		return nil
	}
	tokens := args[1:]
	if n := len(tokens); tokens[n-1] == completionRequestFlag {
		tokens = tokens[:n-1]
	}
	return tokens
}
