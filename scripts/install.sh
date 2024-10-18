#!/usr/bin/env bash

set -e

unameOut="$(uname -s)"
case "${unameOut}" in
    Linux*)   platform=linux                 ;;
    Darwin*)  platform=macos                 ;;
    CYGWIN*)  platform=cygwin                ;;
    MINGW*)   platform=minGw                 ;;
    MSYS_NT*) platform=git                   ;;
    *)        platform="UNKNOWN:${unameOut}" ;;
esac
echo "☺  Current platform: ${unameOut}"

if ! [[ "${platform}" =~ ^(linux|macos)$ ]]; then
    echo "[✘] 此脚本只支持Linux/MacOS"
fi

check_input() {
    local _range=$1
    local is_valid=no
    local _default=${_range:0:1}
    while [[ $is_valid = 'no' ]]; do
        read -p "请输入: " ans
        [[ -z "$ans" ]] && ans=$_default
        ans=$(echo $ans | tr '[A-Z]' '[a-z]')
        if [[ "$_range" = *$ans* ]]; then
            is_valid=yes
        fi
    done

    [[ -n $2 ]] && read $2 <<<$ans
}

if [[ $platform == linux ]] && [[ $(cat /etc/os-release | grep "^ID=" | sed 's/^ID=//') == "arch" ]]; then
    echo -e "☺  ArchLinux推荐使用AUR安装/升级（\`yay -S kd\`），更方便维护。是否继续使用此脚本安装？[Y/n] "
    check_input yn ans
    [[ $ans == "n" ]] && exit 0
    # TODO (k): <2024-01-02>
fi

case $(uname -m) in
    x86_64 | amd64)
        pkg=kd_${platform}_amd64
        ;;
    aarch64 | arm64)
        pkg=kd_${platform}_arm64
        ;;
    *)
        echo "[✘] 暂时不支持此架构，如有需求请提交issue"
        exit 1
        ;;
esac

LATEST_RELEASE_URL="https://github.com/Karmenzind/kd/releases/latest/download"
# LATEST_RELEASE_URL="http://localhost:8901"

BIN_URL=${LATEST_RELEASE_URL}/${pkg}
TEMP_PATH=/tmp/kd.downloaded

for i in curl wget; do
    command -V $tool >/dev/null && tool=$i && break
done

[[ -z $tool ]] && echo "请安装curl或wget" && exit 1

echo "≫  开始下载文件：$BIN_URL"

case $tool in
    curl)
        curl --create-dirs -L -o ${TEMP_PATH} ${BIN_URL}
        ;;
    wget)
        mkdir -p $(dirname ${TEMP_PATH}) && wget -O ${TEMP_PATH} ${BIN_URL}
        ;;
esac

if (($? != 0)); then
    echo "[✘] 下载失败，请重试"
    exit 1
fi

INST_PATH=/usr/bin/kd

echo "[✔] 已经下载完成，文件临时保存位置：${TEMP_PATH}"
if [[ $(whoami) == "root" ]]; then
    usesudo=0
else
    # if [[ ":$PATH:" == *":$HOME/.local/bin:"* ]]; then
    #     usesudo=0
    #     INST_PATH=$HOME/.local/bin/kd
    #     echo "≫  检测到PATH中包含~/.local/bin，kd将保存到该目录下"
    # else
    #     usesudo=1
    # fi
    usesudo=1
    if [[ ":$PATH:" == *":/usr/local/bin:"* ]]; then
        INST_PATH=/usr/local/bin/kd
        echo "≫  检测到PATH中包含/usr/local/bin，kd将保存到该目录下"
    fi
fi

if (($usesudo == 1)); then
    sudo mkdir -p $(dirname $INST_PATH)
    sudo mv -v $TEMP_PATH $INST_PATH
else
    mkdir -p $(dirname $INST_PATH)
    mv -v $TEMP_PATH $INST_PATH
fi
chmod +x ${INST_PATH} && echo "[✔] 已经添加可执行权限"

echo "≫  测试输出版本号"
${INST_PATH} --version

echo "≫  启动守护进程"
${INST_PATH} --daemon && echo "[✔] DONE :)"
