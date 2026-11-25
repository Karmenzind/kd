package ip

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestIPInfoIsCN(t *testing.T) {
	if !(&IPInfo{Country: "cn"}).IsCN() {
		t.Fatal("IsCN(cn) = false, want true")
	}
	if (&IPInfo{Country: "US"}).IsCN() {
		t.Fatal("IsCN(US) = true, want false")
	}
}

func TestGetIPInfoViaLocalServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/json") && r.URL.Path != "/" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(IPInfo{Country: "US"})
	}))
	t.Cleanup(server.Close)

	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme = "http"
		r.URL.Host = server.Listener.Addr().String()
		return orig.RoundTrip(r)
	})

	info, err := GetIPInfo()
	if err != nil {
		t.Fatalf("GetIPInfo() error = %v", err)
	}
	if info.Country != "US" {
		t.Fatalf("Country = %q, want US", info.Country)
	}
	if info.IsCN() {
		t.Fatal("IsCN() = true, want false for US")
	}
}
