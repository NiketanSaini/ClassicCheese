// Package config centralizes env-var loading so every cmd/ binary reads
// connection settings and tunables the same way.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	RedisAddr                 string
	MySQLDSN                  string
	RabbitMQURL               string
	RabbitMQExchange          string
	HTTPPort                  string
	AggregationIntervalSecond int
	RankCrossThreshold        int
}

// Load reads every env var this POC uses, applying defaults where the spec
// defines one. MYSQL_DSN and RABBITMQ_URL carry credentials with no sane
// default, so a missing value fails loudly instead of silently connecting
// to the wrong thing (or nothing).
func Load() (*Config, error) {
	// Populate the process env from .env if present; a missing file is fine
	// since real deployments set these vars directly.
	_ = godotenv.Load()

	cfg := &Config{
		RedisAddr:        getEnv("REDIS_ADDR", "localhost:6379"),
		MySQLDSN:         os.Getenv("MYSQL_DSN"),
		RabbitMQURL:      os.Getenv("RABBITMQ_URL"),
		RabbitMQExchange: getEnv("RABBITMQ_EXCHANGE", "score-events"),
		HTTPPort:         getEnv("HTTP_PORT", "8080"),
	}

	if cfg.MySQLDSN == "" {
		return nil, fmt.Errorf("MYSQL_DSN is not set (expected something like user:pass@tcp(localhost:3306)/leaderboard?parseTime=true)")
	}
	if cfg.RabbitMQURL == "" {
		return nil, fmt.Errorf("RABBITMQ_URL is not set (expected something like amqp://guest:guest@localhost:5672/)")
	}

	interval, err := getEnvInt("AGGREGATION_INTERVAL_SECONDS", 5)
	if err != nil {
		return nil, err
	}
	cfg.AggregationIntervalSecond = interval

	threshold, err := getEnvInt("RANK_CROSS_THRESHOLD", 3)
	if err != nil {
		return nil, err
	}
	cfg.RankCrossThreshold = threshold

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q: %w", key, raw, err)
	}
	return v, nil
}
