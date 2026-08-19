package explorerweb

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	explorerassets "github.com/nstranquist/nicos-catalog/explorer"
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
	entries, err := fs.ReadDir(explorerassets.FS(), "assets")
	if err != nil {
		t.Fatal(err)
	}
	assetName := ""
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".js") {
			assetName = entry.Name()
			break
		}
	}
	if assetName == "" {
		t.Fatal("embedded Explorer has no JavaScript asset")
	}
	asset := httptest.NewRequest(http.MethodGet, "/assets/"+assetName, nil)
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
