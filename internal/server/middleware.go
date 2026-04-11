package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// logResponseWriter wraps http.ResponseWriter to capture the status code and
// number of bytes written so the logging middleware can report them after
// the handler returns. It implements http.Flusher so SSE handlers that cast
// the writer to http.Flusher keep working.
type logResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *logResponseWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *logResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Flush exposes the underlying writer's Flush method when it implements
// http.Flusher. The SSE handler relies on this interface check, so the
// wrapper has to forward it or live-reload stops working.
func (w *logResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// logRequests returns middleware that emits a structured record for every
// HTTP request and its corresponding response. Requests are logged before
// the handler runs so long-running handlers (e.g. SSE) show up in logs as
// soon as they connect; responses are logged on return with status, byte
// count, and duration. Static asset traffic is demoted to debug so a
// normal page load produces one info record per meaningful request.
func logRequests(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			level := slog.LevelInfo
			if isNoisyPath(r.URL.Path) {
				level = slog.LevelDebug
			}
			start := time.Now()

			reqAttrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote", r.RemoteAddr),
			}
			if r.URL.RawQuery != "" {
				reqAttrs = append(reqAttrs, slog.String("query", r.URL.RawQuery))
			}
			logger.LogAttrs(r.Context(), level, "http request", reqAttrs...)

			lw := &logResponseWriter{ResponseWriter: w}
			next.ServeHTTP(lw, r)

			status := lw.status
			if status == 0 {
				status = http.StatusOK
			}
			respLevel := level
			switch {
			case status >= 500:
				respLevel = slog.LevelError
			case status >= 400 && respLevel < slog.LevelWarn:
				respLevel = slog.LevelWarn
			}
			logger.LogAttrs(r.Context(), respLevel, "http response",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int("bytes", lw.bytes),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// isNoisyPath returns true for request paths that would otherwise spam the
// log at info level on every page load — currently just static assets.
func isNoisyPath(path string) bool {
	return strings.HasPrefix(path, "/static/")
}
