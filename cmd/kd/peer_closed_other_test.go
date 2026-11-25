//go:build !windows

package main

import (
	"errors"
	"syscall"
	"testing"
)

func isPeerReset(err error) bool {
	return errors.Is(err, syscall.ECONNRESET)
}

func isPeerGoneOnWrite(err error) bool {
	return isPeerReset(err) || errors.Is(err, syscall.EPIPE)
}

func TestPeerClosed(t *testing.T) {
	testPeerClosed(t, []peerClosedCase{
		{name: "reset", err: syscall.ECONNRESET, wantReset: true, wantWrite: true, wantEmptyRead: true},
		{name: "broken pipe", err: syscall.EPIPE, wantWrite: true},
	})
}
