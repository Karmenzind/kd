#!/usr/bin/env bash

CURDIR=$(
	cd $(dirname $0)
	pwd
)
PROJECT_DIR=$(dirname $CURDIR)
cd $PROJECT_DIR

targetdir=${PROJECT_DIR}/build
mkdir -p $targetdir

do_build() {
	local os=$1
	local arch=$2

    local cgo=1 cc=

    local filename=kd_${os}_${arch}
    [[ $os == "darwin" ]] && filename=kd_macos_${arch}

    local targetfile=${targetdir}/${filename}
    [[ $os == "windows" ]] && targetfile=${targetfile}.exe

    case $os in
        windows )
            local cc=x86_64-w64-mingw32-gcc
            local buildopts=-buildmode=c-shared
            ;;
        linux )
            if [[ $arch == "arm64" ]]; then
                local cc=aarch64-linux-gnu-gcc
            fi
            ;;
        darwin)
            # TODO (k): <2023-12-21> 
            ;;
    esac

	GOOS=$os GOARCH=$arch CGO_ENABLED=$cgo CC=$cc go build ${buildopts} -o ${targetfile} -ldflags="-s -w" -tags urfave_cli_no_docs cmd/kd.go
    if (($?==0)); then
        echo "    Finished -> ${targetfile}"
    else
        echo "    Failed to build for $os $arch"
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
			echo ">>> Building for $os $arch..."
			do_build $os $arch $targetfile
		done
	done
}


if [[ $1 == "" ]]; then
	echo ">>> Building for current platform..."
	do_build '' '' ${PROJECT_DIR}/kd
fi

case $1 in
    -a )
        build_all
        ;;
    *)
        do_build $*
        ;;
esac

