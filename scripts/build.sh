#!/usr/bin/env bash

CURDIR=$(cd `dirname $0` && pwd)

PROJECT_DIR=$(dirname $CURDIR)
cd $PROJECT_DIR

targetdir=${PROJECT_DIR}/build
mkdir -p $targetdir

get_os_type() {
    local os_type="$(uname -s)"
    case "$os_type" in
        Darwin)
            echo "darwin"
            ;;
        Linux)
            echo "linux"
            ;;
        MINGW*|MSYS*|CYGWIN*)
            echo "windows"
            ;;
    esac
}

do_build() {
    local os=$1
    local arch=$2
    local targetfile=$3
    local ldflags="-s -w"

    [[ -z $os ]] && os=$(get_os_type)
    [[ -z $arch ]] && arch=$(go env GOARCH)

    local cgo=1 cc=

    if [[ $targetfile == "" ]] && [[ $os != "" ]] && [[ $arch != "" ]]; then
        echo
        echo "≫  Building for $os $arch..."

        local filename=kd_${os}_${arch}
        [[ $os == "darwin" ]] && filename=kd_macos_${arch}

        local targetfile=${targetdir}/${filename}
        [[ $os == "windows" ]] && targetfile=${targetfile}.exe
    fi

    case $os in
    windows)
        cc=x86_64-w64-mingw32-gcc
        # local buildopts=-buildmode=c-shared
        ;;
    linux)
        if [[ -f /etc/alpine-release ]]; then
            cc=gcc
            cc=aarch64-alpine-linux-musl-gcc
        else
            if [[ $arch == "arm64" ]]; then
                cc=aarch64-linux-musl-gcc
            else
                cc=musl-gcc
            fi
        fi
        ldflags="-extldflags \"-static\" ${ldflags}" \
        ;;
    darwin)
        # TODO (k): <2023-12-21>
        ;;
    esac

    echo "Using: GOOS=$os GOARCH=$arch CGO_ENABLED=$cgo CC=$cc"

    set -x
    GOOS=$os GOARCH=$arch CGO_ENABLED=$cgo CC=$cc go build \
        ${buildopts} \
        -o ${targetfile} \
        -ldflags="${ldflags}" \
        -tags urfave_cli_no_docs \
        cmd/kd/kd.go
    local ret=$?
    set +x

    if (($ret == 0)); then
        echo "    [✔] Finished -> ${targetfile}"
    else
        echo "    [✘] Failed to build for $os $arch"
    fi
}

build_all() {
    declare -A d=(
        ["darwin"]="amd64 arm64"
        ["linux"]="amd64 arm64"
        ["windows"]="amd64"
    )
    rm -rfv $targetdir/*
    for os in "${!d[@]}"; do
        for arch in ${d[$os]}; do
            do_build $os $arch $targetfile
        done
    done
}

case $1 in
"")
    echo ">>> Building for current workspace..."
    # do_build '' '' ${PROJECT_DIR}/kd
    # do_build '' '' /usr/bin/kd
    do_build '' '' /usr/local/bin/kd
    exit
    ;;
-a | --all)
    build_all
    ;;
*)
    do_build $*
    ;;
esac
