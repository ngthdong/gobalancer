package middleware

import (
	"log"
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

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := newResponseRecorder(w)

		next.ServeHTTP(rec, r)

		backend, _ := r.Context().Value(constant.ContextKeyBackend).(string)

		log.Printf(
			"method=%s path=%s backend=%s status=%d latency=%s bytes=%d",
			r.Method,
			r.URL.Path,
			backend,
			rec.statusCode,
			time.Since(start).Round(time.Microsecond),
			rec.written,
		)
	})
}
