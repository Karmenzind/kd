package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal"
	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/core"
	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/query"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/internal/tts"
	"github.com/Karmenzind/kd/internal/update"
	"github.com/Karmenzind/kd/logger"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	// "github.com/kyokomi/emoji/v2"
)

var VERSION = "v0.0.14.dev"

func showPrompt() {
	exename, err := pkg.GetExecutableBasename()
	if err != nil {
		d.EchoFatal(err.Error())
	}
	fmt.Printf(`%[1]s <text>	查单词、词组
%[1]s -t <text>	查长句
%[1]s -h    	查看详细帮助
`, exename)
}

var um = map[string]string{
	"text":            "translate long query `TEXT` with e.g. --text=\"Long time ago\" 翻译长句",
	"nocache":         "don't use cached result 不使用本地词库，查询网络结果",
	"force":           "forcely update (only after --update) 强制更新（仅搭配--update）",
	"theme":           "choose the color theme for current query 选择颜色主题，仅当前查询生效",
	"init":            "initialize shell completion 初始化部分设置，例如shell的自动补全",
	"server":          "start server foreground 在前台启动服务端",
	"daemon":          "ensure/start the daemon process 启动守护进程",
	"stop":            "stop the daemon process 停止守护进程",
	"restart":         "restart the daemon process 重新启动守护进程",
	"update":          "check and update kd client 更新kd的可执行文件",
	"speak":           "(experimental) read the word with speaker program 单词朗读",
	"generate-config": "generate config sample 生成配置文件，Linux/Mac默认地址为~/.config/kd.toml，Win为~\\kd.toml",
	"edit-config":     "edit configuration file with the default editor 用默认编辑器打开配置文件",
	"status":          "show running status 展示运行信息",
	"log-to-stream":   "redirect logging output to stdout&stderr (for debugging or server mode)",
}

//  -----------------------------------------------------------------------------
//  cli flag actions
//  -----------------------------------------------------------------------------

func flagServer(*cli.Context, bool) (err error) {
	err = internal.StartServer()
	if strings.Contains(err.Error(), "address already in use") {
		return fmt.Errorf("端口已经被占用（%s）", err)
	}
	return
}

func flagDaemon(*cli.Context, bool) (err error) {
	p, _ := daemon.FindServerProcess()
	if p != nil {
		d.EchoWrong("已存在运行中的守护进程，PID：%d。请先执行`kd --stop`停止该进程", p.Pid)
		return
	}
	if err := daemon.StartDaemonProcess(); err != nil {
		d.EchoFatal(err.Error())
	}
	return
}

func flagStop(*cli.Context, bool) (err error) {
	if err = daemon.KillDaemonIfRunning(); err != nil {
		d.EchoFatal(err.Error())
	}
	return
}

func flagRestart(*cli.Context, bool) error {
	return daemon.RestartDaemon()
}

