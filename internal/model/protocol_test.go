package model

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type chunkReader struct {
	r    io.Reader
	size int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(p) > r.size {
		p = p[:r.size]
	}
	return r.r.Read(p)
}

func TestProtocolMultipleMessagesAndPartialReads(t *testing.T) {
	var wire bytes.Buffer
	wants := []TCPQuery{
		{Action: "query", B: &BaseResult{Query: "first"}},
		{Action: "query", B: &BaseResult{Query: "第二条"}},
	}
	for _, message := range wants {
		if err := WriteProtocolMessage(&wire, message); err != nil {
			t.Fatalf("WriteProtocolMessage() error = %v", err)
		}
	}

	reader := NewProtocolReader(&chunkReader{r: &wire, size: 3})
	for i, want := range wants {
		var got TCPQuery
		if err := reader.Read(&got); err != nil {
			t.Fatalf("Read(message %d) error = %v", i, err)
		}
		if got.Action != want.Action || got.B == nil || got.B.Query != want.B.Query {
			t.Fatalf("message %d = %+v, want %+v", i, got, want)
		}
	}
	var extra TCPQuery
	if err := reader.Read(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("Read(after messages) error = %v, want io.EOF", err)
	}
}

func TestProtocolRejectsInvalidAndIncompleteMessages(t *testing.T) {
	t.Run("invalid JSON does not consume next line", func(t *testing.T) {
		reader := NewProtocolReader(strings.NewReader("not-json\n{\"Action\":\"query\",\"B\":{\"Query\":\"ok\"}}\n"))
		var got TCPQuery
		if err := reader.Read(&got); err == nil {
			t.Fatal("Read(invalid JSON) returned nil error")
		}
		if err := reader.Read(&got); err != nil {
			t.Fatalf("Read(valid line after invalid JSON) error = %v", err)
		}
		if got.B == nil || got.B.Query != "ok" {
			t.Fatalf("decoded query = %+v", got)
		}
	})

	t.Run("incomplete", func(t *testing.T) {
		reader := NewProtocolReader(strings.NewReader(`{"Action":"query"}`))
		var got TCPQuery
		if err := reader.Read(&got); !errors.Is(err, ErrIncompleteProtocolMessage) {
			t.Fatalf("Read(incomplete) error = %v", err)
		}
	})

	t.Run("too large", func(t *testing.T) {
		reader := NewProtocolReader(strings.NewReader(strings.Repeat("x", MaxProtocolMessageSize+1) + "\n"))
		var got TCPQuery
		if err := reader.Read(&got); !errors.Is(err, ErrProtocolMessageTooLarge) && !strings.Contains(err.Error(), "token too long") {
			t.Fatalf("Read(oversized) error = %v", err)
		}
	})
}
