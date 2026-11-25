# kd

> 2026-07: 近期处于活跃开发阶段，欢迎通过Issue提出建议

:stars: A crystal-clear terminal-native command-line dictionary for Linux, macOS and Windows.

**Go 语言实现的简洁好用的命令行词典，原生面向终端，跨平台、易于安装、持续维护更新**

kd 专注于简洁、高效、丝滑的查询功能，不会发展成集成式的 AI 工具、Agent 或 GUI 应用；现阶段也不会引入 TUI。

保持清晰的边界，把有限的精力持续投入到命令行体验本身，是 kd 的长期设计原则。

[![GitHub Release](https://img.shields.io/github/v/release/karmenzind/kd)](https://github.com/Karmenzind/kd/releases)
[![CI](https://github.com/Karmenzind/kd/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Karmenzind/kd/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Karmenzind/kd/badges/coverage.json)](https://github.com/Karmenzind/kd/actions/workflows/ci.yml)
[![AUR Version](https://img.shields.io/aur/version/kd-bin)](https://aur.archlinux.org/packages/kd-bin)

<!-- <img src="https://raw.githubusercontent.com/Karmenzind/i/master/kd/kd_demo.gif" width="700" align="center"> -->

![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/kd_demo.gif)

**TOC**

<!-- vim-markdown-toc GitLab -->

* [:ballot_box_with_check: 特性](#ballot_box_with_check-特性)
* [🚀 安装和升级](#-安装和升级)
    * [Linux/MacOS](#linuxmacos)
        * [ArchLinux](#archlinux)
    * [Windows](#windows)
    * [其他平台](#其他平台)
    * [卸载](#卸载)
* [⚙️ 用法](#-用法)
    * [配置文件](#配置文件)
    * [更多特性](#更多特性)
        * [Shell补全](#shell补全)
        * [读音功能（experimental）](#读音功能experimental)
* [🎈 提升体验技巧](#-提升体验技巧)
    * [NeoVim插件kd_translate.nvim](#neovim插件kd_translatenvim)
    * [使用tmux的悬浮窗口显示结果](#使用tmux的悬浮窗口显示结果)
    * [通过systemd管理daemon进程](#通过systemd管理daemon进程)
    * [生词本](#生词本)
* [🎨 颜色主题](#-颜色主题)
* [❓ 常见问题和解决方法](#-常见问题和解决方法)
    * [设置less为Pager后显示异常](#设置less为pager后显示异常)
    * [MacOS弹出“无法打开”提醒](#macos弹出无法打开提醒)
* [🤝 参与开发](#-参与开发)
* [📝 进度和计划](#-进度和计划)
* [Thanks to](#thanks-to)

<!-- vim-markdown-toc -->

## :ballot_box_with_check: 特性

- 极速响应，超低延迟 ⚡
- 慢查询会在交互式终端显示查询词和真实查询阶段；快速查询不会闪现加载提示
- 单文件运行，多平台兼容，无需安装任何依赖。Windows运行截图：

    <img src="https://raw.githubusercontent.com/Karmenzind/i/master/kd/win_terminal.png">

- 支持查单词、词组，本地词库（10W+），可离线使用

    > 运行时后台会自动下载数据库

- 支持`-t`翻译长句 👀

    ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/longtext.png)

- 灵活的配置项，支持修改代理、配色等
- 支持`-s`[朗读单词](#读音功能experimental) 📢
- 其他小功能（[详见下文](#-用法)）：
    - 支持纯英文模式，只显示英译/英文例句

        ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/en_only.png)

    - 多次查询相同词汇会出现提醒并加入生词本

        ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/high_freq.png)

    - `kd --update`命令一键更新版本

    - 英文标识符（驼峰、下划线）可拆词翻译，如`kd getUserByID`按`get user by id`查询（需开启配置`split_camelcase_and_snakecase`）

> 更多功能正在开发中 👽

## 🚀 安装和升级

- 这里下载位置为示例，可以下载到任何地方，然后将路径加入PATH环境变量
- 其他架构需求请提交issue
- 系统要求：Linux内核3.2及以上（不支持WSL 1）、macOS 13 Ventura及以上、Windows 10/Windows Server 2016及以上

### Linux/MacOS

在终端中执行：

```bash
bash -c "$(curl -fsSL https://raw.githubusercontent.com/Karmenzind/kd/master/scripts/install.sh)"
```

> 如果raw.githubusercontent.com被屏蔽，改用`git clone https://github.com/Karmenzind/kd && bash kd/install.sh`

<details><summary>或者按照平台/架构复制命令（🖱️ 点击展开）</summary><pre>
```bash
# （如有需要，可以将路径/usr/bin/kd改成/usr/local/bin/kd）
# Linux amd64 (x86-64)
sudo sh -c 'curl --create-dirs -L -o /usr/bin/kd https://github.com/Karmenzind/kd/releases/latest/download/kd_linux_amd64 && chmod +x /usr/bin/kd'
# MacOS arm64 (即M1/M2/M3/M4芯片架构)
sudo sh -c 'curl --create-dirs -L -o /usr/bin/kd https://github.com/Karmenzind/kd/releases/latest/download/kd_macos_arm64 && chmod +x /usr/bin/kd'
# MacOS amd64 (x86-64)
sudo sh -c 'curl --create-dirs -L -o /usr/bin/kd https://github.com/Karmenzind/kd/releases/latest/download/kd_macos_amd64 && chmod +x /usr/bin/kd'
# Linux arm64
sudo sh -c 'curl --create-dirs -L -o /usr/bin/kd https://github.com/Karmenzind/kd/releases/latest/download/kd_linux_arm64 && chmod +x /usr/bin/kd'
```
</pre></details>

#### ArchLinux

[![AUR Version](https://img.shields.io/aur/version/kd-bin)](https://aur.archlinux.org/packages/kd-bin)

ArchLinux推荐直接通过[AUR](https://aur.archlinux.org/packages/kd-bin)安装/更新，例如：`yay -S kd-bin`

### Windows

用非管理员模式的Powershell执行:

```powershell
# 下载文件放入C:\bin，这里是amd64(x86-64)架构
Invoke-WebRequest -uri 'https://github.com/Karmenzind/kd/releases/latest/download/kd_windows_amd64.exe' -OutFile ( New-Item -Path "C:\bin\kd.exe" -Force )
# 将C:\bin加入PATH环境变量
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\bin", "User")
```

> 或手动下载文件，然后通过“计算机->属性->修改环境变量”修改PATH

> [!NOTE]
> kd在Windows上只在PowerShell 7（pwsh）、cmd和Git Bash中进行过稳定性测试。系统自带的旧版Windows PowerShell（5.1及以下）不保证效果，例如查询内容中的双引号会丢失，建议[安装pwsh](https://learn.microsoft.com/zh-cn/powershell/scripting/install/installing-powershell-on-windows)后使用

### 其他平台

**Android**

经测试（感谢@Ze-Wshine协助），用aarch64-linux-android-clang交叉编译的kd可以在Termux（安卓平台上的Linux模拟器APP）运行，可以[在此处下载试用](https://github.com/Karmenzind/kd/issues/33#issuecomment-2447229385)。后续有时间会加入标准编译流程作为release发布。

**自行编译**

其他暂未支持的平台，可创建issue反馈需求。也可以自己动手安装最新Go环境，clone此项目后编译。SQLite 使用纯 Go 的 `modernc.org/sqlite`，请保持 `CGO_ENABLED=0`，无需安装C编译器：

类 Unix 环境：

```bash
CGO_ENABLED=0 go build -mod=readonly -o kd ./cmd/kd
mv kd /usr/bin/kd
```

Windows PowerShell：

```powershell
$env:CGO_ENABLED = "0"
go build -mod=readonly -o kd.exe ./cmd/kd
```

自行编译的kd，版本号取自git（例如`v0.0.15-0.20261125052350-5d1009b610d1`，表示v0.0.14之后的某次提交），无法取得时显示`dev`。`dev`版本无法判断是否有新版本，`kd --update`会提示改用`kd --update --force`安装最新发布版。

### 卸载

<details><summary>🖱️ 点击展开</summary><pre>
1. 删除kd可执行文件（Linux/Mac：/usr/local/bin/kd，Win：C:\bin\kd.exe）
2. 删除配置文件和缓存目录
    - Linux: `rm -rfv ~/.config/kd.toml ~/.cache/kdcache`
    - MacOS: `rm -rfv ~/.config/kd.toml ~/Library/Caches/kdcache`
    - Win（PowerShell）: `Remove-Item -Force ~\kd.toml; Remove-Item -Recurse -Force ~\.cache\kdcache`

如果通过AUR安装，则直接通过AUR管理工具卸载，例如：`yay -Rs kd-bin`
</pre></details>

## ⚙️ 用法

直接执行`kd <text>`查单词、词组（如`kd abandon`、`kd leave me alone`）

<!-- （Windows可能需要`kd.exe`） -->

完整用法如下：

> 此处可能更新不及时，以实际命令输出为准

```
❯ kd --help
NAME:
   kd - A crystal clean command-line dictionary.

USAGE:
   kd [global options]

VERSION:
   v0.1.0

AUTHOR:
   kmz <valesail7@gmail.com>

GLOBAL OPTIONS:
   --text, -t               translate long query with e.g. --text Long time ago 翻译长句
   --json                   output as JSON 输出JSON，长句含q（查询文本）和mt（译文）
   --nocache, -n            don't use cached result 不使用本地词库，查询网络结果
   --theme value, -T value  choose the color theme for current query 选择颜色主题，仅当前查询生效 (default: temp)
   --force, -f              forcely update (only after --update) 强制更新（仅搭配--update）
   --speak, -s              (experimental) read the word with speaker program 单词朗读
   --brief, -b              brief output (omits English explanations and examples) 精简输出(省略英语解释和例句)
   --no-brief               disable brief output 禁用精简输出
   --daemon                 ensure/start the daemon process 启动守护进程
   --update                 check and update kd client 更新kd的可执行文件
   --generate-config        generate config sample 生成配置文件，Linux/Mac默认地址为~/.config/kd.toml，Win为~\kd.toml
   --edit-config            edit configuration file with the default editor 用默认编辑器打开配置文件
   --completion value       print the shell completion script (bash/zsh/fish/pwsh) 输出shell补全脚本
   --help, -h               show help
   --version, -v            print the version
```

说明：

- 开启`split_camelcase_and_snakecase`后，英文标识符式输入会自动拆词后翻译：驼峰如`getUserByID`拆为`get user by id`，下划线如`hello_world`拆为`hello world`。普通查询会先整串查询、未找到再拆分回退；`-t`长句会对句中出现的标识符同样拆分
- JSON模式保留词典数据的简写字段；长句翻译（含标识符拆词后转入的机器翻译）额外包含`q`（实际查询文本）和`mt`（译文），不改变缓存格式
- 查询成功或未找到词条时退出码为0；网络、代理或daemon查询失败时退出码为1，错误写入stderr，JSON模式也遵循此规则
- 查询进度仅用于交互式终端，并在正式结果输出前自动清除。现代终端显示完整动画，能力不明确或较旧的终端会降级为ASCII提示；JSON、管道和重定向输出不会混入动态状态

### 配置文件

📁 配置文件地址：Linux/MacOS为~/.config/kd.toml，Windows为~/kd.toml

以下为可配置项和默认值，可执行`kd --generate-config`生成默认配置文件，执行`kd --edit-config`直接用编辑器打开配置文件

<!-- 修改配置文件后，请执行`kd --stop && kd --daemon`重启服务端 -->

```toml
# 是否使用分页器，MacOS和Debian系上默认false，请自行开启
paging = true
# 分页器命令，例如：less -RF / bat / (不推荐) more -e
# 注意：less命令如果不加上-R，在某些系统/发行版上会产生颜色乱码问题
pager_command = "less -RF"  # Linux默认

# 结果中只显示英文（英译、英文例句等）
english_only = false

# 精简输出，去除英语解释和例句（可使用 --no-brief 参数临时关闭）
brief = false

# 查询英文标识符时先按驼峰/下划线拆词再翻译，如 helloWorld -> hello world；
# 偏编程场景，默认关闭，避免影响普通查询意图
split_camelcase_and_snakecase = false

# 音频缓存目录大小上限（MiB），默认2048（2 GiB）；设为0时朗读后不保留音频
audio_cache_max_size_mb = 2048

# 颜色主题，支持：temp/wudao
theme = "temp"

# 格式：http://<IP或域名>:<端口>。设置为空时，系统代理依然会生效
# 如果需要频繁查询长句，可设置此项，否则本地IP有一定概率会被有道服务器暂时屏蔽
http_proxy = ""

# 输出内容前自动清空终端，适合强迫症
clear_screen = false

# （开发中）安装了emoji字体的可以输出一些emoji字符，just for fun
enable_emoji = true

# 是否开启频率提醒：本月第X次查询xxx
freq_alert = false

# 查词结果中各部分是否显示，默认全部显示；隐藏某一部分不影响其他部分
# brief和english_only照常生效：某部分要两边都允许才会显示
[display]
  # 标题（查询的单词）
  title = true
  # 音标
  pronunciation = true
  # 释义（英译中为中文释义，中译英为英文译文）
  paraphrase = true
  # 柯林斯星级，如★★★
  star = true
  # 考试等级标签，如CET4 TEM4
  rank = true
  # 词形变化，如abandoning,abandoned,abandons
  pattern = true
  # 柯林斯英英释义
  collins = true
  # 例句（柯林斯释义下的例句，以及没有柯林斯释义时显示的例句）
  examples = true

# 日志配置
[logging]
  # 开启日志记录（程序异常时会记录关键信息，不建议关闭）
  enable = true
  # 默认值：Linux/MacOS为/tmp/kd_<username>.log，windows为%TMPDIR%/kd_<username>.log
  path = ""
  # 日志级别，支持：DEBUG/INFO/WARN/PANIC/FATAL
  level = "WARN"
```

配置仅读取上述路径中的 TOML 文件，不再支持 `CONFIGOR_*` 环境变量覆盖、`CONFIGOR_ENV_PREFIX` 自定义前缀、`CONFIGOR_ENV` 额外加载 `kd.<环境名>.toml`、缺少配置文件时回退到 `kd.example.toml`，以及 `CONFIGOR_DEBUG_MODE`、`CONFIGOR_VERBOSE_MODE`、`CONFIGOR_SILENT_MODE`。配置路径存在但不是普通文件（如目录）时会返回读取错误。`pager_command` 拆分失败时会打印警告并直接输出结果；Windows 用户请给命令中的路径加引号，或改用正斜杠。

### 更多特性

#### Shell补全

按Tab可补全参数、主题名（`--theme`后）和shell名（`--completion`后）。单词补全（热词、查询历史）计划在下一阶段实现

| shell | 加载方式 |
|---|---|
| bash | 需要bash-completion（多数Linux发行版默认已安装，Git Bash自带）。在`~/.bashrc`中加入`source <(kd --completion bash)` |
| zsh | 在`~/.zshrc`中`compinit`之后加入`source <(kd --completion zsh)` |
| fish | 执行`kd --completion fish > ~/.config/fish/completions/kd.fish` |
| pwsh | 在`$PROFILE`中加入`kd --completion pwsh \| Out-String \| Invoke-Expression` |

> 输入单词时没有候选项，bash和zsh会退回到补全文件名，这是补全框架的默认行为

#### 读音功能（experimental）

⚠️ 此功能需要访问Google。目前处于实验阶段，请斟酌使用

单词前加speak参数，例如 `kd -s abandon`

音频会缓存在kd的cache目录中。每次使用读音功能时，程序最多每天检查一次缓存总大小；超过
`audio_cache_max_size_mb` 后会优先清理较久未使用的音频。将该值设为`0`可禁用音频缓存，
但不会禁用本次朗读。

**Windows**

- 默认会调用系统默认音频播放程序（如Media Player），会弹出窗口，体验不佳
- 如果要实现命令行直接播放声音，无弹窗干扰，可安装以下之一：
    - mpv，可通过Chocolatey（ `choco install mpvio` ）或参考 https://mpv.io/installation/
    - ffmpeg，可通过Chocolatey（ `choco install ffmpeg` ）或Winget（ `winget install ffmpeg` ）安装


## 🎈 提升体验技巧

### NeoVim插件kd_translate.nvim

🙏 @SilverofLight编写的[NeoVim插件kd_translate.nvim](https://github.com/SilverofLight/kd_translate.nvim)，可以直接在nvim中选中文字查询

![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/kd_translate_nvim.png)

### 使用tmux的悬浮窗口显示结果

如果你在使用tmux，借助悬浮窗口（popup）能让查询体验更舒适友好 🍭

![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/kd_tmux_popup.gif)

在bash/zsh的配置文件中加入：

```bash
if [[ -n $TMUX ]]; then
    __kdwithtmuxpopup() {
        tmux display-popup "kd $@"
    }
    alias kd=__kdwithtmuxpopup
fi
```

### 通过systemd管理daemon进程

为避免每次开机后第一次查询都要等待守护进程启动，可以创建service文件`/usr/lib/systemd/user/kd-server.service`，然后执行`systemctl enable --user kd-server`，daemon进程将随系统自动启动

内容参考[kd-server.service](./scripts/kd-server.service)（请检查service文件中的kd路径是否与实际安装位置一致）

### 生词本

我还在犹豫生词本的具体实现形式，可能考虑作为笔记推送到Notion之类的应用。

目前可以参考[@exaithrg提供的方式](https://github.com/Karmenzind/kd/issues/37)来做一个Repo形式的生词本。

## 🎨 颜色主题

目前支持以下配色，后续会增加更多。如果不希望输出颜色，设置环境变量`NO_COLOR=1`即可

- `temp` 暂定的默认配色

    ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/theme_default.png)

- `canvas` 适合浅色背景

    ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/theme_canvas.png)

- `wudao` 复刻Wudao Dict的配色，鲜明易读

    ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/theme_wudao.png)

## ❓ 常见问题和解决方法

### 设置less为Pager后显示异常

目前发现MacOS/Debian Bookworm设置less为分页器后，颜色渲染会失败，尚未解决。如果其他平台遇到此问题，请提交issue

解决方案：

- 配置文件中设置`pager_command = "less -RF"`（新版本默认）
- 改用其他pager程序，如[bat](https://github.com/sharkdp/bat)/more（安装bat后，在配置文件设置`pager_command = "bat"`）
- 关闭pager，配置文件中设置`paging = false`
- 如果还想使用less作为分页器，可在shell中设置alias，例如bash/zsh可在bashrc/zshrc中添加：
```bash
__kdwithpager() {
  kd ${@} | less -RF
}
alias kd=__kdwithpager
```

### MacOS弹出“无法打开”提醒

如果弹出“无法打开，因为Apple无法检查是否包含恶意软件”的提示，请执行：

```
sudo xattr -r -d com.apple.quarantine <kd文件所在路径>
```

### 以root用户运行时报“不支持Root用户”

在Linux和MacOS上，kd默认拒绝以root用户运行。Docker容器等只有root用户的环境中，如确认需要，可设置环境变量`KD_ALLOW_ROOT=1`：

```bash
KD_ALLOW_ROOT=1 kd hello
```

## 🤝 参与开发

欢迎提交issue和PR。新功能、新配置项、新依赖这类较大的改动，请先开issue讨论。

**构建**：Go版本以`go.mod`为准，始终用`CGO_ENABLED=0`构建（kd以纯Go单文件发布）

```shell
CGO_ENABLED=0 go build -o build/kd ./cmd/kd
```

**提交前**：

```shell
gofmt -w <改动的go文件>
go vet ./...
go test -count=1 ./...
```

改动涉及命令行参数、输出编码、退出码或守护进程启动时，再跑一遍端到端测试，它会在本机的bash、zsh、fish、pwsh、cmd中逐条执行kd（加`-v`可看到每条命令和结果）：

```shell
go test -tags e2e -count=1 ./e2e
```

> [!WARNING]
> 端到端测试会联网，并**停掉你正在运行的kd守护进程**（下次查词时自动重启）；配置和缓存放在临时目录，不会动你自己的数据

**注意事项**：

- 兼容性：配置文件、缓存数据、守护进程协议不能悄悄失效，改动时附带旧格式数据的兼容测试
- 跨平台：功能要同时支持Linux、macOS和Windows
- 输出：查词结果的措辞、排版和颜色属于对外接口，不要随意改动；面向用户的文字用中文
- 配置项和依赖：能不加就不加；新增配置项要同步更新默认值、校验、`--generate-config`示例和本README
- 测试：只用标准库`testing`，单元测试不依赖网络、真实守护进程和你自己的配置
- 提交信息：参照已有写法，如`feat(config): ...`、`fix #85 (youdao): ...`

更完整的项目约定见[AGENTS.md](AGENTS.md)

## 📝 进度和计划

**近期**

- 支持单词补全，包含[热词](https://github.com/first20hours/google-10000-english)和查询历史（参数补全已支持）
- 支持生词本功能

**长期**

- 增加多种主题，包含常见配色如Gruvbox/Molokai，仿照bat实现
- 引入多种查询源和词库，如stardict、bing等
- 增加远程服务端
- 支持通过fzf补全
- Vim插件，浮窗显示查词结果
- 离线词库周期更新


## Thanks to

- 本项目受[无道词典](https://github.com/ChestnutHeng/Wudao-dict)启发，在复刻Wudao核心功能的基础上增加了更丰富的特性。我是Wudao的多年用户，日常工作生活重度依赖随手`wd abandon`，但可惜这个项目已经很久未更新，且存在一些可以优化的地方，所以忍不住重写了一个，选择Go是为了方便地解决安装和跨平台问题。
- 不吝创建Issue、提出建议和意见的各位，让我从这个纯兴趣项目中学到很多，非常感激。
