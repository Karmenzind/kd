package main

import "runtime/debug"

// VERSION is kd's version. Release builds set it to the git tag at link time
// (scripts/build.sh passes -ldflags "-X main.VERSION=$KD_VERSION"). Other
// builds take the version Go records for the main module: the tag for
// `go install ...@vX.Y.Z`, or a pseudo-version derived from git for a local
// `go build`. Without either it is "dev", which never counts as a release.
var VERSION string

func init() {
	VERSION = resolveVersion(VERSION, moduleVersion())
}

func resolveVersion(linked, module string) string {
	if linked != "" {
		return linked
	}
	if module != "" && module != "(devel)" {
		return module
	}
	return "dev"
}

func moduleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return ""
}
