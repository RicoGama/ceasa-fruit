// Package config loads application settings from environment variables.
//
// Go projects usually avoid configuration frameworks: os.LookupEnv plus
// defaults covers most needs and keeps everything explicit.
package config

import (
	"os"
	"strings"
)

// Config holds everything the API needs to start.
// Capitalized fields are "exported" (visible outside this package).
type Config struct {
	Port        string
	DatabaseURL string
	CORSOrigins []string
}

// Load builds a Config from the environment, falling back to dev defaults.
func Load() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://ceasa:ceasa@localhost:5432/ceasa?sslmode=disable"),
		CORSOrigins: strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000"), ","),
	}
}

// getEnv starts with a lowercase letter, so it is private to this package.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
