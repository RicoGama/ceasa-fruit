// Package server monta o roteador HTTP, os middlewares e todas as rotas da API.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RicoGama/ceasa-fruit/api/internal/admin"
	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/city"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/supplier"
)

func init() {
	// Por padrão o Huma declara listas como "podem ser null" no OpenAPI, porque
	// um slice nil do Go vira null no JSON. Nossos handlers sempre criam a
	// lista com make(), então devolvem [] e nunca null: avisamos o OpenAPI
	// para o TypeScript do front não precisar tratar um null que nunca vem.
	huma.DefaultArrayNullable = false
}

// Pinger é qualquer coisa capaz de testar a conexão com o banco.
// *pgxpool.Pool implementa; os testes podem usar um falso.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps agrupa o que o servidor precisa. Passar dependências explicitamente
// (em vez de variáveis globais) deixa o código mais fácil de seguir e testar.
type Deps struct {
	Logger       *slog.Logger
	Pool         *pgxpool.Pool
	CORSOrigins  []string
	CookieSecure bool // true em produção (HTTPS)
}

// New devolve o http.Handler raiz da aplicação.
func New(d Deps) http.Handler {
	h, _ := build(d)
	return h
}

// OpenAPISpec devolve a especificação OpenAPI em JSON, sem subir servidor nem
// conectar no banco. Usada pelo cmd/openapi para gerar o arquivo que o front lê.
func OpenAPISpec() ([]byte, error) {
	_, api := build(Deps{Logger: slog.New(slog.DiscardHandler)})
	return json.MarshalIndent(api.OpenAPI(), "", "  ")
}

// build monta o roteador e devolve também a API do Huma (para o OpenAPISpec).
func build(d Deps) (http.Handler, huma.API) {
	r := chi.NewRouter()
	queries := db.New(d.Pool)

	// Middlewares envolvem cada requisição, na ordem em que são adicionados.
	r.Use(middleware.RequestID) // adiciona um X-Request-Id em cada requisição
	r.Use(middleware.RealIP)    // usa o X-Forwarded-For quando atrás de um proxy
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer) // transforma um panic em 500 em vez de derrubar o servidor
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: d.CORSOrigins,
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Content-Type"},
		// Necessário para o navegador enviar o cookie de sessão em requisições
		// vindas do front (outra porta/subdomínio).
		AllowCredentials: true,
		MaxAge:           300,
	}))
	// Lê o cookie de sessão (se houver) e coloca o usuário no context.
	r.Use(auth.LoadSession(queries, d.Logger))

	// Rota com chi + net/http puro, escrita à mão de propósito (ver healthHandler).
	r.Get("/health", healthHandler(d.Pool))

	// O Huma fica por cima do mesmo roteador chi. Ele também serve:
	//   /docs          -> documentação interativa da API
	//   /openapi.json  -> a especificação OpenAPI (usada para gerar o cliente do Nuxt)
	config := huma.DefaultConfig("CEASA Fruit API", "0.1.0")
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		auth.SecuritySchemeName: auth.SecurityScheme,
	}
	api := humachi.New(r, config)

	// Grupos de rotas: cada grupo aplica um middleware de acesso a todas as
	// rotas registradas nele. Esquecer a proteção numa rota fica bem difícil,
	// porque a rota é registrada já dentro do grupo certo.
	public := huma.NewGroup(api)

	authed := huma.NewGroup(api) // exige login
	authed.UseMiddleware(auth.RequireUser(api))
	authed.UseSimpleModifier(auth.RequireSecurity)

	tenant := huma.NewGroup(api) // exige login + organização ativa
	tenant.UseMiddleware(auth.RequireOrganization(api))
	tenant.UseSimpleModifier(auth.RequireSecurity)

	platformAdmin := huma.NewGroup(api) // exige administrador da plataforma
	platformAdmin.UseMiddleware(auth.RequirePlatformAdmin(api))
	platformAdmin.UseSimpleModifier(auth.RequireSecurity)

	auth.Register(public, authed, d.Pool, auth.Cookies{Secure: d.CookieSecure})
	admin.Register(platformAdmin, queries)
	city.Register(authed, queries)

	// Rotas de negócio: cada requisição roda numa transação com RLS da organização.
	tenantTx := func(ctx context.Context, orgID int64, fn func(supplier.Store) error) error {
		return postgres.InTenant(ctx, d.Pool, orgID, func(q *db.Queries) error { return fn(q) })
	}
	supplier.Register(tenant, tenantTx)

	return r, api
}

// healthHandler mostra como é um handler SEM o Huma:
// você recebe o ResponseWriter e o Request crus e faz tudo sozinho.
//
// Ele devolve uma função (uma closure) que "captura" a variável p.
func healthHandler(p Pinger) http.HandlerFunc {
	type response struct {
		Status   string `json:"status"`
		Database string `json:"database"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		resp := response{Status: "ok", Database: "ok"}
		status := http.StatusOK
		if err := p.Ping(ctx); err != nil {
			resp = response{Status: "degraded", Database: "unreachable"}
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status) // headers precisam ser definidos ANTES do WriteHeader
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// requestLogger é um middleware: uma função que recebe o próximo handler e
// devolve um novo handler que faz algo antes/depois de chamá-lo.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			// Envolve o writer para conseguirmos ler o status code depois.
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
