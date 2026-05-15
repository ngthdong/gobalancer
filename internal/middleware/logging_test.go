package middleware_test

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ngthdong/gobalancer/internal/constant"
	"github.com/ngthdong/gobalancer/internal/middleware"
)

func TestLoggingMiddlewareStatus(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)

	handler := middleware.Logging(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)

	handler.ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), "status=418") {
		t.Errorf("log line missing status=418, got: %s", buf.String())
	}
}

func TestLoggingMiddlewareDefaultStatusOK(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello")
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)

	handler := middleware.Logging(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/hello", nil)

	handler.ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), "status=200") {
		t.Errorf("log line missing status=200, got: %s", buf.String())
	}
}

func TestLoggingMiddlewareBytesWritten(t *testing.T) {
	body := "hello world"

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)

	handler := middleware.Logging(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/bytes", nil)

	handler.ServeHTTP(rec, req)

	expected := "bytes=11"

	if !strings.Contains(buf.String(), expected) {
		t.Errorf("log line missing %s, got: %s",
			expected,
			buf.String(),
		)
	}
}

func TestLoggingMiddlewareBackendField(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)

	handler := middleware.Logging(inner)

	rec := httptest.NewRecorder()

	req := httptest.NewRequest("GET", "/backend", nil)

	ctx := context.WithValue(
		req.Context(),
		constant.ContextKeyBackend,
		"backend-1:8080",
	)

	req = req.WithContext(ctx)

	handler.ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), "backend=backend-1:8080") {
		t.Errorf("backend field missing in log: %s", buf.String())
	}
}

func TestLoggingMiddlewarePreservesResponse(t *testing.T) {
	expectedBody := "proxy response"

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, expectedBody)
	})

	handler := middleware.Logging(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	handler.ServeHTTP(rec, req)

	if rec.Body.String() != expectedBody {
		t.Errorf("want body %q, got %q",
			expectedBody,
			rec.Body.String(),
		)
	}
}