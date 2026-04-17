package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestLogger returns a JSON logger writing into the caller's buffer at
// debug level so tests can assert on any field the middleware emits.
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLogRequestsLogsRequestAndResponse(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mw := logRequests(logger)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest("GET", "/view/foo.md?q=1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	out := buf.String()
	if !strings.Contains(out, `"msg":"http request"`) {
		t.Errorf("missing request log: %s", out)
	}
	if !strings.Contains(out, `"msg":"http response"`) {
		t.Errorf("missing response log: %s", out)
	}
	if !strings.Contains(out, `"method":"GET"`) {
		t.Errorf("missing method field: %s", out)
	}
	if !strings.Contains(out, `"path":"/view/foo.md"`) {
		t.Errorf("missing path field: %s", out)
	}
	if !strings.Contains(out, `"query":"q=1"`) {
		t.Errorf("missing query field: %s", out)
	}
	if !strings.Contains(out, `"status":200`) {
		t.Errorf("missing status field: %s", out)
	}
	if !strings.Contains(out, `"bytes":5`) {
		t.Errorf("missing bytes field: %s", out)
	}
}

func TestLogRequestsElevatesServerErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mw := logRequests(logger)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))

	req := httptest.NewRequest("GET", "/view/bad", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// The response log line should be at ERROR level for a 500.
	out := buf.String()
	if !strings.Contains(out, `"level":"ERROR"`) || !strings.Contains(out, `"msg":"http response"`) {
		t.Errorf("expected ERROR-level response log, got: %s", out)
	}
	if !strings.Contains(out, `"status":500`) {
		t.Errorf("missing status field: %s", out)
	}
}

func TestLogRequestsElevatesClientErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mw := logRequests(logger)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	req := httptest.NewRequest("GET", "/view/missing.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	out := buf.String()
	// Response line should be WARN for 4xx.
	if !strings.Contains(out, `"level":"WARN"`) {
		t.Errorf("expected WARN-level response log, got: %s", out)
	}
	if !strings.Contains(out, `"status":404`) {
		t.Errorf("missing 404 status: %s", out)
	}
}

func TestLogRequestsDemotesStaticAssets(t *testing.T) {
	var buf bytes.Buffer
	// Info level — debug records should NOT appear at this level.
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	mw := logRequests(logger)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/static/app.js", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if buf.Len() != 0 {
		t.Errorf("expected static asset traffic to be silenced at info level, got: %s", buf.String())
	}
}

// TestSetHXCacheHeadersTopLevel verifies a plain top-level GET gets
// Vary: HX-Request, HX-Target so a URL-keyed cache won't later serve
// this full-page response as a partial (or vice versa). No-store
// should NOT be set on top-level responses — they're genuinely
// cacheable.
func TestSetHXCacheHeadersTopLevel(t *testing.T) {
	handler := setHXCacheHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/view/foo.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Vary"); got != "HX-Request, HX-Target" {
		t.Errorf("Vary = %q, want %q", got, "HX-Request, HX-Target")
	}
	if got := w.Header().Get("Cache-Control"); got != "" {
		t.Errorf("Cache-Control on top-level = %q, want empty", got)
	}
}

// TestSetHXCacheHeadersPartial verifies an HTMX partial response
// carries both Vary and Cache-Control: no-store, so it stays out of
// the HTTP cache AND out of Firefox's session store (which replays
// bodies on tab restore without honoring Vary).
func TestSetHXCacheHeadersPartial(t *testing.T) {
	handler := setHXCacheHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/view/foo.md", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "note-pane")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Vary"); got != "HX-Request, HX-Target" {
		t.Errorf("Vary = %q, want %q", got, "HX-Request, HX-Target")
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestLogResponseWriterFlushPassesThrough(t *testing.T) {
	// The wrapped writer must still expose http.Flusher so SSE keeps
	// working. httptest.NewRecorder implements Flusher, so we can assert
	// that casting our wrapper succeeds.
	rec := httptest.NewRecorder()
	lw := &logResponseWriter{ResponseWriter: rec}
	if _, ok := interface{}(lw).(http.Flusher); !ok {
		t.Fatalf("logResponseWriter must implement http.Flusher")
	}
	// Calling Flush on the wrapper should not panic.
	lw.Flush()
}

// TestLogRequestsWithServerRoutes exercises the middleware via the full
// Server.Routes() wiring, to verify requests flowing through the normal
// handler stack produce structured log records.
func TestLogRequestsWithServerRoutes(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	srv, _ := setupTestServer(t)
	srv.logger = logger
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/README.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	out := buf.String()
	if !strings.Contains(out, `"path":"/view/README.md"`) {
		t.Errorf("expected request path in log: %s", out)
	}
	if !strings.Contains(out, `"msg":"http response"`) {
		t.Errorf("expected response log: %s", out)
	}
}
