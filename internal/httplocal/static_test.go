package httplocal

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStaticFSServing(t *testing.T) {
	fixtureFS := fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html><html><body>Karyora Test</body></html>")},
		"assets/style.css": {Data: []byte("body { color: blue; }")},
		"favicon.ico":      {Data: []byte("icon")},
	}

	env := newTestEnv(t)
	deps := env.srv.deps
	deps.StaticFS = fixtureFS
	srv, err := New(deps)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 1. GET / serves index.html
	req := httptest.NewRequest(http.MethodGet, "http://localhost:27210/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<!doctype html><html><body>Karyora Test</body></html>" {
		t.Fatalf("GET / body mismatch: %q", rec.Body.String())
	}

	// 2. GET /chat (SPA client-side route) falls back to index.html
	req = httptest.NewRequest(http.MethodGet, "http://localhost:27210/chat", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /chat expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<!doctype html><html><body>Karyora Test</body></html>" {
		t.Fatalf("GET /chat body mismatch: %q", rec.Body.String())
	}

	// 3. GET /assets/style.css serves static asset
	req = httptest.NewRequest(http.MethodGet, "http://localhost:27210/assets/style.css", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /assets/style.css expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "body { color: blue; }" {
		t.Fatalf("GET /assets/style.css body mismatch: %q", rec.Body.String())
	}

	// 4. Missing asset in /assets/ returns 404 (does NOT fall back to index.html)
	req = httptest.NewRequest(http.MethodGet, "http://localhost:27210/assets/missing.js", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /assets/missing.js expected 404, got %d", rec.Code)
	}

	// 5. GET /v1/nonexistent returns JSON envelope (API boundary preserved)
	req = httptest.NewRequest(http.MethodGet, "http://localhost:27210/v1/nonexistent", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /v1/nonexistent expected 404, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("GET /v1/nonexistent expected JSON Content-Type, got %q", rec.Header().Get("Content-Type"))
	}
}
