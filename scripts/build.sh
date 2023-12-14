#!/usr/bin/env bash

CURDIR=$(
	cd $(dirname $0)
	pwd
)
PROJECT_DIR=$(dirname $CURDIR)
cd $PROJECT_DIR

targetdir=${PROJECT_DIR}/build
mkdir -p $targetdir

declare -A d=(
	["darwin"]="amd64 arm64"
	["linux"]="amd64 arm64 386"
	["windows"]="amd64"
)

do_build() {
	local os=$1
	local arch=$2
	local targetfile=$3
	GOOS=$os GOARCH=$arch go build -o ${targetfile} -ldflags="-s -w" -tags urfave_cli_no_docs cmd/kd.go
	echo "    Finished -> ${targetfile}"
}

if [[ $1 == "-a" ]]; then
    rm -rfv $targetdir/*
	for os in "${!d[@]}"; do
		for arch in ${d[$os]}; do
			filename=kd_${os}_${arch}
            [[ $os == "darwin" ]] && filename=kd_macos_${arch}

			targetfile=${targetdir}/${filename}
			[[ $os == "windows" ]] && targetfile=${targetfile}.exe

			echo ">>> Building for $os $arch..."
			do_build $os $arch $targetfile
		done
	done
else
	echo ">>> Building for current platform..."
	do_build '' '' ${PROJECT_DIR}/kd
fi
