//go:build windows

package main

import (
	"errors"
	"syscall"
	"testing"
)

func isPeerReset(err error) bool {
	return errors.Is(err, syscall.WSAECONNRESET) || errors.Is(err, syscall.WSAECONNABORTED)
}

func isPeerGoneOnWrite(err error) bool {
	return isPeerReset(err)
}

func TestPeerClosed(t *testing.T) {
	testPeerClosed(t, []peerClosedCase{
		{name: "reset", err: syscall.WSAECONNRESET, wantReset: true, wantWrite: true, wantEmptyRead: true},
		{name: "aborted", err: syscall.WSAECONNABORTED, wantReset: true, wantWrite: true, wantEmptyRead: true},
	})
}
