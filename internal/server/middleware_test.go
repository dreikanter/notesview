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
