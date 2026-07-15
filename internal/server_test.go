package internal

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

func TestHandleClientMultipleMessages(t *testing.T) {
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		handleClientWithFetcher(server, func(r *model.Result) error {
			r.Found = true
			r.Keyword = "reply:" + r.Query
			return nil
		})
		close(done)
	}()
	t.Cleanup(func() { client.Close() })

	reader := model.NewProtocolReader(client)
	for _, query := range []string{"first", "第二条"} {
		request := model.TCPQuery{Action: "query", B: &model.BaseResult{Query: query}}
		if err := model.WriteProtocolMessage(client, request); err != nil {
			t.Fatalf("WriteProtocolMessage(%q) error = %v", query, err)
		}
		var response model.DaemonResponse
		if err := reader.Read(&response); err != nil {
			t.Fatalf("Read(response %q) error = %v", query, err)
		}
		if response.Error != "" || response.R == nil || response.R.Keyword != "reply:"+query {
			t.Fatalf("response for %q = %+v", query, response)
		}
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleClientWithFetcher() did not stop after connection close")
	}
}

func TestHandleClientInvalidRequestAndFetcherError(t *testing.T) {
	client, server := net.Pipe()
	go handleClientWithFetcher(server, func(*model.Result) error {
		return errors.New("fetch failed")
	})
	t.Cleanup(func() { client.Close() })

	reader := model.NewProtocolReader(client)
	if _, err := client.Write([]byte("not-json\n")); err != nil {
		t.Fatalf("Write(invalid JSON) error = %v", err)
	}
	var invalid model.DaemonResponse
	if err := reader.Read(&invalid); err != nil {
		t.Fatalf("Read(invalid response) error = %v", err)
	}
	if !strings.Contains(invalid.Error, "无效daemon请求") {
		t.Fatalf("invalid response = %+v", invalid)
	}

	request := model.TCPQuery{Action: "query", B: &model.BaseResult{Query: "valid"}}
	if err := model.WriteProtocolMessage(client, request); err != nil {
		t.Fatalf("WriteProtocolMessage() error = %v", err)
	}
	var failed model.DaemonResponse
	if err := reader.Read(&failed); err != nil {
		t.Fatalf("Read(fetch error response) error = %v", err)
	}
	if !strings.Contains(failed.Error, "fetch failed") {
		t.Fatalf("fetch error response = %+v", failed)
	}
}

func TestHandleClientEarlyClose(t *testing.T) {
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		handleClientWithFetcher(server, func(*model.Result) error {
			t.Error("fetch called after client closed")
			return nil
		})
		close(done)
	}()
	client.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleClientWithFetcher() did not stop after early close")
	}
}
