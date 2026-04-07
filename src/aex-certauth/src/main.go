package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-certauth/internal/clients"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/config"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/service"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/store"
	"github.com/parlakisik/agent-exchange/internal/events"
	aexnats "github.com/parlakisik/agent-exchange/internal/nats"
	"github.com/parlakisik/agent-exchange/internal/telemetry"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

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
	tracerShutdown, err := telemetry.InitTracer(context.Background(), "aex-certauth", otlpEndpoint)
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
	metricsHandler, err := telemetry.InitMeter("aex-certauth")
	if err != nil {
		slog.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}

	slog.Info("starting aex-certauth",
		"environment", cfg.Environment,
		"port", cfg.Port,
	)

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(cfg.MongoURI)
	mongoClient, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		slog.Error("failed to connect to mongodb", "error", err)
		os.Exit(1)
	}

	if err := mongoClient.Ping(ctx, nil); err != nil {
		slog.Error("failed to ping mongodb", "error", err)
		os.Exit(1)
	}

	// Initialize store
	certStore := store.NewMongoCertAuthStore(mongoClient, cfg.MongoDB)
	if err := certStore.EnsureIndexes(ctx); err != nil {
		slog.Warn("failed to create indexes", "error", err)
	}
	slog.Info("using mongodb store", "db", cfg.MongoDB)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mongoClient.Disconnect(ctx); err != nil {
			slog.Error("failed to disconnect mongodb", "error", err)
		}
	}()

	// Initialize CA engine
	caEngine, err := service.NewCAEngine(cfg.CAKeyPath)
	if err != nil {
		slog.Error("failed to initialize CA engine", "error", err)
		os.Exit(1)
	}
	slog.Info("CA engine initialized")

	// Initialize NATS and event publisher
	publisher := events.NewPublisher("aex-certauth")
	if cfg.WebhookSecret != "" {
		publisher.WithWebhookSecret(cfg.WebhookSecret)
	}
	if cfg.NatsURL != "" {
		natsCfg := aexnats.DefaultConfig()
		natsCfg.URL = cfg.NatsURL
		natsCfg.Name = "aex-certauth"
		natsCfg.StreamReplicas = cfg.NatsStreamReplicas
		natsClient, natsErr := aexnats.Connect(natsCfg)
		if natsErr != nil {
			slog.Warn("failed to connect to NATS, events will be log-only", "error", natsErr)
		} else {
			if err := natsClient.EnsureStreams(); err != nil {
				slog.Warn("failed to ensure NATS streams", "error", err)
			}
			publisher.WithNATS(natsClient)
			slog.Info("NATS connected", "url", cfg.NatsURL, "replicas", cfg.NatsStreamReplicas)
			defer func() {
				if err := natsClient.Close(); err != nil {
					slog.Error("failed to close NATS", "error", err)
				}
			}()
		}
	}

	// Initialize clients
	trustClient := clients.NewTrustBrokerClient(cfg.TrustBrokerURL)

	// Initialize services
	certSvc := service.NewCertificateService(certStore, caEngine, publisher, cfg)
	repSvc := service.NewReputationService(certStore, trustClient, publisher)
	verifySvc := service.NewVerificationService(certStore, caEngine, publisher)

	// Setup HTTP router
	mux := http.NewServeMux()
	mux.Handle("/", httpapi.NewRouter(certSvc, repSvc, verifySvc, caEngine))
	mux.Handle("GET /metrics", metricsHandler)

	// Wrap with OpenTelemetry tracing middleware
	handler := telemetry.HTTPMiddleware("aex-certauth")(mux)

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case err := <-serverErr:
		slog.Error("http server error", "error", err)
	}

	slog.Info("shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
