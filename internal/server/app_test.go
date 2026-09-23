package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type bufferedConn struct {
	input  *bytes.Reader
	output bytes.Buffer
}

func (c *bufferedConn) Read(p []byte) (int, error)       { return c.input.Read(p) }
func (c *bufferedConn) Write(p []byte) (int, error)      { return c.output.Write(p) }
func (c *bufferedConn) Close() error                     { return nil }
func (c *bufferedConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *bufferedConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *bufferedConn) SetDeadline(time.Time) error      { return nil }
func (c *bufferedConn) SetReadDeadline(time.Time) error  { return nil }
func (c *bufferedConn) SetWriteDeadline(time.Time) error { return nil }

func TestBodyLimitUsesPublicError(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app, err := New(Config{WebDistDir: dist, DataDir: t.TempDir(), MaxUploadMB: 1}, logger)
	if err != nil {
		t.Fatal(err)
	}
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	part, err := writer.CreateFormFile("file", "oversize.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte{'x'}, 1024*1024)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if payload.Len() <= 1024*1024 {
		t.Fatalf("multipart body length %d does not exceed 1 MiB limit", payload.Len())
	}
	req := httptest.NewRequest(http.MethodPost, "/api/assets/audio", &payload)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	warmup, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/missing", nil))
	if err != nil {
		t.Fatal(err)
	}
	warmup.Body.Close()
	var wire bytes.Buffer
	if err := req.Write(&wire); err != nil {
		t.Fatal(err)
	}
	conn := &bufferedConn{input: bytes.NewReader(wire.Bytes())}
	// app.Test returns the parser error before reading the 413 already written by fasthttp.
	serveErr := app.Server().ServeConn(conn)
	response, err := http.ReadResponse(bufio.NewReader(&conn.output), req)
	if err != nil {
		t.Fatalf("read response after ServeConn error %v: %v", serveErr, err)
	}
	defer response.Body.Close()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 413 || body.Error.Code != "UPLOAD_TOO_LARGE" {
		t.Fatalf("status=%d error=%+v", response.StatusCode, body.Error)
	}
	page, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/missing", nil))
	if err != nil {
		t.Fatal(err)
	}
	page.Body.Close()
	if page.StatusCode != http.StatusNotFound {
		t.Fatalf("app stopped responding after oversized upload: status=%d", page.StatusCode)
	}
	if _, err := New(Config{WebDistDir: dist, DataDir: t.TempDir(), MaxUploadMB: int(^uint(0) >> 1)}, logger); err == nil {
		t.Fatal("overflowing upload limit accepted")
	}
}

func TestStaticPageAndSPAFallback(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<h1>ClipWeaver</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{WebDistDir: dist, DataDir: t.TempDir(), MaxUploadMB: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestMixRoutesRegistered(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{WebDistDir: dist, DataDir: t.TempDir(), MaxUploadMB: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, body, code string
		status                   int
	}{
		{"POST", "/api/mixes", `{}`, "NO_VIDEO_SELECTED", 400},
		{"GET", "/api/mixes/bad/file", "", "INVALID_ID", 400},
		{"GET", "/api/mixes/bad/download", "", "INVALID_ID", 400},
	} {
		response, err := app.Test(httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)), -1)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&got)
		response.Body.Close()
		if decodeErr != nil || response.StatusCode != tc.status || got.Error.Code != tc.code {
			t.Fatalf("%s %s: status=%d error=%+v decode=%v", tc.method, tc.path, response.StatusCode, got.Error, decodeErr)
		}
	}
}
