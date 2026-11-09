// Package config carrega as configurações da aplicação a partir de variáveis de ambiente.
//
// Em Go é comum evitar frameworks de configuração: os.LookupEnv + valores
// padrão resolvem a maioria dos casos e deixam tudo explícito.
package config

import (
	"os"
	"strings"
)

// Config reúne tudo o que a API precisa para subir.
// Campos com letra maiúscula são "exportados" (visíveis fora deste pacote).
type Config struct {
	Port        string
	DatabaseURL string
	CORSOrigins []string
}

// Load monta uma Config a partir do ambiente, com padrões de desenvolvimento.
func Load() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://ceasa:ceasa@localhost:5432/ceasa?sslmode=disable"),
		CORSOrigins: strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000"), ","),
	}
}

// getEnv começa com letra minúscula, então é privada deste pacote.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
