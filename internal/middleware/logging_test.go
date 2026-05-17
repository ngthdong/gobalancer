package middleware_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ngthdong/gobalancer/internal/constant"
	"github.com/ngthdong/gobalancer/internal/middleware"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	return slog.New(handler)
}

func TestLoggingMiddlewareStatus(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	handler := middleware.Logging(inner, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)

	handler.ServeHTTP(rec, req)

	logOutput := buf.String()

	if !strings.Contains(logOutput, "status=418") {
		t.Fatalf("expected status=418 in log, got: %s", logOutput)
	}
}

func TestLoggingMiddlewareDefaultStatusOK(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello")
	})

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	handler := middleware.Logging(inner, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/hello", nil)

	handler.ServeHTTP(rec, req)

	logOutput := buf.String()

	if !strings.Contains(logOutput, "status=200") {
		t.Fatalf("expected status=200 in log, got: %s", logOutput)
	}
}

func TestLoggingMiddlewareBytesWritten(t *testing.T) {
	body := "hello world"

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	handler := middleware.Logging(inner, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/bytes", nil)

	handler.ServeHTTP(rec, req)

	logOutput := buf.String()

	if !strings.Contains(logOutput, "bytes=11") {
		t.Fatalf("expected bytes=11 in log, got: %s", logOutput)
	}
}

func TestLoggingMiddlewareBackendField(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	handler := middleware.Logging(inner, logger)

	rec := httptest.NewRecorder()

	req := httptest.NewRequest("GET", "/backend", nil)

	ctx := context.WithValue(
		req.Context(),
		constant.ContextKeyBackend,
		"backend-1:8080",
	)

	req = req.WithContext(ctx)

	handler.ServeHTTP(rec, req)

	logOutput := buf.String()

	if !strings.Contains(logOutput, "backend=backend-1:8080") {
		t.Fatalf("expected backend field in log, got: %s", logOutput)
	}
}

func TestLoggingMiddlewarePreservesResponse(t *testing.T) {
	expectedBody := "proxy response"

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, expectedBody)
	})

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	handler := middleware.Logging(inner, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	handler.ServeHTTP(rec, req)

	if rec.Body.String() != expectedBody {
		t.Fatalf(
			"expected body %q, got %q",
			expectedBody,
			rec.Body.String(),
		)
	}
}
