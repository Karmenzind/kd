package cache

import "testing"

func TestCheckNotFound(t *testing.T) {
	line, _ := CheckNotFound("asdf")
	if line == 0 {
		t.Failed()
	}
}
