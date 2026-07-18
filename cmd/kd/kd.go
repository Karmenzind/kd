package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal"
	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/core"
	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/query"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/internal/tts"
	"github.com/Karmenzind/kd/internal/ui"
	"github.com/Karmenzind/kd/internal/update"
	"github.com/Karmenzind/kd/logger"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	// "github.com/kyokomi/emoji/v2"
)

var VERSION = "v0.0.14"

func showPrompt() {
	exename, err := pkg.GetExecutableBasename()
	if err != nil {
		d.EchoFatal("%s", err)
	}
	fmt.Printf(`%[1]s <text>	查单词、词组
%[1]s -t <text>	查长句
%[1]s -h    	查看详细帮助
`, exename)
}

func shouldEnableQueryProgress(jsonOutput, logToStream, terminal bool) bool {
	return terminal && !jsonOutput && !logToStream
}

func queryFromCommand(cmd *cli.Command) string {
	return strings.Join(cmd.Args().Slice(), " ")
}

type debugProgressDelay struct {
	ui.Progress
	delay time.Duration
}

func (p debugProgressDelay) Start(state ui.State) {
	p.Progress.Start(state)
	time.Sleep(p.delay)
}

func (p debugProgressDelay) Update(state ui.State) {
	p.Progress.Update(state)
	time.Sleep(p.delay)
}

var um = map[string]string{
	"text":            "translate long query with e.g. --text Long time ago 翻译长句",
	"nocache":         "don't use cached result 不使用本地词库，查询网络结果",
	"force":           "forcely update (only after --update) 强制更新（仅搭配--update）",
	"theme":           "choose the color theme for current query 选择颜色主题，仅当前查询生效",
	"json":            "output as JSON",
	"init":            "initialize shell completion 初始化部分设置，例如shell的自动补全",
	"server":          "start server foreground 在前台启动服务端",
	"daemon":          "ensure/start the daemon process 启动守护进程",
	"stop":            "stop the daemon process 停止守护进程",
	"restart":         "restart the daemon process 重新启动守护进程",
	"update":          "check and update kd client 更新kd的可执行文件",
	"speak":           "(experimental) read the word with speaker program 单词朗读",
	"brief":           "brief output (omits English explanations and examples) 精简输出(省略英语解释和例句)",
	"no-brief":        "disable brief output 禁用精简输出",
	"generate-config": "generate config sample 生成配置文件，Linux/Mac默认地址为~/.config/kd.toml，Win为~\\kd.toml",
	"edit-config":     "edit configuration file with the default editor 用默认编辑器打开配置文件",
	"status":          "show running status 展示运行信息",
	"log-to-stream":   "redirect logging output to stdout&stderr (for debugging or server mode)",
}

//  -----------------------------------------------------------------------------
//  cli flag actions
//  -----------------------------------------------------------------------------

func flagServer(context.Context, *cli.Command, bool) (err error) {
	err = internal.StartServer()
	if errors.Is(err, daemon.ErrPortOccupied) {
		return fmt.Errorf("daemon端口已被其他程序占用: %w", err)
	}
	return err
}

func flagDaemon(context.Context, *cli.Command, bool) (err error) {
	err = daemon.StartDaemonProcess()
	switch {
	case errors.Is(err, daemon.ErrDaemonStartTimeout):
		if logger.LOG_FILE != "" {
			return fmt.Errorf("守护进程启动超时，请查看日志 %s: %w", logger.LOG_FILE, err)
		}
		return fmt.Errorf("守护进程启动超时: %w", err)
	case errors.Is(err, daemon.ErrProtocolIncompatible):
		return fmt.Errorf("已有daemon协议版本不兼容，请手动停止已确认的旧daemon进程后重试: %w", err)
	case errors.Is(err, daemon.ErrPortOccupied):
		return fmt.Errorf("daemon端口已被其他程序占用: %w", err)
	case errors.Is(err, daemon.ErrDaemonInit):
		return fmt.Errorf("守护进程初始化失败: %w", err)
	default:
		return err
	}
}

func flagStop(context.Context, *cli.Command, bool) (err error) {
	if err = daemon.KillDaemonIfRunning(); err != nil {
		return daemonControlError(err)
	}
	return
}

func flagRestart(context.Context, *cli.Command, bool) error {
	return daemonControlError(daemon.RestartDaemon())
}

