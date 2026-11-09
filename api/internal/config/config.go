// Package config carrega as configurações da aplicação a partir de variáveis de ambiente.
//
// Em Go é comum evitar frameworks de configuração: os.LookupEnv + valores
// padrão resolvem a maioria dos casos e deixam tudo explícito.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config reúne tudo o que a API precisa para subir.
// Campos com letra maiúscula são "exportados" (visíveis fora deste pacote).
type Config struct {
	Port        string
	DatabaseURL string
	CORSOrigins []string
	// CookieSecure faz o cookie de sessão só trafegar em HTTPS. Em
	// desenvolvimento (http://localhost) precisa ser false.
	CookieSecure bool
	// AppURL é o endereço do front, usado nos links dos e-mails
	// (ex.: link de redefinição de senha).
	AppURL string
	// WorkersEnabled liga os workers da fila neste processo. Em produção dá
	// para rodar a API com false e os workers em outro processo (cmd/worker).
	WorkersEnabled bool
	SMTP           SMTP
}

// SMTP: todo envio de e-mail do sistema sai por SMTP.
type SMTP struct {
	Host     string
	Port     int
	Username string // vazio = sem autenticação (ex.: Mailpit em desenvolvimento)
	Password string
	// TLS: "none" (sem criptografia, só para desenvolvimento), "starttls"
	// (porta 587, o mais comum) ou "tls" (TLS direto, porta 465).
	TLS  string
	From string // remetente, ex.: "CEASA Fruit <nao-responda@empresa.com.br>"
}

// Load monta uma Config a partir do ambiente, com padrões de desenvolvimento.
func Load() Config {
	return Config{
		Port:           getEnv("PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://ceasa:ceasa@localhost:5432/ceasa?sslmode=disable"),
		CORSOrigins:    strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000"), ","),
		CookieSecure:   getEnv("COOKIE_SECURE", "false") == "true",
		AppURL:         strings.TrimSuffix(getEnv("APP_URL", "http://localhost:3000"), "/"),
		WorkersEnabled: getEnv("WORKERS_ENABLED", "true") == "true",
		// Os padrões apontam para o Mailpit, o SMTP de desenvolvimento.
		SMTP: SMTP{
			Host:     getEnv("SMTP_HOST", "localhost"),
			Port:     getEnvInt("SMTP_PORT", 1025),
			Username: getEnv("SMTP_USERNAME", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
			TLS:      getEnv("SMTP_TLS", "none"),
			From:     getEnv("SMTP_FROM", "CEASA Fruit <nao-responda@ceasafruit.test>"),
		},
	}
}

// getEnv começa com letra minúscula, então é privada deste pacote.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if n, err := strconv.Atoi(getEnv(key, "")); err == nil {
		return n
	}
	return fallback
}
