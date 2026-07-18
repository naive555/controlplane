// Package config loads and validates process configuration from environment
// variables, mirroring the env contract documented in docs/02-api-contract.md.
//
// Unlike the source Node app (which only checked REDIS_URL at boot), Load
// fails fast on ANY missing or invalid required variable and reports all
// problems at once.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const minSecretLen = 32

type Config struct {
	AppName  string
	AppEnv   string
	Port     string
	LogLevel string

	DatabaseURL string
	RedisURL    string

	JWTAccessSecret     string
	JWTRefreshSecret    string
	JWTAccessExpiresIn  time.Duration
	JWTRefreshExpiresIn time.Duration
}

// Load reads configuration from the environment, applies defaults, and
// validates required fields. It returns a single error aggregating every
// problem found so an operator can fix them all in one pass.
func Load() (*Config, error) {
	cfg := &Config{
		AppName:  getEnv("APP_NAME", "controlplane-api"),
		AppEnv:   getEnv("APP_ENV", "development"),
		Port:     getEnv("PORT", "3000"),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),

		JWTAccessSecret:  os.Getenv("JWT_ACCESS_SECRET"),
		JWTRefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),
	}

	var problems []string

	if cfg.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		problems = append(problems, "REDIS_URL is required")
	}

	if len(cfg.JWTAccessSecret) < minSecretLen {
		problems = append(problems, fmt.Sprintf("JWT_ACCESS_SECRET must be at least %d characters", minSecretLen))
	}
	if len(cfg.JWTRefreshSecret) < minSecretLen {
		problems = append(problems, fmt.Sprintf("JWT_REFRESH_SECRET must be at least %d characters", minSecretLen))
	}

	accessExp, err := time.ParseDuration(getEnv("JWT_ACCESS_EXPIRES_IN", "15m"))
	if err != nil {
		problems = append(problems, fmt.Sprintf("JWT_ACCESS_EXPIRES_IN is not a valid duration: %v", err))
	} else {
		cfg.JWTAccessExpiresIn = accessExp
	}

	refreshExpSeconds, err := strconv.Atoi(getEnv("JWT_REFRESH_EXPIRES_IN", "604800"))
	if err != nil {
		problems = append(problems, fmt.Sprintf("JWT_REFRESH_EXPIRES_IN is not a valid integer (seconds): %v", err))
	} else {
		cfg.JWTRefreshExpiresIn = time.Duration(refreshExpSeconds) * time.Second
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