func daemonControlError(err error) error {
	switch {
	case errors.Is(err, daemon.ErrProtocolIncompatible):
		return fmt.Errorf("旧版daemon不支持安全停止，请手动结束已确认的旧daemon进程: %w", err)
	case errors.Is(err, daemon.ErrPortOccupied), errors.Is(err, daemon.ErrNotKDDaemon):
		return fmt.Errorf("daemon端口由其他程序占用，未执行进程终止操作: %w", err)
	default:
		return err
	}
}

func flagUpdate(_ context.Context, cmd *cli.Command, _ bool) (err error) {
	var ver string
	if runtime.GOOS == "linux" && run.Info.GetOSInfo().Distro == "arch" {
		d.EchoFine("您在使用ArchLinux，推荐直接通过AUR安装/升级（例如`yay -S kd`），更便于维护")
	}
	force := cmd.Bool("force")
	if force {
		d.EchoRun("开始强制更新")
	}
	doUpdate := force
	if !doUpdate {
		ver, err = update.GetNewerVersion(VERSION)
		if err != nil {
			d.EchoError("%s", err)
			return
		}
		if ver != "" {
			prompt := fmt.Sprintf("Found new version (%s). Update?", ver)
			if pkg.AskYN(prompt) {
				doUpdate = true
			} else {
				fmt.Println("Canceled.", d.B(d.Green(":)")))
				return nil
			}
		} else {
			fmt.Println("You're using the latest version.")
			return nil
		}
	}

	if doUpdate {
		// emoji.Println(":lightning: Let's update now")
		if err = daemon.KillDaemonIfRunning(); err != nil {
			warnMsg := "可能会影响后续文件替换。如果出现问题，请手动执行`kd --stop`后重试"
			d.EchoWarn("停止守护进程出现异常（%s），%s", err, warnMsg)
			if p, perr := daemon.FindServerProcess(); perr == nil {
				if p == nil {
					d.EchoOkay("守护进程已确认停止")
				} else {
					d.EchoWarn("守护进程（PID %v）未能停止，%s", p.Pid, warnMsg)
				}
			}
		}
		err = update.UpdateBinary(VERSION)
	}
	return err
}

func writeConfigFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

func ensureDefaultConfigFile(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("检查配置文件失败: %w", err)
	}

	content, err := config.GenerateDefaultConfig()
	if err != nil {
		return false, fmt.Errorf("生成默认配置失败: %w", err)
	}
	if err := writeConfigFile(path, content); err != nil {
		return false, err
	}
	return true, nil
}

func flagGenerateConfig(context.Context, *cli.Command, bool) (err error) {
	if pkg.IsPathExists(config.CONFIG_PATH) {
		if !pkg.AskYN(fmt.Sprintf("配置文件%s已经存在，是否覆盖？", config.CONFIG_PATH)) {
			d.EchoFine("已取消")
			return
		}
	}
	conf, err := config.GenerateDefaultConfig()
	if err != nil {
		d.EchoFatal("%s", err)
	}
	d.EchoRun("以下默认配置将会被写入配置文件，路径为%s", config.CONFIG_PATH)
	fmt.Println(conf)
	if !pkg.AskYN("是否继续？") {
		d.EchoFine("已取消")
		return
	}

	if err = writeConfigFile(config.CONFIG_PATH, conf); err != nil {
		return err
	}
	d.EchoOkay("已经写入配置文件")
	return
}

