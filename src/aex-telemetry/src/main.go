package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/config"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/service"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/store"
	"github.com/parlakisik/agent-exchange/internal/telemetry"
)

func main() {
	cfg := config.Load()

	// Setup structured logging with trace correlation
	logLevel := slog.LevelInfo
	if cfg.Environment == "development" {
		logLevel = slog.LevelDebug
	}
	logHandler := telemetry.TraceHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	// Initialize OpenTelemetry tracing
	otlpEndpoint := os.Getenv("OTLP_ENDPOINT")
	tracerShutdown, err := telemetry.InitTracer(context.Background(), "aex-telemetry", otlpEndpoint)
	if err != nil {
		slog.Error("failed to initialize tracer", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := tracerShutdown(context.Background()); err != nil {
			slog.Error("failed to shutdown tracer", "error", err)
		}
	}()

	// Initialize Prometheus metrics
	metricsHandler, err := telemetry.InitMeter("aex-telemetry")
	if err != nil {
		slog.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}

	// Initialize store
	memStore := store.NewMemoryStore(cfg.MaxLogEntries, cfg.MaxMetricItems)

	// Initialize service
	svc := service.New(memStore)

	// Setup HTTP router with metrics endpoint
	mux := http.NewServeMux()
	mux.Handle("/", httpapi.NewRouter(svc))
	mux.Handle("GET /metrics", metricsHandler)

	// Wrap with OpenTelemetry tracing middleware
	handler := telemetry.HTTPMiddleware("aex-telemetry")(mux)

	// Initialize HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("aex-telemetry listening", "port", cfg.Port, "env", cfg.Environment)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)

	slog.Info("server stopped")
}
