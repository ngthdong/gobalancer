package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ngthdong/gobalancer/internal/constant"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	n, err := rr.ResponseWriter.Write(b)
	rr.written += int64(n)
	return n, err
}

func Logging(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := newResponseRecorder(w)

		next.ServeHTTP(rec, r)

		backend, _ := r.Context().Value(constant.ContextKeyBackend).(string)

		fields := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"backend", backend,
			"status", rec.statusCode,
			"latency", time.Since(start).Round(time.Microsecond).String(),
			"bytes", rec.written,
		}

		switch {
		case rec.statusCode >= 500:
			logger.Error("request completed", fields...)
		case rec.statusCode >= 400:
			logger.Warn("request completed", fields...)
		default:
			logger.Info("request completed", fields...)
		}
	})
}
