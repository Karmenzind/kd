package main

import (
	"os"
	"testing"
)

// testVersion stands in for a release version: a test binary has no module
// version, and --update and the daemon version checks need a release here.
const testVersion = "v0.1.0"

func TestMain(m *testing.M) {
	VERSION = testVersion
	os.Exit(m.Run())
}

func TestResolveVersion(t *testing.T) {
	for _, tt := range []struct {
		name, linked, module, want string
	}{
		{name: "linked tag wins", linked: "v0.1.0", module: "v0.0.15-0.20261125021722-dbbf2553b64f", want: "v0.1.0"},
		{name: "go install tag", module: "v0.1.0", want: "v0.1.0"},
		{name: "git pseudo-version", module: "v0.1.1-0.20261125021722-dbbf2553b64f+dirty", want: "v0.1.1-0.20261125021722-dbbf2553b64f+dirty"},
		{name: "no version control", module: "(devel)", want: "dev"},
		{name: "no build info", want: "dev"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.linked, tt.module); got != tt.want {
				t.Fatalf("resolveVersion(%q, %q) = %q, want %q", tt.linked, tt.module, got, tt.want)
			}
		})
	}
}