func flagUpdate(ctx *cli.Context, _ bool) (err error) {
	var ver string
	if runtime.GOOS == "linux" && run.Info.GetOSInfo().Distro == "arch" {
		d.EchoFine("您在使用ArchLinux，推荐直接通过AUR安装/升级（例如`yay -S kd`），更便于维护")
	}
	force := ctx.Bool("force")
	if force {
		d.EchoRun("开始强制更新")
	}
	doUpdate := force
	if !doUpdate {
		ver, err = update.GetNewerVersion(VERSION)
		if err != nil {
			d.EchoError(err.Error())
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

func flagGenerateConfig(*cli.Context, bool) (err error) {
	if pkg.IsPathExists(config.CONFIG_PATH) {
		if !pkg.AskYN(fmt.Sprintf("配置文件%s已经存在，是否覆盖？", config.CONFIG_PATH)) {
			d.EchoFine("已取消")
			return
		}
	}
	conf, err := config.GenerateDefaultConfig()
	if err != nil {
		d.EchoFatal(err.Error())
	}
	d.EchoRun("以下默认配置将会被写入配置文件，路径为" + config.CONFIG_PATH)
	fmt.Println(conf)
	if !pkg.AskYN("是否继续？") {
		d.EchoFine("已取消")
		return
	}

	os.WriteFile(config.CONFIG_PATH, []byte(conf), os.ModePerm)
	d.EchoOkay("已经写入配置文件")
	return
}

func flagEditConfig(ctx *cli.Context, b bool) error {
	var err error
	var cmd *exec.Cmd
	p := config.CONFIG_PATH
	if !pkg.IsPathExists(p) {
		d.EchoRun("检测到配置文件不存在")
		err = flagGenerateConfig(ctx, b)
		if err != nil || !pkg.IsPathExists(p) {
			return err
		}
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

func flagStatus(*cli.Context, bool) error {
	di, _ := daemon.GetDaemonInfo()
	d.EchoRun("运行和相关配置信息如下：")
	fmt.Printf("    版本：%s\n", VERSION)
	fmt.Printf("    Daemon端口：%s\n", di.Port)
	fmt.Printf("    Daemon PID：%d\n", di.PID)
	fmt.Printf("    配置文件地址：%s\n", config.CONFIG_PATH)
	fmt.Printf("    数据文件目录：%s\n", cache.CACHE_ROOT_PATH)
	fmt.Printf("    Log地址：%s\n", logger.LOG_FILE)
	kdpath, err := pkg.GetExecutablePath()
	if err == nil {
		fmt.Printf("    Binary地址：%s\n", kdpath)
	}
	return err
}

func checkAndNoticeUpdate() {
	if ltag := update.GetCachedLatestTag(); ltag != "" {
		if update.CompareVersions(ltag, VERSION) == 1 {
			prompt := fmt.Sprintf("发现新版本%s，请执行`kd --update`更新", ltag)
			if run.Info.GetOSInfo().Distro == "arch" {
				prompt += "。ArchLinux推荐通过AUR安装/升级"
			}
			d.EchoWeakNotice(prompt)
		}
	}
}

func basicCheck() {
	if runtime.GOOS != "windows" {
		if u, _ := user.Current(); u.Username == "root" {
			d.EchoWrong("不支持Root用户")
			os.Exit(1)
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

func main() {
	basicCheck()
	if err := config.InitConfig(); err != nil {
		if !pkg.HasAnyFlag("status", "edit-config", "generate-config") { // XXX (k): <2024-10-18 22:35> 可能不够
			d.EchoFatal(err.Error())
		}
		d.EchoWarn(err.Error())
	}
	cfg := config.Cfg
	d.ApplyConfig(cfg.EnableEmoji)
	run.Info.Version = VERSION

	if cfg.Logging.Enable {
		l, err := logger.InitLogger(&cfg.Logging)
		if err != nil {
			d.EchoFatal(err.Error())
		}
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
		d.EchoFatal(err.Error())
	}
	defer cache.LiteDB.Close()
	defer core.WG.Wait()

	app := &cli.App{
		Suggest:         true, // XXX
		Name:            "kd",
		Version:         VERSION,
		Usage:           "A crystal clean command-line dictionary.",
		HideHelpCommand: true,
		// EnableBashCompletion: true,

		Authors: []*cli.Author{{Name: "kmz", Email: "valesail7@gmail.com"}},
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "text", Aliases: []string{"t"}, DisableDefaultText: true, Usage: um["text"]},
			&cli.BoolFlag{Name: "nocache", Aliases: []string{"n"}, DisableDefaultText: true, Usage: um["nocache"]},
			&cli.StringFlag{Name: "theme", Aliases: []string{"T"}, DefaultText: "temp", Usage: um["theme"]},
			&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, DisableDefaultText: true, Usage: um["force"]},
			&cli.BoolFlag{Name: "speak", Aliases: []string{"s"}, DisableDefaultText: true, Usage: um["speak"]},

			// BoolFlags as commands
			// &cli.BoolFlag{Name: "init", DisableDefaultText: true, Hidden: true, Usage: um["init"]},
			&cli.BoolFlag{Name: "server", DisableDefaultText: true, Action: flagServer, Hidden: true, Usage: um["server"]},
			&cli.BoolFlag{Name: "daemon", DisableDefaultText: true, Action: flagDaemon, Usage: um["daemon"]},
			&cli.BoolFlag{Name: "stop", DisableDefaultText: true, Hidden: true, Action: flagStop, Usage: um["stop"]},
			&cli.BoolFlag{Name: "restart", DisableDefaultText: true, Hidden: true, Action: flagRestart, Usage: um["restart"]},
			&cli.BoolFlag{Name: "update", DisableDefaultText: true, Action: flagUpdate, Usage: um["update"]},
			&cli.BoolFlag{Name: "generate-config", DisableDefaultText: true, Action: flagGenerateConfig, Usage: um["generate-config"]},
			&cli.BoolFlag{Name: "edit-config", DisableDefaultText: true, Action: flagEditConfig, Usage: um["edit-config"]},
			&cli.BoolFlag{Name: "status", DisableDefaultText: true, Hidden: true, Action: flagStatus, Usage: um["status"]},
			&cli.BoolFlag{Name: "log-to-stream", DisableDefaultText: true, Hidden: true, Usage: um["log-to-stream"]},
		},
		Action: func(cCtx *cli.Context) error {
			// 这里BoolFlag都当subcommand用
			if !cCtx.Bool("update") {
				defer checkAndNoticeUpdate()
			}

			if pkg.HasAnyFlag("init", "server", "daemon", "stop", "restart", "update", "generate-config", "edit-config", "status") {
				return nil
			}

			if cfg.FileExists {
				di, err := daemon.GetDaemonInfo()
				if err == nil && cfg.ModTime > di.StartTime {
					d.EchoWarn("检测到配置文件发生修改，正在重启守护进程")
					flagRestart(cCtx, true)
				}
			}

			if cCtx.String("theme") != "" {
				cfg.Theme = cCtx.String("theme")
			}
			d.ApplyTheme(cfg.Theme)

			if cCtx.Args().Len() > 0 {
				zap.S().Debugf("Recieved Arguments (len: %d): %+v", cCtx.Args().Len(), cCtx.Args().Slice())
				// emoji.Printf("Test emoji:\n:accept: :inbox_tray: :information: :us: :uk:  🗣  :lips: :eyes: :balloon: \n")
				if cfg.ClearScreen {
					pkg.ClearScreen()
				}

				qstr := strings.Join(cCtx.Args().Slice(), " ")

				if r, err := internal.Query(qstr, cCtx.Bool("nocache"), cCtx.Bool("text")); err == nil {
					if cfg.FreqAlert {
						if h := <-r.History; h > 3 {
							d.EchoWarn(fmt.Sprintf("本月第%d次查询`%s`", h, r.Query))
						}
					}
					if r.Found {
						err = pkg.OutputResult(query.PrettyFormat(r, cfg.EnglishOnly), cfg.Paging, cfg.PagerCommand)
						if err != nil {
							d.EchoFatal(err.Error())
						}
						if cCtx.Bool("speak") {
							if cCtx.Bool("text") {
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
							d.EchoWrong(r.Prompt)
						} else {
							fmt.Println("Not found", d.Yellow(":("))
						}
					}
				} else {
					d.EchoError(err.Error())
					zap.S().Errorf("%+v", err)
				}
			} else {
				showPrompt()
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		zap.S().Errorf("APP stopped: %s", err)
		d.EchoError(err.Error())
	}
}
