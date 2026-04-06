package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port             int
	Environment      string
	MongoURI         string
	MongoDB          string
	TrustBrokerURL   string
	NatsURL            string
	NatsStreamReplicas int
	WebhookSecret      string
	CAKeyPath          string
	CertValidityDays int
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:             getEnvInt("PORT", 8091),
		Environment:      getEnv("ENVIRONMENT", "development"),
		MongoURI:         getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:          getEnv("MONGO_DB", "aex"),
		TrustBrokerURL:   getEnv("TRUST_BROKER_URL", ""),
		NatsURL:            getEnv("NATS_URL", ""),
		NatsStreamReplicas: getEnvInt("NATS_STREAM_REPLICAS", 1),
		WebhookSecret:      getEnv("WEBHOOK_SECRET", ""),
		CAKeyPath:           getEnv("CA_KEY_PATH", ""),
		CertValidityDays: getEnvInt("CERT_VALIDITY_DAYS", 365),
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
