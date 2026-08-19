package explorerapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
	"github.com/nstranquist/nicos-catalog/internal/explorerweb"
)

func TestHandlerAPIContractAndSecurity(t *testing.T) {
	s := serviceFixture(t)
	h, err := NewHandler(s, HandlerConfig{ProductVersion: "v0.3.0", AllowedHosts: []string{"127.0.0.1:7788"}, Web: explorerweb.Handler()})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1:7788"+target, nil)
		req.Host = "127.0.0.1:7788"
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	for _, target := range []string{"/api/v1/status", "/api/v1/entities?limit=2", "/api/v1/entities/system.orchard", "/api/v1/search?q=seed", "/api/v1/graph", "/api/v1/health", "/api/v1/schema"} {
		res := request(http.MethodGet, target)
		if res.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", target, res.Code, res.Body.String())
		}
		var envelope explorercontract.Envelope
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil || !envelope.OK || envelope.SchemaVersion != 1 {
			t.Fatalf("%s envelope = %+v err=%v", target, envelope, err)
		}
		if res.Header().Get("Content-Security-Policy") == "" || res.Header().Get("Access-Control-Allow-Origin") != "" || res.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s security headers = %v", target, res.Header())
		}
	}
	if res := request(http.MethodGet, "/api/v1/entities?unknown=private-canary"); res.Code != http.StatusBadRequest || strings.Contains(res.Body.String(), "private-canary") {
		t.Fatalf("unknown query = %d %s", res.Code, res.Body.String())
	}
	if res := request(http.MethodPost, "/api/v1/entities"); res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", res.Code)
	}
	if res := request(http.MethodHead, "/api/v1/status"); res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("HEAD API = %d", res.Code)
	}
	if res := request(http.MethodGet, "/catalog"); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Nicos Catalog Explorer") {
		t.Fatalf("SPA = %d %s", res.Code, res.Body.String())
	}
	if res := request(http.MethodGet, "/api/v1/missing"); res.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", res.Code)
	}
	for _, target := range []string{
		"/api/v1/entities?limit=-1", "/api/v1/entities/service.missing", "/api/v1/entities/BAD%20ID",
		"/api/v1/search", "/api/v1/search?q=seed&limit=99", "/api/v1/graph?mode=full",
		"/api/v1/graph?mode=neighborhood&id=system.orchard&depth=9", "/api/v1/health?severity=fatal",
	} {
		res := request(http.MethodGet, target)
		if res.Code < 400 {
			t.Fatalf("invalid %s returned %d", target, res.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "http://evil.invalid/api/v1/status", nil)
	req.Host = "evil.invalid"
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("bad host = %d", res.Code)
	}
}

func TestHandlerETagAndResponseBound(t *testing.T) {
	s := serviceFixture(t)
	h, _ := NewHandler(s, HandlerConfig{ProductVersion: "v0.3.0", AllowedHosts: []string{"localhost:9000"}, Web: explorerweb.Handler()})
	req := httptest.NewRequest(http.MethodGet, "http://localhost:9000/api/v1/entities?limit=1", nil)
	req.Host = "localhost:9000"
	first := httptest.NewRecorder()
	h.ServeHTTP(first, req)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	req = httptest.NewRequest(http.MethodGet, "http://localhost:9000/api/v1/entities?limit=1", nil)
	req.Host = "localhost:9000"
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	h.ServeHTTP(second, req)
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("conditional = %d %q", second.Code, second.Body.String())
	}
}

func TestLoopbackValidationAndLiveShutdown(t *testing.T) {
	ctx := context.Background()
	for _, address := range []string{"127.0.0.1:0", "[::1]:0", "localhost:0"} {
		if err := ValidateListenAddress(ctx, address); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:8080", "example.com:80", ":8080", "127.0.0.1:70000", "bad"} {
		if err := ValidateListenAddress(ctx, address); err == nil {
			t.Fatalf("unsafe listen accepted: %s", address)
		}
	}
	if _, err := AllowedHostsForListener("bad"); err == nil {
		t.Fatal("bad listener accepted")
	}

	runCtx, cancel := context.WithCancel(context.Background())
	ready := make(chan Ready, 1)
	done := make(chan error, 1)
	liveService := serviceFixture(t)
	go func() {
		done <- RunServer(runCtx, ServerConfig{Listen: "127.0.0.1:0", ProductVersion: "v0.3.0", Service: liveService, Web: explorerweb.Handler(), OnReady: func(value Ready) error { ready <- value; return nil }})
	}()
	var state Ready
	select {
	case state = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not become ready")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(state.URL + "/api/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live status = %d", response.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestNewHandlerRejectsMissingConfiguration(t *testing.T) {
	if _, err := NewHandler(nil, HandlerConfig{}); err == nil {
		t.Fatal("nil service accepted")
	}
}
