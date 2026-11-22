// Package e2e runs a freshly built kd from real shells (bash, zsh, fish,
// pwsh and cmd) and checks what each shell hands to kd and gets back from it.
//
// Unlike the unit tests these use the real daemon on port 19707 and the real
// network, so they only build with the e2e tag:
//
//	go test -tags e2e -count=1 ./e2e
//
// -count=1 matters: go test otherwise may replay a cached result without
// running anything. Progress goes to the terminal while the suite runs, or
// to stdout with -v.
//
// See shells_test.go for the isolation rules the suite follows.
package e2e
