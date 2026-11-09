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

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/supplier"
)

// Pinger é qualquer coisa capaz de testar a conexão com o banco.
// *pgxpool.Pool implementa; os testes podem usar um falso.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps agrupa o que o servidor precisa. Passar dependências explicitamente
// (em vez de variáveis globais) deixa o código mais fácil de seguir e testar.
type Deps struct {
	Logger      *slog.Logger
	DB          db.DBTX // o pool de conexões, usado pelas queries do sqlc
	Pinger      Pinger
	CORSOrigins []string
}

// New devolve o http.Handler raiz da aplicação.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	// Middlewares envolvem cada requisição, na ordem em que são adicionados.
	r.Use(middleware.RequestID) // adiciona um X-Request-Id em cada requisição
	r.Use(middleware.RealIP)    // usa o X-Forwarded-For quando atrás de um proxy
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer) // transforma um panic em 500 em vez de derrubar o servidor
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: d.CORSOrigins,
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
		MaxAge:         300,
	}))

	// Rota com chi + net/http puro, escrita à mão de propósito (ver healthHandler).
	r.Get("/health", healthHandler(d.Pinger))

	// O Huma fica por cima do mesmo roteador chi. Ele também serve:
	//   /docs          -> documentação interativa da API
	//   /openapi.json  -> a especificação OpenAPI (usada para gerar o cliente do Nuxt)
	api := humachi.New(r, huma.DefaultConfig("CEASA Fruit API", "0.1.0"))

	supplier.Register(api, db.New(d.DB))

	return r
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
