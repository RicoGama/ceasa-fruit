package server

import (
	"context"
	"errors"
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
