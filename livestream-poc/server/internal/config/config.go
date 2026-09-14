// Package config centralizes env-var loading for the livestream server.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort        string
	LiveKitAPIKey   string
	LiveKitSecret   string
	CORSAllowOrigin string
}

// Load reads env vars, applying defaults where the spec defines one.
// LIVEKIT_API_KEY/LIVEKIT_API_SECRET have no sane default, so a missing
// value fails loudly instead of minting tokens LiveKit will reject.
func Load() (*Config, error) {
	// Populate the process env from .env if present; a missing file is fine
	// since real deployments set these vars directly.
	_ = godotenv.Load()

	cfg := &Config{
		HTTPPort:        getEnv("HTTP_PORT", "8080"),
		LiveKitAPIKey:   os.Getenv("LIVEKIT_API_KEY"),
		LiveKitSecret:   os.Getenv("LIVEKIT_API_SECRET"),
		CORSAllowOrigin: getEnv("CORS_ALLOW_ORIGIN", "*"),
	}

	if cfg.LiveKitAPIKey == "" {
		return nil, fmt.Errorf("LIVEKIT_API_KEY is not set")
	}
	if cfg.LiveKitSecret == "" {
		return nil, fmt.Errorf("LIVEKIT_API_SECRET is not set")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
