package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

var testEncs = []encoding{{coding: "br", ext: ".br"}, {coding: "gzip", ext: ".gz"}}

// writeFiles creates each name → content under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// newTestHandler serves a dist folder holding an index and a JS asset, both with compressed copies,
// and a CSS asset with none.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	dist := t.TempDir()
	writeFiles(t, dist, map[string]string{
		"index.html":         "<html>",
		"index.html.gz":      "index-gz",
		"assets/app.js":      "plain-js",
		"assets/app.js.br":   "br-js",
		"assets/app.js.gz":   "gz-js",
		"assets/style.css":   "plain-css",
		"assets/blob.zzz":    "plain-blob",
		"assets/blob.zzz.gz": "gz-blob",
	})
	return newHandler(dist, filepath.Join(dist, "index.html"), "no-cache", "immutable", testEncs)
}

// get requests path with the given Accept-Encoding header ("" sends none).
func get(t *testing.T, h http.Handler, path, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptEncoding != "" {
		r.Header.Set("Accept-Encoding", acceptEncoding)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestServesPrecompressed(t *testing.T) {
	h := newTestHandler(t)
	tests := []struct {
		name, path, acceptEncoding               string
		wantBody, wantEncoding, wantType, wantCC string
		wantVary                                 bool
	}{
		{"br preferred", "/assets/app.js", "br, gzip", "br-js", "br", "text/javascript; charset=utf-8", "immutable", true},
		{"gzip only", "/assets/app.js", "gzip", "gz-js", "gzip", "text/javascript; charset=utf-8", "immutable", true},
		{"br refused", "/assets/app.js", "br;q=0, gzip", "gz-js", "gzip", "text/javascript; charset=utf-8", "immutable", true},
		{"star", "/assets/app.js", "*", "br-js", "br", "text/javascript; charset=utf-8", "immutable", true},
		{"no header", "/assets/app.js", "", "plain-js", "", "text/javascript; charset=utf-8", "immutable", true},
		{"no copy", "/assets/style.css", "br, gzip", "plain-css", "", "text/css; charset=utf-8", "immutable", false},
		{"unknown type", "/assets/blob.zzz", "gzip", "plain-blob", "", "", "immutable", true},
		{"spa route", "/some/route", "gzip", "index-gz", "gzip", "text/html; charset=utf-8", "no-cache", true},
		{"root", "/", "gzip", "index-gz", "gzip", "text/html; charset=utf-8", "no-cache", true},
		{"root plain", "/", "", "<html>", "", "text/html; charset=utf-8", "no-cache", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := get(t, h, tt.path, tt.acceptEncoding)
			if w.Code != http.StatusOK || w.Body.String() != tt.wantBody {
				t.Fatalf("got %d %q, want 200 %q", w.Code, w.Body.String(), tt.wantBody)
			}
			if got := w.Header().Get("Content-Encoding"); got != tt.wantEncoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tt.wantEncoding)
			}
			if got := w.Header().Get("Content-Type"); tt.wantType != "" && got != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantType)
			}
			if got := w.Header().Get("Cache-Control"); got != tt.wantCC {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCC)
			}
			if got := w.Header().Get("Vary") == "Accept-Encoding"; got != tt.wantVary {
				t.Errorf("Vary = %q, want Accept-Encoding: %v", w.Header().Get("Vary"), tt.wantVary)
			}
		})
	}
}

func TestMissingAssetIs404(t *testing.T) {
	w := get(t, newTestHandler(t), "/assets/missing.js", "br, gzip")
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", w.Code)
	}
	for _, header := range []string{"Content-Encoding", "Cache-Control", "Vary"} {
		if got := w.Header().Get(header); got != "" {
			t.Errorf("%s = %q, want none", header, got)
		}
	}
}

func TestAccepts(t *testing.T) {
	tests := []struct {
		header, coding string
		want           bool
	}{
		{"", "gzip", false},
		{"gzip, deflate, br", "br", true},
		{"GZIP", "gzip", true},
		{"gzip;q=0", "gzip", false},
		{"gzip; q=0.5", "gzip", true},
		{"*;q=0", "br", false},
		{"*, br;q=0", "br", false},
		{"identity", "gzip", false},
	}
	for _, tt := range tests {
		if got := accepts(tt.header, tt.coding); got != tt.want {
			t.Errorf("accepts(%q, %q) = %v, want %v", tt.header, tt.coding, got, tt.want)
		}
	}
}
