package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePinger permite simular um banco saudável ou fora do ar.
type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

// Testando um handler net/http puro: monta a requisição e grava a resposta.
func TestHealthHandler(t *testing.T) {
	cases := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{"database up", nil, http.StatusOK, `"database":"ok"`},
		{"database down", errors.New("boom"), http.StatusServiceUnavailable, `"database":"unreachable"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			rec := httptest.NewRecorder()

			healthHandler(fakePinger{err: c.pingErr}).ServeHTTP(rec, req)

			if rec.Code != c.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, c.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), c.wantBody) {
				t.Errorf("body = %s, want it to contain %s", rec.Body.String(), c.wantBody)
			}
		})
	}
}

// TestNewBuildsOpenAPI monta o servidor inteiro (sem banco) e baixa o OpenAPI.
// O Huma valida os tipos ao registrar as rotas (ex.: dois schemas com o mesmo
// nome entram em pânico), então este teste pega esse tipo de erro sem Postgres.
func TestNewBuildsOpenAPI(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Deps{Logger: logger}) // Pool nil: nenhuma rota que use o banco é chamada

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, path := range []string{`"/suppliers"`, `"/auth/login"`, `"/admin/organizations"`, `"sessionCookie"`} {
		if !strings.Contains(rec.Body.String(), path) {
			t.Errorf("openapi.json is missing %s", path)
		}
	}
}

// Sem cookie, as rotas protegidas respondem 401 antes de tocar no banco.
func TestProtectedRoutesRequireLogin(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Deps{Logger: logger})

	for _, path := range []string{"/suppliers", "/auth/me", "/admin/organizations"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s: status = %d, want 401", path, rec.Code)
		}
	}
}
