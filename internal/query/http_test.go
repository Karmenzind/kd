package query

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

func TestRequestYoudaoWith(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.EscapedPath()
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.Header.Get("Accept-Language") == "" || r.Header.Get("User-Agent") == "" {
			t.Errorf("required headers missing: %#v", r.Header)
		}
		if r.Header.Get("Upgrade-Insecure-Requests") != "1" {
			t.Errorf("Upgrade-Insecure-Requests = %q", r.Header.Get("Upgrade-Insecure-Requests"))
		}
		_, _ = w.Write([]byte("<html>translated</html>"))
	}))
	t.Cleanup(server.Close)

	r := &model.Result{BaseResult: &model.BaseResult{Query: "hello world 中文", IsLongText: true}}
	body, err := requestYoudaoWith(server.Client(), server.URL, r)
	if err != nil {
		t.Fatalf("requestYoudaoWith() error = %v", err)
	}
	if string(body) != "<html>translated</html>" {
		t.Fatalf("body = %q", body)
	}
	if !strings.Contains(receivedPath, "hello%20world%20%E4%B8%AD%E6%96%87") {
		t.Fatalf("escaped path = %q", receivedPath)
	}
}

// issue #82：查询词里的`%`、`#`、`&`等字符以前是原样拼进URL的，
// `50%`后面跟着空格转义出的`%20`会凑成非法转义`%%2`，请求直接构造失败。
func TestRequestYoudaoWithEscapesQuery(t *testing.T) {
	for _, tt := range []struct {
		name  string
		query string
		want  string
	}{
		{name: "percent", query: "About 50% of the students agreed.", want: "/About%2050%25%20of%20the%20students%20agreed./"},
		{name: "hash", query: "c# language", want: "/c%23%20language/"},
		{name: "slash", query: "and/or", want: "/and%2For/"},
		{name: "apostrophe", query: "aren't", want: "/aren%27t/"},
		{name: "ampersand", query: "AT&T", want: "/AT&T/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var receivedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.EscapedPath()
				_, _ = w.Write([]byte("<html></html>"))
			}))
			t.Cleanup(server.Close)

			r := &model.Result{BaseResult: &model.BaseResult{Query: tt.query}}
			if _, err := requestYoudaoWith(server.Client(), server.URL, r); err != nil {
				t.Fatalf("requestYoudaoWith(%q) error = %v", tt.query, err)
			}
			if receivedPath != tt.want {
				t.Fatalf("escaped path = %q, want %q", receivedPath, tt.want)
			}
		})
	}
}

func TestRequestYoudaoWithResponseErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "non-2xx", status: http.StatusBadGateway, body: "upstream failed", want: "502 Bad Gateway"},
		{name: "empty response", status: http.StatusOK, body: "", want: "empty response"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)

			_, err := requestYoudaoWith(server.Client(), server.URL, &model.Result{BaseResult: &model.BaseResult{Query: "test"}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("requestYoudaoWith() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRequestYoudaoWithTimeoutAndConnectionError(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		t.Cleanup(server.Close)
		client := server.Client()
		client.Timeout = 20 * time.Millisecond

		_, err := requestYoudaoWith(client, server.URL, &model.Result{BaseResult: &model.BaseResult{Query: "test"}})
		if err == nil {
			t.Fatal("requestYoudaoWith(timeout) returned nil error")
		}
	})

	t.Run("connection error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := server.URL
		server.Close()

		_, err := requestYoudaoWith(&http.Client{Timeout: time.Second}, url, &model.Result{BaseResult: &model.BaseResult{Query: "test"}})
		if err == nil {
			t.Fatal("requestYoudaoWith(closed server) returned nil error")
		}
	})
}
