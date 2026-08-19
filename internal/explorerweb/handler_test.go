package explorerweb

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerRoutesAndTraversal(t *testing.T) {
	h := Handler()
	for _, path := range []string{"/", "/catalog", "/entity/service.seed-api", "/graph", "/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("%s: status=%d type=%q", path, res.Code, res.Header().Get("Content-Type"))
		}
	}
	for _, path := range []string{"/../go.mod", "/assets/missing.js", "/missing.txt"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusNotFound {
			t.Fatalf("%s returned %d", path, res.Code)
		}
	}
	asset := httptest.NewRequest(http.MethodGet, "/assets/app-00000000.js", nil)
	assetRes := httptest.NewRecorder()
	h.ServeHTTP(assetRes, asset)
	if assetRes.Code != http.StatusOK || assetRes.Header().Get("Cache-Control") == "" {
		t.Fatalf("asset = %d %v", assetRes.Code, assetRes.Header())
	}
	head := httptest.NewRequest(http.MethodHead, "/", nil)
	headRes := httptest.NewRecorder()
	h.ServeHTTP(headRes, head)
	if headRes.Code != http.StatusOK || headRes.Body.Len() != 0 {
		t.Fatalf("HEAD = %d %q", headRes.Code, headRes.Body.String())
	}
	post := httptest.NewRequest(http.MethodPost, "/", nil)
	postRes := httptest.NewRecorder()
	h.ServeHTTP(postRes, post)
	if postRes.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", postRes.Code)
	}
	backslash := httptest.NewRequest(http.MethodGet, "/safe", nil)
	backslash.URL.Path = `/assets\escape.js`
	backslashRes := httptest.NewRecorder()
	h.ServeHTTP(backslashRes, backslash)
	if backslashRes.Code != http.StatusNotFound {
		t.Fatalf("backslash = %d", backslashRes.Code)
	}
}