func flagEditConfig(context.Context, *cli.Command, bool) error {
	var err error
	var cmd *exec.Cmd
	p := config.CONFIG_PATH
	created, err := ensureDefaultConfigFile(p)
	if err != nil {
		return err
	}
	if created {
		d.EchoOkay("配置文件不存在，已生成默认配置：%s", p)
	}
	switch runtime.GOOS {
	case "linux", "darwin":
		for _, k := range []string{"VISUAL", "EDITOR"} {
			if env := os.Getenv(k); env != "" {
				d.EchoRun("找到预设%s：%s，正在启动", k, env)
				cmd = exec.Command(env, p)
				break
			}
		}
		if cmd == nil {
			if runtime.GOOS == "darwin" {
				cmd = exec.Command("open", "-e", p)
			} else {
				for _, k := range []string{"nano", "vim", "vi"} {
					d.EchoRun("未找到EDITOR或VISUAL环境变量，尝试启动编辑器%s", k)
					if pkg.CommandExists(k) {
						cmd = exec.Command(k, p)
						break
					}
				}
				if cmd == nil {
					return errors.New("未找到nano或vim，请安装至少一种，或者指定环境变量EDITOR/VISUAL")
				}
			}
		}
	case "windows":
		cmd = exec.Command("notepad", p)
	default:
		return fmt.Errorf("暂不支持为当前操作系统%s自动打开编辑器，请提交issue反馈", runtime.GOOS)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	return err
}

type daemonStatus struct {
	Running bool
	PID     int
	Port    string
	State   daemon.State
}

func resolveDaemonStatus(status daemon.Status) daemonStatus {
	result := daemonStatus{State: status.State}
	if status.State == daemon.StateRunning && status.Ping != nil {
		result.Running = true
		result.PID = status.Ping.PID
		if status.Runtime != nil && status.Runtime.PID == status.Ping.PID {
			result.Port = status.Runtime.Port
		}
	}
	return result
}

func writeStatus(out io.Writer, status daemonStatus) error {
	d.EchoRun("运行和相关配置信息如下：")
	fmt.Fprintf(out, "    版本：%s\n", VERSION)
	if status.Running {
		fmt.Fprintln(out, "    Daemon状态：运行中")
		fmt.Fprintf(out, "    Daemon PID：%d\n", status.PID)
		if status.Port != "" {
			fmt.Fprintf(out, "    Daemon端口：%s\n", status.Port)
		}
	} else {
		switch status.State {
		case daemon.StateStaleRuntime:
			fmt.Fprintln(out, "    Daemon状态：未运行（存在失效运行信息）")
		case daemon.StatePortOccupied:
			fmt.Fprintln(out, "    Daemon状态：不可用（端口被其他程序占用）")
		case daemon.StateIncompatible:
			fmt.Fprintln(out, "    Daemon状态：不可用（协议版本不兼容）")
		case daemon.StateUnresponsive:
			fmt.Fprintln(out, "    Daemon状态：不可用（无响应）")
		case daemon.StateRuntimeError:
			fmt.Fprintln(out, "    Daemon状态：未运行（运行信息损坏）")
		default:
			fmt.Fprintln(out, "    Daemon状态：未运行")
		}
	}
	fmt.Fprintf(out, "    配置文件地址：%s\n", config.CONFIG_PATH)
	fmt.Fprintf(out, "    数据文件目录：%s\n", cache.CACHE_ROOT_PATH)
	fmt.Fprintf(out, "    Log地址：%s\n", logger.LOG_FILE)
	kdpath, err := pkg.GetExecutablePath()
	if err == nil {
		fmt.Fprintf(out, "    Binary地址：%s\n", kdpath)
	}
	return err
}

func flagStatus(context.Context, *cli.Command, bool) error {
	return writeStatus(os.Stdout, resolveDaemonStatus(daemon.CheckDaemonStatus(daemon.DefaultAddress())))
}

func checkAndNoticeUpdate() {
	if ltag := update.GetCachedLatestTag(); ltag != "" {
		if update.CompareVersions(ltag, VERSION) == 1 {
			prompt := fmt.Sprintf("发现新版本%s，请执行`kd --update`更新", ltag)
			if run.Info.GetOSInfo().Distro == "arch" {
				prompt += "。ArchLinux推荐通过AUR安装/升级"
			}
			d.EchoWeakNotice("%s", prompt)
		}
	}
}

func basicCheck() {
	if runtime.GOOS != "windows" {
		if u, _ := user.Current(); u.Username == "root" {
			if os.Getenv("KD_ALLOW_ROOT") != "1" {
				d.EchoWrong("不支持Root用户。如确认需要，请设置环境变量 KD_ALLOW_ROOT=1")
				os.Exit(1)
			}
		}
	}

	// XXX (k): <2024-01-01>
	// if exename, err := pkg.GetExecutableBasename(); err == nil {
	// 	if exename != "kd" {
	// 		d.EchoWrong("请将名字改成kd")
	// 		os.Exit(1)
	// 	}
	// } else {
	// 	d.EchoError(err.Error())
	// }
}

type boolFlagAction func(context.Context, *cli.Command, bool) error

type cliActions struct {
	server         boolFlagAction
	daemon         boolFlagAction
	stop           boolFlagAction
	restart        boolFlagAction
	update         boolFlagAction
	generateConfig boolFlagAction
	editConfig     boolFlagAction
	status         boolFlagAction
	root           cli.ActionFunc
}

func newCLICommand(actions cliActions) *cli.Command {
	stopAfterFirstArg := 1
	return &cli.Command{
		Suggest:         true, // XXX
		Name:            "kd",
		Version:         VERSION,
		Usage:           "A crystal clean command-line dictionary.",
		HideHelpCommand: true,
		Authors:         []any{"kmz <valesail7@gmail.com>"},
		StopOnNthArg:    &stopAfterFirstArg,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "text", Aliases: []string{"t"}, HideDefault: true, Usage: um["text"]},
			&cli.BoolFlag{Name: "json", HideDefault: true, Usage: um["json"]},
			&cli.BoolFlag{Name: "nocache", Aliases: []string{"n"}, HideDefault: true, Usage: um["nocache"]},
			&cli.StringFlag{Name: "theme", Aliases: []string{"T"}, DefaultText: "temp", Usage: um["theme"]},
			&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, HideDefault: true, Usage: um["force"]},
			&cli.BoolFlag{Name: "speak", Aliases: []string{"s"}, HideDefault: true, Usage: um["speak"]},
			&cli.BoolFlag{Name: "brief", Aliases: []string{"b"}, HideDefault: true, Usage: um["brief"]},
			&cli.BoolFlag{Name: "no-brief", HideDefault: true, Usage: um["no-brief"]},

			// BoolFlags as commands
			// &cli.BoolFlag{Name: "init", HideDefault: true, Hidden: true, Usage: um["init"]},
			&cli.BoolFlag{Name: "server", HideDefault: true, Action: actions.server, Hidden: true, Usage: um["server"]},
			&cli.BoolFlag{Name: "daemon", HideDefault: true, Action: actions.daemon, Usage: um["daemon"]},
			&cli.BoolFlag{Name: "stop", HideDefault: true, Hidden: true, Action: actions.stop, Usage: um["stop"]},
			&cli.BoolFlag{Name: "restart", HideDefault: true, Hidden: true, Action: actions.restart, Usage: um["restart"]},
			&cli.BoolFlag{Name: "update", HideDefault: true, Action: actions.update, Usage: um["update"]},
			&cli.BoolFlag{Name: "generate-config", HideDefault: true, Action: actions.generateConfig, Usage: um["generate-config"]},
			&cli.BoolFlag{Name: "edit-config", HideDefault: true, Action: actions.editConfig, Usage: um["edit-config"]},
			&cli.BoolFlag{Name: "status", HideDefault: true, Action: actions.status, Hidden: true, Usage: um["status"]},
			&cli.BoolFlag{Name: "log-to-stream", HideDefault: true, Hidden: true, Usage: um["log-to-stream"]},
		},
		Action: actions.root,
	}
}

