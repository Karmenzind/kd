package logger

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestComponentField(t *testing.T) {
	var output bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&output), zap.InfoLevel)
	logger := zap.New(core, componentField("server"))
	logger.Info("started")

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("Unmarshal(log output) error = %v; output = %q", err, output.String())
	}
	if got := entry["component"]; got != "server" {
		t.Fatalf("component field = %v, want server", got)
	}
}
