// Package config carrega as configurações da aplicação a partir de variáveis de ambiente.
//
// Em Go é comum evitar frameworks de configuração: os.LookupEnv + valores
// padrão resolvem a maioria dos casos e deixam tudo explícito.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
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
	// TrustedProxies: endereços (CIDR) dos proxies na frente da API. Só deles
	// aceitamos o cabeçalho X-Forwarded-For para saber o IP real do cliente.
	// Padrão: a própria máquina (o Nuxt repassa /api/* para cá).
	TrustedProxies []string
	SMTP           SMTP
	Fleet          Fleet
	// CNPJ: endereços dos serviços de consulta (trocados nos testes).
	CNPJBrasilAPIURL string
	CNPJCNPJaURL     string
	// SecretsKey criptografa segredos no banco (o certificado digital).
	// Texto aleatório de pelo menos 32 caracteres; a instalação gera.
	// Vazio: o sistema funciona, mas não aceita certificado.
	SecretsKey string
}

// Fleet: rastreamento dos caminhões pelo Traccar (servidor na nuvem).
// Sem TraccarURL, a frota funciona só como cadastro (sem posições).
type Fleet struct {
	TraccarURL   string // ex.: https://rastreio.empresa.com.br
	TraccarToken string // Traccar: Configurações > Preferências > Token
	// SyncInterval: de quanto em quanto tempo busca posições novas.
	SyncInterval time.Duration
	// RetentionDays: quantos dias de trajeto guardar (0 = para sempre).
	RetentionDays int
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
		TrustedProxies: strings.Split(getEnv("TRUSTED_PROXIES", "127.0.0.1/32,::1/128"), ","),
		// Os padrões apontam para o Mailpit, o SMTP de desenvolvimento.
		SMTP: SMTP{
			Host:     getEnv("SMTP_HOST", "localhost"),
			Port:     getEnvInt("SMTP_PORT", 1025),
			Username: getEnv("SMTP_USERNAME", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
			TLS:      getEnv("SMTP_TLS", "none"),
			From:     getEnv("SMTP_FROM", "CEASA Fruit <nao-responda@ceasafruit.test>"),
		},
		CNPJBrasilAPIURL: getEnv("CNPJ_BRASILAPI_URL", "https://brasilapi.com.br/api/cnpj/v1"),
		CNPJCNPJaURL:     getEnv("CNPJ_CNPJA_URL", "https://open.cnpja.com/office"),
		SecretsKey:       getEnv("SECRETS_KEY", ""),
		Fleet: Fleet{
			TraccarURL:    strings.TrimSuffix(getEnv("TRACCAR_URL", ""), "/"),
			TraccarToken:  getEnv("TRACCAR_TOKEN", ""),
			SyncInterval:  time.Duration(getEnvInt("FLEET_SYNC_SECONDS", 15)) * time.Second,
			RetentionDays: getEnvInt("FLEET_RETENTION_DAYS", 180),
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
