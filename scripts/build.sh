#!/usr/bin/env bash

CURDIR=$(cd `dirname $0` && pwd)

PROJECT_DIR=$(dirname $CURDIR)
cd $PROJECT_DIR

targetdir=${PROJECT_DIR}/build
tmpdir=$(mktemp -d)
mkdir -p $targetdir

echook() {
    echo -e "\033[32m$1\033[0m"
}

echoerr() {
    echo -e "\033[31m$@\033[0m"
}

do_build() {
    local os=$1
    local arch=$2
    local targetfile=$3

    local ldflags="-s -w"
    local cgo=1 cc=

    [[ -z "$os" ]] && os=$(go env GOOS)
    [[ -z "$arch" ]] && arch=$(go env GOARCH)

    local targetfilename=
    if [[ -z "$targetfile" ]]; then
        targetfilename=kd_${os}_${arch}
        [[ $os == "darwin" ]] && targetfilename=kd_macos_${arch}
        [[ $os == "windows" ]] && targetfilename=${targetfilename}.exe
        targetfile=${targetdir}/${targetfilename}
    else
        targetfilename=$(basename $targetfile)
    fi
    echo
    echo "≫  Building for $os $arch... -> ${targetfile}"

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
    local targetfiletmp="${tmpdir}/${targetfilename}.tmp"
    GOOS=$os GOARCH=$arch CGO_ENABLED=$cgo CC=$cc go build \
        ${buildopts} \
        -o $targetfiletmp \
        -ldflags="${ldflags}" \
        -tags urfave_cli_no_docs \
        cmd/kd/kd.go
    local buildret=$?
    set +x

    if (($buildret == 0)); then
        echook "    [✔] Finished building -> ${targetfiletmp}"
    else
        echoerr "    [✘] Failed to build for $os $arch"
        return
    fi
    [[ -f ${targetfile} ]] && rm -fv ${targetfile}
    if [[ $os == "darwin" ]]; then
        mv -v ${targetfiletmp} ${targetfile}
    else
        if upx -o ${targetfile} ${targetfiletmp}; then
            echook "    [✔] Finished compression -> ${targetfile}"
        else
            echoerr "    [✘] Failed to compress for $os $arch"
        fi
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
    do_build '' '' ${PROJECT_DIR}/build/kd
    # do_build '' '' /usr/bin/kd
    # do_build '' '' /usr/local/bin/kd
    ;;
-a | --all)
    build_all
    ;;
*)
    do_build $*
    ;;
esac

[[ -d ${tmpdir} ]] && rm -rf $tmpdir && echo "Removed cache dir"
echo 'DONE :)'
