package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsServer struct {
	server *http.Server
	logger *slog.Logger
}

func NewMetricsServer(addr string, reg prometheus.Gatherer, logger *slog.Logger) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	}))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return &MetricsServer{
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
		logger: logger,
	}
}

func (ms *MetricsServer) Start(ctx context.Context) {
	go func() {
		ms.logger.Info("metrics server listening", "addr", ms.server.Addr)
		if err := ms.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ms.logger.Error("metrics server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		ms.server.Shutdown(context.Background())
	}()
}
