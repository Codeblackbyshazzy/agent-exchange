package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/config"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/service"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/store"
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
	tracerShutdown, err := telemetry.InitTracer(context.Background(), "aex-work-publisher", otlpEndpoint)
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
	metricsHandler, err := telemetry.InitMeter("aex-work-publisher")
	if err != nil {
		slog.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}

	slog.Info("starting aex-work-publisher",
		"environment", cfg.Environment,
		"port", cfg.Port,
		"store_type", cfg.StoreType,
	)

	// Initialize store
	var workStore store.WorkStore
	var mongoClient *mongo.Client

	switch cfg.StoreType {
	case "mongo":
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		clientOpts := options.Client().ApplyURI(cfg.MongoURI)
		var mongoErr error
		mongoClient, mongoErr = mongo.Connect(ctx, clientOpts)
		if mongoErr != nil {
			slog.Error("failed to connect to mongodb", "error", mongoErr)
			os.Exit(1)
		}

		if err := mongoClient.Ping(ctx, nil); err != nil {
			slog.Error("failed to ping mongodb", "error", err)
			os.Exit(1)
		}

		mongoStore := store.NewMongoWorkStore(mongoClient, cfg.MongoDB, cfg.MongoCollection)
		if err := mongoStore.EnsureIndexes(ctx); err != nil {
			slog.Warn("failed to create indexes", "error", err)
		}
		workStore = mongoStore
		slog.Info("using mongodb store", "uri", cfg.MongoURI, "db", cfg.MongoDB, "collection", cfg.MongoCollection)

	case "firestore":
		var storeErr error
		workStore, storeErr = store.NewFirestoreStore(cfg.FirestoreProjectID, cfg.FirestoreCollection)
		if storeErr != nil {
			slog.Error("failed to initialize firestore", "error", storeErr)
			os.Exit(1)
		}
		slog.Info("using firestore store", "project", cfg.FirestoreProjectID, "collection", cfg.FirestoreCollection)

	default:
		workStore = store.NewMemoryStore()
		slog.Info("using in-memory store (development mode)")
	}
	defer func() { _ = workStore.Close() }()
	if mongoClient != nil {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := mongoClient.Disconnect(ctx); err != nil {
				slog.Error("failed to disconnect mongodb", "error", err)
			}
		}()
	}

	// Initialize NATS and event publisher
	publisher := events.NewPublisher("aex-work-publisher")
	if cfg.WebhookSecret != "" {
		publisher.WithWebhookSecret(cfg.WebhookSecret)
	}
	if cfg.NatsURL != "" {
		natsCfg := aexnats.DefaultConfig()
		natsCfg.URL = cfg.NatsURL
		natsCfg.Name = "aex-work-publisher"
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

	// Initialize service
	svc := service.New(workStore, cfg.ProviderRegistryURL, publisher)

	// Setup HTTP router
	mux := http.NewServeMux()
	mux.Handle("/", httpapi.NewRouter(svc))
	mux.Handle("GET /metrics", metricsHandler)

	// Wrap with OpenTelemetry tracing middleware
	handler := telemetry.HTTPMiddleware("aex-work-publisher")(mux)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
