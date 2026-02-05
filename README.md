:stars: a crystal clear command-line dictionary, written in Go, supported Linux/Win/Mac

**Go语言实现的简洁好用的命令行词典，跨平台、易于安装、持续维护更新**

[![GitHub Release](https://img.shields.io/github/v/release/karmenzind/kd)](https://github.com/Karmenzind/kd/releases)

<!-- <img src="https://raw.githubusercontent.com/Karmenzind/i/master/kd/kd_demo.gif" width="700" align="center"> -->

![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/kd_demo.gif)

本项目受[无道词典](https://github.com/ChestnutHeng/Wudao-dict)启发，在复刻Wudao核心功能的基础上增加了更丰富的特性。我是Wudao的多年用户，日常工作生活重度依赖随手`wd abandon`，但可惜这个项目已经很久未更新，且存在一些可以优化的地方，所以忍不住重写了一个，选择Go是为了方便地解决安装和跨平台问题。

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
* [📝 进度和计划](#-进度和计划)

<!-- vim-markdown-toc -->

## :ballot_box_with_check: 特性

- 极速响应，超低延迟 ⚡
- 单文件运行，多平台兼容，无需安装任何依赖。Windows运行截图：

    <img src="https://raw.githubusercontent.com/Karmenzind/i/master/kd/win_terminal.png">

- 支持查单词、词组，本地词库（10W+），可离线使用

    > 运行时后台会自动下载数据库

- 支持`-t`翻译长句 👀

    ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/longtext.png)

- 灵活的配置项，支持修改代理、配色等
- 支持`-s`[朗读单词](#读音功能experimental) 📢
- 支持`--fzf`使用 fzf 交互式选择单词（需要先安装 [fzf](https://github.com/junegunn/fzf)）🔍
- 其他小功能（[详见下文](#-用法)）：
    - 支持纯英文模式，只显示英译/英文例句

        ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/en_only.png)

    - 多次查询相同词汇会出现提醒并加入生词本

        ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/high_freq.png)

    - `kd --update`命令一键更新版本

> 更多功能正在开发中 👽

## 🚀 安装和升级

- 这里下载位置为示例，可以下载到任何地方，然后将路径加入PATH环境变量
- 其他架构需求请提交issue

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

### 其他平台

**Android**

经测试（感谢@Ze-Wshine协助），用aarch64-linux-android-clang交叉编译的kd可以在Termux（安卓平台上的Linux模拟器APP）运行，可以[在此处下载试用](https://github.com/Karmenzind/kd/issues/33#issuecomment-2447229385)。后续有时间会加入标准编译流程作为release发布。

**自行编译**

其他暂未支持的平台，可创建issue反馈需求。也可以自己动手安装最新Go环境，clone此项目，然后执行以下流程编译安装：

> 此处为类unix环境，具体命令根据平台修改

```bash
go mod tidy
CGO_ENABLED=1 go build -o kd cmd/kd.go
mv kd /usr/bin/kd
```

### 卸载

<details><summary>🖱️ 点击展开</summary><pre>
1. 删除kd可执行文件（Linux/Mac：/usr/local/bin/kd，Win：C:\bin\kd.exe）
2. 删除配置文件和缓存目录
    - Linux: `rm -rfv ~/.config/kd.toml ~/.cache/kdcache`
    - MacOS: `rm -rfv ~/.config/kd.toml ~/Library/Caches/kdcache`
    - Win: `rm ~\kd.toml ~\kdcache`

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
   v0.0.14

AUTHOR:
   kmz <valesail7@gmail.com>

GLOBAL OPTIONS:
   --text TEXT, -t TEXT     translate long query TEXT with e.g. --text="Long time ago" 翻译长句
   --json                   output as JSON
   --nocache, -n            don't use cached result 不使用本地词库，查询网络结果
   --theme value, -T value  choose the color theme for current query 选择颜色主题，仅当前查询生效 (default: temp)
   --force, -f              forcely update (only after --update) 强制更新（仅搭配--update）
   --speak, -s              (experimental) read the word with speaker program 单词朗读
   --fzf                    interactive word selection with fzf 使用 fzf 交互式选择单词
   --daemon                 ensure/start the daemon process 启动守护进程
   --update                 check and update kd client 更新kd的可执行文件
   --generate-config        generate config sample 生成配置文件，Linux/Mac默认地址为~/.config/kd.toml，Win为~\kd.toml
   --edit-config            edit configuration file with the default editor 用默认编辑器打开配置文件
   --help, -h               show help
   --version, -v            print the version
```

说明：

- JSON模式直接输出数据库中raw data，key为简写，暂不打算优化为易读形式

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

# 日志配置
[logging]
  # 开启日志记录（程序异常时会记录关键信息，不建议关闭）
  enable = true
  # 默认值：Linux/MacOS为/tmp/kd_<username>.log，windows为%TMPDIR%/kd_<username>.log
  path = ""
  # 日志级别，支持：DEBUG/INFO/WARN/PANIC/FATAL
  level = "WARN"
```

### 更多特性

#### 读音功能（experimental）

⚠️ 此功能需要访问Google。目前处于实验阶段，请斟酌使用

单词前加speak参数，例如 `kd -s abandon`

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

## 📝 进度和计划

**近期**

- 支持bash/zsh/fish补全，包含命令补全和[热词](https://github.com/first20hours/google-10000-english)补全
- 支持生词本功能

**长期**

- 增加多种主题，包含常见配色如Gruvbox/Molokai，仿照bat实现
- 支持全模块自定义显示配置
- 引入多种查询源和词库，如stardict、bing等
- 增加远程服务端
- 支持通过fzf补全
- Vim插件，浮窗显示查词结果
- 离线词库周期更新

