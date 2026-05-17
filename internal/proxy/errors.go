package proxy

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"

	"github.com/ngthdong/gobalancer/internal/constant"
)

func errorHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		backend, _ := r.Context().Value(constant.ContextKeyBackend).(string)
		code, kind := classifyError(err)
		fields := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"backend", backend,
			"status", code,
			"error", err.Error(),
			"kind", kind,
		}

		if code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout {
			logger.Warn("proxy request failed", fields...)
		} else {
			logger.Error("proxy request failed", fields...)
		}

		if isHeaderWritten(w) {
			return
		}

		if code == http.StatusServiceUnavailable {
			w.Header().Set("Retry-After", "1")
		}

		http.Error(w, statusMessage(code), code)
	}
}

func classifyError(err error) (statusCode int, kind string) {
	if err == nil {
		return http.StatusOK, "none"
	}

	if errors.Is(err, context.Canceled) {
		return 499, "client_closed" 
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, "timeout"
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return http.StatusGatewayTimeout, "upstream_timeout"
		}
		return http.StatusBadGateway, "upstream_error"
	}

	msg := err.Error()
	if contains(msg, "no available backends") {
		return http.StatusServiceUnavailable, "no_backends"
	}

	return http.StatusBadGateway, "unknown"
}

func isHeaderWritten(w http.ResponseWriter) bool {
	type wroteHeader interface {
		WroteHeader() bool
	}
	if wh, ok := w.(wroteHeader); ok {
		return wh.WroteHeader()
	}
	return false
}

func statusMessage(code int) string {
	switch code {
	case http.StatusBadGateway:
		return "Bad Gateway"
	case http.StatusServiceUnavailable:
		return "Service Unavailable"
	case http.StatusGatewayTimeout:
		return "Gateway Timeout"
	case 499:
		return "Client Closed Request"
	default:
		return http.StatusText(code)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(substr) == 0 ||
			(len(s) > 0 && indexStr(s, substr) >= 0))
}

func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
