package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

type accessEntry struct {
	Kind      string  `json:"kind"`
	UserID    *int64  `json:"userId"`
	Email     string  `json:"email"`
	IP        *string `json:"ip"`
	UserAgent *string `json:"userAgent"`
	Status    string  `json:"status"`
	EndReason *string `json:"endReason"`
}

type accessList struct {
	Items []accessEntry `json:"items"`
	Total int64         `json:"total"`
}

// loginFrom faz login como um celular na rede do box: passa pelo "proxy"
// (a conexão vem de 127.0.0.1) informando o IP real e o navegador.
func loginFrom(t *testing.T, c *client, email, password, ip, ua string) int {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"email": email, "password": password})
	req, _ := http.NewRequest("POST", c.base+"/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ip)
	req.Header.Set("User-Agent", ua)
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestIntegrationAccessLog(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	ctx := context.Background()
	if _, err := env.pool.Exec(ctx, "TRUNCATE access_log RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	const android = "Mozilla/5.0 (Linux; Android 14; SM-A546E) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36"

	admin := newClient(t, env.srv.URL)
	if st := loginFrom(t, admin, "admin@test.com", "admin-password", "192.168.0.20", android); st != http.StatusOK {
		t.Fatalf("admin login: %d", st)
	}

	// Usuários de teste: um vendedor e um que vai ser desativado.
	newUser := func(email, role string) int64 {
		hash, _ := auth.HashPassword("senha-de-teste-1")
		u, err := db.New(env.pool).CreateUser(ctx, db.CreateUserParams{
			Name: email, Email: email, PasswordHash: hash, RoleID: roleID(t, env.pool, role),
		})
		if err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	sellerID := newUser("vendedor@test.com", "seller")
	newUser("nunca@test.com", "seller") // nunca entra

	// --- Falhas -------------------------------------------------------------------
	anon := newClient(t, env.srv.URL)
	loginFrom(t, anon, "admin@test.com", "senha-errada", "192.168.0.99", "curl/8")
	loginFrom(t, anon, "ninguem@test.com", "qualquer", "192.168.0.99", "curl/8")

	// --- Vendedor entra e sai -------------------------------------------------------
	seller := newClient(t, env.srv.URL)
	loginFrom(t, seller, "vendedor@test.com", "senha-de-teste-1", "192.168.0.30", android)
	seller.expect(http.StatusNoContent, "POST", "/auth/logout", nil, nil)
	// ...entra de novo e é desativado com a sessão aberta.
	loginFrom(t, seller, "vendedor@test.com", "senha-de-teste-1", "192.168.0.30", android)
	var users struct {
		Items []struct {
			ID         int64   `json:"id"`
			Email      string  `json:"email"`
			Version    int32   `json:"version"`
			LastSeenAt *string `json:"lastSeenAt"`
		} `json:"items"`
	}
	admin.expect(http.StatusOK, "GET", "/users", nil, &users)
	for _, u := range users.Items {
		switch u.Email {
		case "nunca@test.com":
			if u.LastSeenAt != nil {
				t.Errorf("never logged in user has lastSeenAt %v", *u.LastSeenAt)
			}
		case "admin@test.com":
			if u.LastSeenAt == nil {
				t.Error("admin has no lastSeenAt")
			}
		case "vendedor@test.com":
			admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/users/%d", u.ID), map[string]any{
				"name": "Vendedor", "roleId": roleID(t, env.pool, "seller"), "active": false, "version": u.Version,
			}, nil)
		}
	}
	// Desativado tentando entrar.
	loginFrom(t, anon, "vendedor@test.com", "senha-de-teste-1", "192.168.0.30", android)

	// --- O registro -----------------------------------------------------------------
	var log accessList
	admin.expect(http.StatusOK, "GET", "/access-log", nil, &log)
	got := map[string]int{}
	for _, e := range log.Items {
		key := e.Status
		if e.EndReason != nil {
			key += "/" + *e.EndReason
		}
		got[key]++
	}
	want := map[string]int{
		"online":                1, // o admin, agora
		"failed/wrong_password": 1, "failed/unknown_email": 1, "failed/inactive_user": 1,
		"ended/logout": 1, "ended/deactivated": 1,
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: got %d, want %d (all: %v)", k, got[k], n, got)
		}
	}
	// O mais recente primeiro; IP real e navegador gravados.
	first := log.Items[len(log.Items)-1] // o login do admin foi o primeiro
	if first.Email != "admin@test.com" || first.IP == nil || *first.IP != "192.168.0.20" || first.UserAgent == nil || *first.UserAgent != android {
		t.Errorf("admin entry: %+v", first)
	}
	for _, e := range log.Items {
		if e.Status == "failed" && e.EndReason != nil && *e.EndReason == "unknown_email" && (e.UserID != nil || e.Email != "ninguem@test.com") {
			t.Errorf("unknown email entry: %+v", e)
		}
	}

	// --- Filtros --------------------------------------------------------------------
	admin.expect(http.StatusOK, "GET", "/access-log?kind=failed_login", nil, &log)
	if log.Total != 3 {
		t.Errorf("failed logins = %d", log.Total)
	}
	admin.expect(http.StatusOK, "GET", fmt.Sprintf("/access-log?userId=%d", sellerID), nil, &log)
	if log.Total != 3 { // 2 sessões + 1 tentativa como desativado
		t.Errorf("seller entries = %d", log.Total)
	}

	// --- Ocioso, depois ativo de novo; e sessão vencida ----------------------------
	if _, err := env.pool.Exec(ctx, "UPDATE access_log SET last_seen_at = now() - interval '1 hour' WHERE email = 'admin@test.com' AND kind = 'session'"); err != nil {
		t.Fatal(err)
	}
	admin.expect(http.StatusOK, "GET", "/access-log?kind=session&userId=1", nil, &log)
	// A própria consulta acima já "tocou" a sessão (última atividade velha):
	admin.expect(http.StatusOK, "GET", "/access-log?kind=session&userId=1", nil, &log)
	if log.Items[0].Status != "online" {
		t.Errorf("after activity: %s", log.Items[0].Status)
	}
	if _, err := env.pool.Exec(ctx, "UPDATE sessions SET expires_at = now() - interval '1 minute' WHERE user_id = 1"); err != nil {
		t.Fatal(err)
	}
	other := newClient(t, env.srv.URL)
	loginFrom(t, other, "admin@test.com", "admin-password", "192.168.0.21", "Firefox")
	other.expect(http.StatusOK, "GET", "/access-log?kind=session&userId=1", nil, &log)
	if len(log.Items) != 2 || log.Items[0].Status != "online" || log.Items[1].Status != "expired" {
		t.Errorf("expired session: %+v", log.Items)
	}

	// --- Permissão ------------------------------------------------------------------
	newUser("gerente@test.com", "manager")
	manager := newClient(t, env.srv.URL)
	loginFrom(t, manager, "gerente@test.com", "senha-de-teste-1", "192.168.0.40", "Safari")
	manager.expect(http.StatusForbidden, "GET", "/access-log", nil, nil) // ninguém tem por padrão
}
