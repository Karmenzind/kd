**当前状态** 初版本测试中，Archlinux已经无障碍使用

-----

:stars: a crystal clear command-line dictionary, written in Go, supported Linux/Win/Mac**

**Go语言实现的简洁好用的命令行词典，跨平台、易于安装、持续维护更新**

本项目受[无道词典](https://github.com/ChestnutHeng/Wudao-dict)启发，在复刻Wudao核心功能的基础上增加了更丰富的特性。我是Wudao的多年用户，日常工作生活重度依赖随手`wd abandon`，但可惜这个项目已经很久未更新，且存在一些可以优化的地方，所以忍不住重写了一个，选择Go是为了方便地解决安装和跨平台问题。

![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/kd_demo.gif)

**TOC**
<!-- vim-markdown-toc GitLab -->

* [特性](#特性)
* [安装](#安装)
    * [Linux/MacOS](#linuxmacos)
    * [Windows](#windows)
* [用法](#用法)
* [配置](#配置)
* [颜色主题](#颜色主题)
* [常见问题和解决方法](#常见问题和解决方法)
    * [MacOS弹出“无法打开”提醒](#macos弹出无法打开提醒)
    * [设置Pager后显示异常](#设置pager后显示异常)
* [进度和计划](#进度和计划)
* [卸载](#卸载)

<!-- vim-markdown-toc -->

## 特性

- 单文件运行，多平台兼容，无需安装任何依赖

    > 理论上可兼容[所有平台/架构](https://gist.github.com/asukakenji/f15ba7e588ac42795f421b48b8aede63)，但我只有Linux/Win可测试，欢迎使用其他平台的朋友试用反馈

- 支持查单词、词组（ :eyes: 长句翻译功能也快写好了）
- 极速响应，超低延迟
- 本地词库（10W热词），可离线使用

    > 运行时后台会自动下载数据库

- 灵活的配置项，支持修改代理、配色等
- 其他小功能：
    - 多次查询相同词汇会出现提醒并加入生词本

        ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/high_freq.png)

    - 支持纯英文模式，只显示英译/英文例句

        ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/en_only.png)

> 更多功能正在开发中 :rocket:

<!-- - `kd --update`参数一键更新 -->

## 安装

**注**:

- 将命令中的`<URL>`**替换**为[releases页面](https://github.com/Karmenzind/kd/releases)中对应你平台和架构的文件链接
- 这里下载位置为示例，可以下载到任何地方，然后将路径加入PATH环境变量


### Linux/MacOS

```bash
sudo sh -c 'curl --create-dirs -L -o /usr/local/bin/kd <URL> && chmod +x /usr/local/bin/kd'
```

<!-- ArchLinux推荐通过AUR一键安装`yay -S aur/kd`，后续直接通过包管理升级 -->

### Windows

用Powershell执行:

```powershell
# 下载文件放入C:\bin
Invoke-WebRequest -uri '<URL>' -OutFile ( New-Item -Path "C:\bin\kd.exe" -Force )
# 将C:\bin加入PATH环境变量
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\bin", "Machine")
```

或手动下载文件，然后通过“计算机->属性->修改环境变量”修改PATH

## 用法

直接执行`kd <text>`查单词、词组（如`kd abandon`、`kd leave me alone`）

<!-- （Windows可能需要`kd.exe`） -->

完整用法如下：

```
❯ kd --help
NAME:
   kd - A crystal clean command-line dictionary.

USAGE:
   kd [global options] [arguments...]

GLOBAL OPTIONS:
   --nocache, -n                        don't use cached result 不使用本地词库，查询网络结果
   --theme value, -T value              choose the color theme for current query 选择颜色主题，仅当前查询生效 (default: temp)
   --server                             start server foreground 在前台启动服务端
   --daemon                             ensure/start the daemon process 启动守护进程
   --update                             check and update kd client 更新kd的可执行文件
   --generate-config                    generate config sample 生成配置文件，默认地址为~/.config/kd.toml
   --edit-config                        edit configuration file with the default editor 用默认编辑器打开配置文件
   --help, -h                           show help
   --version, -v                        print the version
```

## 配置

配置文件地址：Linux/MacOS为~/.config/kd.toml，Windows为~/kd.toml

以下为可配置项和默认值，可执行`kd --generate-config`生成默认配置文件，执行`kd --edit-config`直接用编辑器打开配置文件

<!-- 修改配置文件后，请执行`kd --stop && kd --daemon`重启服务端 -->

```toml
# 是否使用分页器，MacOS上默认false
paging = true
# 分页器命令，例如：less -F / bat / more
pager_command = "less -F"

# 本地最多缓存的单词条数
max_cached = 10000

# 结果中只显示英文（英译、英文例句等）
english_only = false

# 颜色主题，支持：temp/wudao
theme = "temp"

# 格式：http://<IP或域名>:<端口>。设置为空时，系统代理依然会生效
# 如果需要频繁查询长句，可设置此项，否则本地IP有一定概率会被有道服务器暂时屏蔽
http_proxy = ""

# 输出内容前自动清空终端，适合强迫症
clear_screen = false

# 是否开启频率提醒：本月第X次查询xxx
freq_alert = true

# （开发中）安装了emoji字体的可以输出一些emoji字符，just for fun
enable_emoji = true

# 日志配置
[logging]
  # 开启日志记录
  enable = false
  # 默认值：Linux/MacOS为/tmp/kd_<username>.log，windows为%TMPDIR%/kd_<username>.log
  path = ""
  level = "DEBUG"
  stderr = false
```

## 颜色主题

目前支持以下配色，后续会增加更多

- `default` 暂定的默认配色

    ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/theme_default.png)

- `wudao` 复刻Wudao Dict的配色，鲜明易读

    ![](https://raw.githubusercontent.com/Karmenzind/i/master/kd/theme_wudao.png)

## 常见问题和解决方法

### MacOS弹出“无法打开”提醒

如果弹出“无法打开，因为Apple无法检查是否包含恶意软件”的提示，请执行：

```
sudo xattr -r -d com.apple.quarantine <kd文件所在路径>
```

### 设置Pager后显示异常

目前发现MacOS/Debian Bookworm设置分页器后，颜色渲染会失败，尚未解决。如果其他平台遇到此问题，请提交issue

配置文件中设置`paging = false`可规避此问题，如果还想使用分页器，可在shell中设置alias，例如bash/zsh可在bashrc/zshrc中添加：

```bash
__kdwithpager() {
    kd ${@} | less -F
}
alias kd=__kdwithpager
```

## 进度和计划

**进行中**

- 长句翻译
- 通过命令一键更新

**近期**

- 支持bash/zsh/fish补全，包含命令补全和[热词](https://github.com/first20hours/google-10000-english)补全
- 支持查看生词本功能
- 可更新自动提醒
- 支持tmux浮窗模式

**长期**

- 增加多种主题，包含常见配色如Gruvbox/Molokai，仿照bat实现
- 支持全模块自定义显示配置
- 引入多种查询源和词库，如stardict、必应等
- 增加服务端
- 支持通过fzf补全
- Vim插件，浮窗显示查词结果
- 离线词库周期更新

## 卸载

1. 删除kd可执行文件
2. 删除配置文件和缓存目录
    - Linux: `rm -rfv ~/.config/kd.toml ~/.cache/kdcache`
    - MacOS: `rm -rfv ~/.config/kd.toml ~/Library/Caches/kdcache`
    - Win: `rm ~\kd.toml ~\kdcache`
