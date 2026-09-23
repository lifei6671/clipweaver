package server

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticPageAndSPAFallback(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<h1>ClipWeaver</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{WebDistDir: dist, MaxUploadMB: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/preview"} {
		response, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || !strings.Contains(string(body), "ClipWeaver") {
			t.Fatalf("%s: status=%d body=%q err=%v", path, response.StatusCode, body, err)
		}
		if response.Header.Get("X-Request-Id") == "" {
			t.Fatalf("%s: missing request id", path)
		}
	}
	response, err := app.Test(httptest.NewRequest("GET", "/api/missing", nil))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatalf("unknown API returned %d", response.StatusCode)
	}
}
