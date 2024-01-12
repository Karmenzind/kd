package internal

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/core"
	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/model"
	q "github.com/Karmenzind/kd/internal/query"
	"github.com/Karmenzind/kd/internal/run"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"github.com/Karmenzind/kd/pkg/str"
	"go.uber.org/zap"
)

func ensureDaemon(running chan bool) {
	p, _ := daemon.FindServerProcess()
	var err error
	if p == nil {
		d.EchoRun("未找到守护进程，正在启动...")
		err = daemon.StartDaemonProcess()
		if err != nil {
			d.EchoFatal(err.Error())
		}
	} else {
		var warn string
		// recorded daemon info
		recDi, err := daemon.GetDaemonInfo()
		if err == nil && run.Info.Version != recDi.Version {
			warn = fmt.Sprintf("正在运行的守护程序版本（%s）与当前程序（%s）不一致", recDi.Version, run.Info.Version)
		} else if daemonExepath, _ := p.Exe(); run.Info.ExePath != daemonExepath {
			warn = fmt.Sprintf("正在运行的守护程序（%s）与当前程序（%s）文件路径不一致", daemonExepath, run.Info.ExePath)
			// err := proc.KillProcess(p)
			// if err != nil {
			// 	cmd := proc.GetKillCMD(p.Pid)
			// 	d.EchoError("停止进程%v失败，请手动执行：", p.Pid)
			// 	fmt.Println(cmd.String())
			// 	os.Exit(1)
			// }
			// d.EchoRun("已终止，正在启动新的守护进程...")
			// err = daemon.StartDaemonProcess()
			// if err != nil {
			// 	d.EchoFatal(err.Error())
			// }
		}
		if warn != "" {
			d.EchoWarn(warn + "，建议执行`kd --restart`重启")
		}
	}
	running <- true
}

func Query(query string, noCache bool, longText bool) (r *model.Result, err error) {
	// TODO (k): <2024-01-02> regexp
	query = str.Simplify(query)
	if !longText {
		query = strings.ToLower(query)
	}

	r = buildResult(query, longText)
	r.History = make(chan int, 1)

	daemonRunning := make(chan bool)
	go ensureDaemon(daemonRunning)

	if !longText {
		core.WG.Add(1)
		go cache.CounterIncr(query, r.History)
	}

	// any valid char
	if m, _ := regexp.MatchString("^[a-zA-Z0-9\u4e00-\u9fa5]", query); !m {
		r.Found = false
		r.Prompt = "请输入有效查询字符或参数"
		return
	}

	var inNotFound bool
	var line int
	if !longText {
		line, err = cache.CheckNotFound(r.Query)
		if err != nil {
			zap.S().Warnf("[cache] check not found error: %s", err)
		} else if line > 0 {
			if !noCache {
				r.Found = false
				zap.S().Debugf("`%s` is in not-found-list", r.Query)
				return
			}
			inNotFound = true
		}
		r.Initialize()
	}

	if !noCache {
		if cacheErr := q.FetchCached(r); cacheErr == nil && r.Found {
			return
		}
	}

	if <-daemonRunning {
		err = QueryDaemon(r)
		if err == nil && r.Found && inNotFound {
			go cache.RemoveNotFound(r.Query)
		}
	} else {
		d.EchoFatal("守护进程未启动，请手动执行`kd --daemon`")
	}

	return r, err
}

func QueryDaemon(r *model.Result) error {
	addr := fmt.Sprintf("localhost:%d", run.SERVER_PORT)
	err := q.QueryDaemon(addr, r)
	return err
}
