// Package server wires the HTTP router, middlewares and all API routes.
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

// Pinger is anything that can check the database connection.
// *pgxpool.Pool implements it; tests can use a fake.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps groups what the server needs. Passing dependencies explicitly
// (instead of globals) makes the code easier to follow and to test.
type Deps struct {
	Logger      *slog.Logger
	DB          db.DBTX // the connection pool, used by the sqlc queries
	Pinger      Pinger
	CORSOrigins []string
}

// New returns the root http.Handler of the application.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	// Middlewares wrap every request, in the order they are added.
	r.Use(middleware.RequestID) // adds an X-Request-Id to each request
	r.Use(middleware.RealIP)    // uses X-Forwarded-For when behind a proxy
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer) // turns a panic into a 500 instead of crashing
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: d.CORSOrigins,
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
		MaxAge:         300,
	}))

	// Plain chi + net/http route, written by hand on purpose (see healthHandler).
	r.Get("/health", healthHandler(d.Pinger))

	// Huma sits on top of the same chi router. It also serves:
	//   /docs          -> interactive API documentation
	//   /openapi.json  -> the OpenAPI spec (used to generate the Nuxt client)
	api := humachi.New(r, huma.DefaultConfig("CEASA Fruit API", "0.1.0"))

	supplier.Register(api, db.New(d.DB))

	return r
}

// healthHandler shows what a handler looks like WITHOUT Huma:
// you receive the raw ResponseWriter and Request and do everything yourself.
//
// It returns a function (a closure) that "captures" the pinger variable.
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
		w.WriteHeader(status) // headers must be set BEFORE WriteHeader
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// requestLogger is a middleware: a function that takes the next handler and
// returns a new handler that does something before/after calling it.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			// Wrap the writer so we can read the status code afterwards.
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
