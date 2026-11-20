package server

import (
	"context"
	"encoding/json"
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
	for _, path := range []string{`"/suppliers"`, `"/auth/login"`, `"/users"`, `"/company"`, `"sessionCookie"`} {
		if !strings.Contains(rec.Body.String(), path) {
			t.Errorf("openapi.json is missing %s", path)
		}
	}
}

// Sem cookie, as rotas protegidas respondem 401 antes de tocar no banco.
func TestProtectedRoutesRequireLogin(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Deps{Logger: logger})

	for _, path := range []string{"/suppliers", "/auth/me", "/users", "/company", "/cities?search=rio"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s: status = %d, want 401", path, rec.Code)
		}
	}
}

// O IP do cliente que vai para o log não pode ser forjado: o cabeçalho
// X-Forwarded-For só vale quando a conexão vem de um proxy confiável.
func TestClientIPCannotBeSpoofed(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string // quem abriu a conexão TCP
		xff        string // o que veio no cabeçalho
		want       string
	}{
		{"direct client faking the header", "192.168.0.50:51000", "10.9.9.9", "client_ip=192.168.0.50"},
		{"through the local proxy (Nuxt)", "127.0.0.1:40000", "192.168.0.77", "client_ip=192.168.0.77"},
		{"proxy chain: only the last untrusted hop counts", "127.0.0.1:40000", "6.6.6.6, 192.168.0.77", "client_ip=192.168.0.77"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var logs strings.Builder
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			h := New(Deps{Logger: logger})

			req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
			req.RemoteAddr = c.remoteAddr
			req.Header.Set("X-Forwarded-For", c.xff)
			h.ServeHTTP(httptest.NewRecorder(), req)

			if !strings.Contains(logs.String(), c.want) {
				t.Errorf("log = %s, want it to contain %s", logs.String(), c.want)
			}
		})
	}
}

// TestEveryRouteHasPermission garante que nenhuma rota de negócio fica sem
// permissão: toda rota que exige login precisa ter x-permissions no OpenAPI
// (anotado por auth.Protect), a não ser as poucas que valem para qualquer
// usuário logado, listadas aqui de propósito.
func TestEveryRouteHasPermission(t *testing.T) {
	anyLoggedUser := map[string]bool{
		"get-me":        true, // quem sou eu
		"search-cities": true, // lista pública do IBGE, usada em vários formulários
		"get-company":   true, // nome da empresa no menu e nos documentos
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rec := httptest.NewRecorder()
	New(Deps{Logger: logger}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string           `json:"operationId"`
			Security    []map[string]any `json:"security"`
			Permissions []string         `json:"x-permissions"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for path, ops := range spec.Paths {
		for method, op := range ops {
			if len(op.Security) == 0 || anyLoggedUser[op.OperationID] {
				continue // rota pública (login, senha) ou liberada de propósito
			}
			checked++
			if len(op.Permissions) == 0 {
				t.Errorf("%s %s (%s) requires login but declares no permission; wrap it with auth.Protect",
					strings.ToUpper(method), path, op.OperationID)
			}
		}
	}
	if checked < 15 {
		t.Fatalf("only %d protected operations found; is the spec being read?", checked)
	}
}
