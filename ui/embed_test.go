package ui

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDist(t *testing.T) {
	dist, err := Dist()
	if err != nil {
		t.Fatalf("Dist() failed: %v", err)
	}
	f, err := dist.Open("index.html")
	if err != nil {
		t.Skipf("UI not built (run `npm run build` in ui/ before `go build`): %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("io.ReadAll failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("index.html is empty")
	}
	html := string(data)
	if !strings.Contains(html, `id="root"`) {
		t.Fatalf("embedded index.html missing root mount point")
	}
}

// TestDistFourPlatformSingleSpaEmbedding verifies Gate G-15 (UI-03, UI-16):
// one embedded `karyora/ui/dist` SPA serves Web, Linux, macOS, and Windows over HTTP
// with code-split lazy chunks for SettingsOverlayHub and all 12 non-English locales.
func TestDistFourPlatformSingleSpaEmbedding(t *testing.T) {
	dist, err := Dist()
	if err != nil {
		t.Fatalf("Dist() failed: %v", err)
	}
	if _, err := dist.Open("index.html"); err != nil {
		t.Skipf("UI not built (run `npm run build` in ui/ before `go build`): %v", err)
	}

	entries, err := fs.ReadDir(dist, "assets")
	if err != nil {
		t.Fatalf("embedded assets/ directory missing: %v", err)
	}

	var names []string
	var hasEntryJS, hasEntryCSS, hasSettingsChunk bool
	for _, entry := range entries {
		name := entry.Name()
		names = append(names, name)
		if strings.HasPrefix(name, "index.") && strings.HasSuffix(name, ".js") {
			hasEntryJS = true
		}
		if strings.HasPrefix(name, "index.") && strings.HasSuffix(name, ".css") {
			hasEntryCSS = true
		}
		if strings.HasPrefix(name, "SettingsOverlayHub.") && strings.HasSuffix(name, ".js") {
			hasSettingsChunk = true
		}
	}

	if !hasEntryJS || !hasEntryCSS || !hasSettingsChunk {
		t.Fatalf("expected index.js, index.css, and lazy SettingsOverlayHub chunk in assets/, got: %v", names)
	}

	// Verify all 12 lazy non-English locale chunks are embedded in the single Go binary
	lazyLocales := []string{"bn", "zh-CN", "es", "pt-BR", "ja", "ru", "ko", "de", "fr", "id", "tr", "vi"}
	for _, loc := range lazyLocales {
		found := false
		prefix := loc + "."
		for _, name := range names {
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".js") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected lazy embedded locale chunk %q*.js in assets/, got: %v", prefix, names)
		}
	}

	// Verify Go HTTP file server serves the embedded SPA cleanly
	server := httptest.NewServer(http.FileServer(http.FS(dist)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}
}