func main() {
	basicCheck()
	if err := run.EnsureCacheDirs(); err != nil {
		d.EchoFatal("%s", err)
	}
	if err := config.InitConfig(); err != nil {
		if !pkg.HasAnyFlag("status", "edit-config", "generate-config") { // XXX (k): <2024-10-18 22:35> 可能不够
			d.EchoFatal("%s", err)
		}
		d.EchoWarn("%s", err)
	}
	cfg := config.Cfg
	d.ApplyConfig(cfg.EnableEmoji)
	run.Info.Version = VERSION

	if cfg.Logging.Enable {
		component := "client"
		if pkg.HasAnyFlag("server") {
			component = "server"
		}
		l, err := logger.InitLogger(&cfg.Logging, component)
		if err != nil {
			d.EchoFatal("%s", err)
		}
		defer func() { _ = l.Sync() }()
		defer func() {
			if r := recover(); r != nil {
				zap.S().Errorln("Application crashed", zap.Any("reason", r))
				if syncErr := l.Sync(); syncErr != nil {
					fmt.Printf("Failed to sync logger: %v\n", syncErr)
				}
			}
		}()
	}
	zap.S().Debugf("Got configuration: %+v", cfg)
	zap.S().Debugf("Got run info: %+v", run.Info)

	if err := cache.InitDB(); err != nil {
		d.EchoFatal("%s", err)
	}
	defer cache.LiteDB.Close()
	defer core.WG.Wait()

	command := newCLICommand(cliActions{
		server:         flagServer,
		daemon:         flagDaemon,
		stop:           flagStop,
		restart:        flagRestart,
		update:         flagUpdate,
		generateConfig: flagGenerateConfig,
		editConfig:     flagEditConfig,
		status:         flagStatus,
		root: func(ctx context.Context, cmd *cli.Command) error {
			// 这里BoolFlag都当subcommand用
			if !cmd.Bool("update") && !cmd.Bool("json") {
				defer checkAndNoticeUpdate()
			}

			if pkg.HasAnyFlag("init", "server", "daemon", "stop", "restart", "update", "generate-config", "edit-config", "status") {
				return nil
			}

			if cfg.FileExists {
				status := daemon.CheckDaemonStatus(daemon.DefaultAddress())
				if status.State == daemon.StateRunning && status.Ping != nil && cfg.ModTime > status.Ping.StartTime {
					if cmd.Bool("json") {
						if err := daemon.RestartDaemonQuiet(); err != nil {
							zap.S().Warnf("Failed to restart daemon after config change: %s", err)
						}
					} else {
						d.EchoWarn("检测到配置文件发生修改，正在重启守护进程")
						flagRestart(ctx, cmd, true)
					}
				}
			}

			if cmd.String("theme") != "" {
				cfg.Theme = cmd.String("theme")
			}
			d.ApplyTheme(cfg.Theme)

			if cmd.Args().Len() > 0 {
				zap.S().Debugf("Recieved Arguments (len: %d): %+v", cmd.Args().Len(), cmd.Args().Slice())
				// emoji.Printf("Test emoji:\n:accept: :inbox_tray: :information: :us: :uk:  🗣  :lips: :eyes: :balloon: \n")
				if cfg.ClearScreen && !cmd.Bool("json") {
					pkg.ClearScreen()
				}

				qstr := queryFromCommand(cmd)
				capabilities := ui.DetectCapabilities(os.Stderr)
				interactiveTerminal := capabilities.Level != ui.CapabilityPlain && ui.IsTerminal(os.Stdout)
				progress := ui.NewProgress(ctx, ui.Options{
					Writer:       os.Stderr,
					Enabled:      shouldEnableQueryProgress(cmd.Bool("json"), cmd.Bool("log-to-stream"), interactiveTerminal),
					Capabilities: capabilities,
				})
				if interactiveTerminal && os.Getenv("KD_DEBUG_PROGRESS") == "1" {
					progress = debugProgressDelay{Progress: progress, delay: 1200 * time.Millisecond}
				}
				progress.Start(ui.State{Query: qstr, Phase: ui.PhaseStarting})
				defer progress.Stop()

				if r, err := internal.QueryWithProgress(qstr, cmd.Bool("nocache"), cmd.Bool("text"), progress); err == nil {
					if cmd.Bool("json") {
						progress.Stop()
						if j, jsonErr := json.Marshal(r); jsonErr == nil {
							fmt.Println(string(j))
							return nil
						} else {
							return fmt.Errorf("转化JSON失败：%s", jsonErr)
						}
					}

					var formatted string
					if r.Found {
						brief := cfg.Brief
						if cmd.Bool("brief") {
							brief = true
						}
						if cmd.Bool("no-brief") {
							brief = false
						}
						progress.Update(ui.State{Query: r.Query, Phase: ui.PhaseFormatting})
						formatted = query.PrettyFormat(r, cfg.EnglishOnly, brief)
					}
					progress.Stop()

					if cfg.FreqAlert {
						if h := <-r.History; h > 3 {
							d.EchoWarn("本月第%d次查询`%s`", h, r.Query)
						}
					}
					if r.Found {
						if err = pkg.OutputResult(formatted, cfg.Paging, cfg.PagerCommand); err != nil {
							d.EchoFatal("%s", err)
						}
						if cmd.Bool("speak") {
							if cmd.Bool("text") {
								d.EchoWarn("读音功能暂不支持长文本模式")
							} else {
								if err = tts.Speak(qstr); err != nil {
									d.EchoWarn("发音功能报错：%s", err)
									zap.S().Warnf("Failed to read the word. Error: %s", err)
								}
							}
						}
					} else {
						if r.Prompt != "" {
							d.EchoWrong("%s", r.Prompt)
						} else {
							fmt.Println("Not found", d.Yellow(":("))
						}
					}
				} else {
					progress.Stop()
					d.EchoError("%s", err)
					zap.S().Errorf("%+v", err)
				}
			} else {
				showPrompt()
			}
			return nil
		},
	})

	if err := command.Run(context.Background(), os.Args); err != nil {
		zap.S().Errorf("APP stopped: %s", err)
		d.EchoError("%s", err)
		os.Exit(1)
	}
}
