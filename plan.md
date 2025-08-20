# master plan

## wip
- Docker mode
- 下载audio增加timeout
- 读文件加锁
- 指定port
- release增加version，aur判断此文件
- gitea镜像
- -t查询本地缓存增加清理，例如保留最近1000条

## short-term
- logger加上server/client标识
- edit时候文件不存在，自动创建
- 重启改为signal
- 多source直接嵌套进列表
- 记录pid/port，先检查这两个
- 预留一个接口调用，获取重要信息
- 用代理爬词库
- use hash as cache file's name for long query
- json储存所有查过的单词最后访问时间

## Long-term
- 引入stardict源
- 增加服务端
- 自动更新UA数据
- 加入词库设置，供选择词库大小

## low priority
- --update下载之后缓存，避免重复下载
- cli替换为cobra
- source数据 分为base-sourse & web-source
- 刷数据，去掉音标[]
- server增加信号处理，做一些善后处理like删除文件
- 检测配置保存时间变化的基础上再加上内容判断？
- not found list记录查询时间，超时删除
- not found和索引都加入服务端缓存，benchmark比较直接查本地和tcp通信的速度
- AUR收尾工作
- move default log to cache dir

# BUG

- `--status` shows port and pid of dead daemon
- disable pager on Debian/MacOS

```
panic: assignment to entry in nil map

goroutine 27 [running]:
github.com/Karmenzind/kd/internal/cache.UpdateLongTextCache(0xc000294400)
        /home/runner/work/kd/kd/internal/cache/cache.go:121 +0x10c
created by github.com/Karmenzind/kd/internal/query.FetchOnline in goroutine 26
        /home/runner/work/kd/kd/internal/query/online.go:114 +0x1a8
```

## Risk
- 实际文件名 不改的时候的process_name，增加同时校验kd和当前文件名

## low priority
- (action) linux 386

## 写入release介绍

架构选择：
- 常用的64位Intel/AMD选择amd64
- 32位选择386
- Mac m1/2/3芯片选择arm64架构
- 注意区分amd64和arm64

如果没有你所使用的平台/架构，请提交issue反馈

如果下载受阻，请前往gitee备份页面


## Others


```
panic: assignment to entry in nil map

goroutine 651 [running]:
github.com/Karmenzind/kd/internal/cache.UpdateLongTextCache(0xc000340400)
        /app/internal/cache/cache.go:121 +0x10c
created by github.com/Karmenzind/kd/internal/query.FetchOnline in goroutine 664
        /app/internal/query/online.go:114 +0x1a8

```

```

2025-06-29T11:48:13.051Z        WARN    daemon/cron.go:139      Failed to download zip file: Get "https://gitee.com/void_kmz/kd/releases/download/v0.0.1/kd_data.zip": tls: failed to verify certificate: x509: certificate signed by unknown authority
github.com/Karmenzind/kd/internal/daemon.cronUpdateDataZip.func1
        /app/internal/daemon/cron.go:139

```

```

2025-06-29T11:47:20.166Z        WARN    model/others.go:41  Failed to fetch os info: %s. (Current GOOS: %s)open /etc/os-release: no such file or directorylinux
github.com/Karmenzind/kd/internal/model.(*RunInfo).SetOSInfo
        /app/internal/model/others.go:41
github.com/Karmenzind/kd/internal.StartServer
        /app/internal/server.go:40
main.flagServer
        /app/cmd/kd/kd.go:66
github.com/urfave/cli/v2.(*BoolFlag).RunAction
        /app/vendor/github.com/urfave/cli/v2/flag_bool.go:101
github.com/urfave/cli/v2.runFlagActions
        /app/vendor/github.com/urfave/cli/v2/app.go:484
github.com/urfave/cli/v2.(*Command).Run
        /app/vendor/github.com/urfave/cli/v2/command.go:224
github.com/urfave/cli/v2.(*App).RunContext
        /app/vendor/github.com/urfave/cli/v2/app.go:333
github.com/urfave/cli/v2.(*App).Run
        /app/vendor/github.com/urfave/cli/v2/app.go:307
main.main
        /app/cmd/kd/kd.go:404
runtime.main
        /usr/local/go/src/runtime/proc.go:272


```

