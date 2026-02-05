package internal

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal/cache"
	"go.uber.org/zap"
)

// CheckFzfExists checks if fzf is available in PATH
func CheckFzfExists() error {
	_, err := exec.LookPath("fzf")
	if err != nil {
		return fmt.Errorf("fzf 未找到，请先安装 fzf (https://github.com/junegunn/fzf)")
	}
	return nil
}

// FzfInteractiveQuery shows an fzf interface to select and query a word
func FzfInteractiveQuery() (string, error) {
	// Check if fzf exists
	if err := CheckFzfExists(); err != nil {
		return "", err
	}

	// Get all words from database
	words, err := cache.GetAllWords()
	if err != nil {
		return "", fmt.Errorf("获取词库失败：%s", err)
	}

	if len(words) == 0 {
		return "", fmt.Errorf("词库为空，请先查询一些单词或等待数据库下载完成")
	}

	zap.S().Debugf("Got %d words from database", len(words))

	// Prepare fzf input
	input := strings.Join(words, "\n")

	// Build fzf arguments from config
	args := buildFzfArgs(&config.Cfg.Fzf)

	// Run fzf
	cmd := exec.Command("fzf", args...)

	cmd.Stdin = strings.NewReader(input)
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	if err != nil {
		// User cancelled (Ctrl-C or ESC)
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 130 {
				return "", fmt.Errorf("已取消")
			}
		}
		zap.S().Warnf("fzf error: %s, stderr: %s", err, errBuf.String())
		return "", fmt.Errorf("fzf 执行失败：%s", err)
	}

	selected := strings.TrimSpace(outBuf.String())
	if selected == "" {
		return "", fmt.Errorf("未选择任何单词")
	}

	return selected, nil
}

// buildFzfArgs builds fzf command arguments from config
func buildFzfArgs(cfg *config.FzfConfig) []string {
	args := []string{}

	// Apply simple options first
	if cfg.Height != "" {
		args = append(args, "--height", cfg.Height)
	}

	if cfg.Reverse {
		args = append(args, "--reverse")
	}

	if cfg.Border {
		args = append(args, "--border")
	}

	if cfg.Prompt != "" {
		args = append(args, "--prompt", cfg.Prompt)
	}

	if cfg.Info != "" {
		args = append(args, "--info", cfg.Info)
	}

	if cfg.Preview {
		// Basic preview support - can be extended
		args = append(args, "--preview-window", "right:50%:wrap")
	} else {
		args = append(args, "--preview-window", "hidden")
	}

	// Append custom args (low priority - as additional options)
	if len(cfg.CustomArgs) > 0 {
		args = append(args, cfg.CustomArgs...)
	}

	return args
}
