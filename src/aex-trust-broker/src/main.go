package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/config"
	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/service"
	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/store"
	"github.com/parlakisik/agent-exchange/internal/telemetry"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Setup structured logging with trace correlation
	logHandler := telemetry.TraceHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	cfg := config.Load()

	// Initialize OpenTelemetry tracing
	otlpEndpoint := os.Getenv("OTLP_ENDPOINT")
	tracerShutdown, err := telemetry.InitTracer(context.Background(), "aex-trust-broker", otlpEndpoint)
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
	metricsHandler, err := telemetry.InitMeter("aex-trust-broker")
	if err != nil {
		slog.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}

	var st store.Store
	var mongoClient *mongo.Client
	if cfg.MongoURI != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
		if err != nil {
			slog.Error("failed to connect to mongo", "error", err)
			os.Exit(1)
		}
		mongoClient = c

		ms := store.NewMongoStore(c, cfg.MongoDatabase, cfg.MongoCollectionTrust, cfg.MongoCollectionOutcomes)
		if err := ms.EnsureIndexes(ctx); err != nil {
			slog.Warn("mongo index creation failed", "error", err)
		}
		st = ms
		slog.Info("mongo enabled", "uri", cfg.MongoURI, "db", cfg.MongoDatabase)
	} else {
		st = store.NewMemoryStore()
		slog.Info("mongo disabled (set MONGO_URI to enable)")
	}

	svc := service.New(st)

	// Setup HTTP router with metrics endpoint
	mux := http.NewServeMux()
	mux.Handle("/", httpapi.NewRouter(svc))
	mux.Handle("GET /metrics", metricsHandler)

	// Wrap with OpenTelemetry tracing middleware
	handler := telemetry.HTTPMiddleware("aex-trust-broker")(mux)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	go func() {
		slog.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	if mongoClient != nil {
		_ = mongoClient.Disconnect(shutdownCtx)
	}

	slog.Info("server stopped")
}
