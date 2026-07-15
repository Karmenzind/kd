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

	if [[ $targetfile == "" ]] && [[ $os != "" ]] && [[ $arch != "" ]]; then
		echo
		echo "≫  Building for $os $arch..."

		local filename=kd_${os}_${arch}
		[[ $os == "darwin" ]] && filename=kd_macos_${arch}

		local targetfile=${targetdir}/${filename}
		[[ $os == "windows" ]] && targetfile=${targetfile}.exe
	fi

	set -x
	GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -mod=readonly \
        ${buildopts} \
        -o ${targetfile} \
        -ldflags="-s -w" \
        -tags urfave_cli_no_docs \
        cmd/kd/kd.go
	local ret=$?
	set +x

	if (($ret == 0)); then
		echo "    [✔] Finished -> ${targetfile}"
	else
		echo "    [✘] Failed to build for $os $arch"
	fi
	return $ret
}

build_all() {
	local ret=0
	declare -A d=(
		["darwin"]="amd64 arm64"
		["linux"]="amd64 arm64"
		["windows"]="amd64"
	)
	rm -rfv $targetdir/*
	for os in "${!d[@]}"; do
		for arch in ${d[$os]}; do
			do_build "$os" "$arch" "" || ret=1
		done
	done
	return $ret
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
	do_build "$@"
	;;
esac
