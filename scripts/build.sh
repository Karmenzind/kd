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
	local targetfile=$3

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
		local cc=x86_64-w64-mingw32-gcc
		# local buildopts=-buildmode=c-shared
		;;
	linux)
		if [[ $arch == "arm64" ]]; then
			local cc=aarch64-linux-gnu-gcc
		fi
		;;
	darwin)
		# TODO (k): <2023-12-21>
		;;
	esac

	set -x
	GOOS=$os GOARCH=$arch CGO_ENABLED=$cgo CC=$cc go build \
        ${buildopts} \
        -o ${targetfile} \
        -ldflags="-s -w" \
        -tags urfave_cli_no_docs \
        -mod=vendor \
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
