package model

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const MaxProtocolMessageSize = 1 << 20

var (
	ErrIncompleteProtocolMessage = errors.New("incomplete protocol message")
	ErrProtocolMessageTooLarge   = errors.New("protocol message too large")
	ErrEmptyProtocolMessage      = errors.New("empty protocol message")
	ErrInvalidProtocolMessage    = errors.New("invalid protocol message")
)

type ProtocolReader struct {
	scanner *bufio.Scanner
}

func NewProtocolReader(r io.Reader) *ProtocolReader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), MaxProtocolMessageSize+1)
	scanner.Split(splitProtocolLine)
	return &ProtocolReader{scanner: scanner}
}

func splitProtocolLine(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		if i > MaxProtocolMessageSize {
			return 0, nil, ErrProtocolMessageTooLarge
		}
		return i + 1, bytes.TrimSuffix(data[:i], []byte{'\r'}), nil
	}
	if len(data) > MaxProtocolMessageSize {
		return 0, nil, ErrProtocolMessageTooLarge
	}
	if atEOF && len(data) > 0 {
		return 0, nil, ErrIncompleteProtocolMessage
	}
	return 0, nil, nil
}

func (r *ProtocolReader) Read(v any) error {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return err
		}
		return io.EOF
	}
	if len(bytes.TrimSpace(r.scanner.Bytes())) == 0 {
		return ErrEmptyProtocolMessage
	}
	if err := json.Unmarshal(r.scanner.Bytes(), v); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProtocolMessage, err)
	}
	return nil
}

func WriteProtocolMessage(w io.Writer, v any) error {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("encode protocol message: %w", err)
	}
	return nil
}
