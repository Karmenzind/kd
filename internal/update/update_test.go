package update

import "testing"

func TestCompareVersions(t *testing.T) {
	for _, tt := range []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{name: "equal", v1: "v1.2.3", v2: "1.2.3", want: 0},
		{name: "newer major", v1: "2.0.0", v2: "v1.99.99", want: 1},
		{name: "newer minor", v1: "1.10.0", v2: "1.9.9", want: 1},
		{name: "older patch", v1: "1.2.2", v2: "1.2.3", want: -1},
		{name: "prerelease below release", v1: "v1.2.3-beta.1", v2: "1.2.3", want: -1},
		{name: "release above prerelease", v1: "v1.2.3", v2: "v1.2.3-rc.1", want: 1},
		{name: "two prereleases", v1: "v1.2.3-rc.2", v2: "v1.2.3-rc.1", want: 0},
		{name: "pseudo-version after a tag", v1: "v0.1.1-0.20261125021722-dbbf2553b64f", v2: "v0.1.0", want: 1},
		{name: "pseudo-version before the next tag", v1: "v0.1.1-0.20261125021722-dbbf2553b64f+dirty", v2: "v0.1.1", want: -1},
		{name: "build metadata ignored", v1: "v1.2.3+dirty", v2: "1.2.3", want: 0},
		{name: "invalid first version", v1: "latest", v2: "1.2.3", want: 0},
		{name: "invalid second version", v1: "1.2.3", v2: "", want: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareVersions(tt.v1, tt.v2); got != tt.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestBinaryURLFor(t *testing.T) {
	for _, tt := range []struct {
		name   string
		goos   string
		goarch string
		want   string
	}{
		{name: "Linux amd64", goos: "linux", goarch: "amd64", want: "kd_linux_amd64"},
		{name: "macOS arm64", goos: "darwin", goarch: "arm64", want: "kd_macos_arm64"},
		{name: "Windows amd64", goos: "windows", goarch: "amd64", want: "kd_windows_amd64.exe"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := binaryURLFor(tt.goos, tt.goarch)
			if got != LATEST_RELEASE_URL+tt.want {
				t.Fatalf("binaryURLFor(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, LATEST_RELEASE_URL+tt.want)
			}
		})
	}
}
