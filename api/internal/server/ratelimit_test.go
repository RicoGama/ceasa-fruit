package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

type loginResult struct {
	Status     int
	RetryAfter int
	Minutes    float64
}

// tryLogin tenta entrar vindo de um IP (passando pelo "proxy" local).
func tryLogin(t *testing.T, base, email, password, ip string) loginResult {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"email": email, "password": password})
	req, _ := http.NewRequest("POST", base+"/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := loginResult{Status: resp.StatusCode}
	r.RetryAfter, _ = strconv.Atoi(resp.Header.Get("Retry-After"))
	var body struct {
		Errors []struct {
			Value float64 `json:"value"`
		} `json:"errors"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Errors) > 0 {
		r.Minutes = body.Errors[0].Value
	}
	return r
}

func TestIntegrationLoginRateLimit(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	ctx := context.Background()
	if _, err := env.pool.Exec(ctx, "TRUNCATE access_log RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	newUser := func(email string) {
		hash, _ := auth.HashPassword("senha-certa-123")
		if _, err := db.New(env.pool).CreateUser(ctx, db.CreateUserParams{
			Name: email, Email: email, PasswordHash: hash, RoleID: roleID(t, env.pool, "seller"),
		}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(email, reason string) int {
		var n int
		_ = env.pool.QueryRow(ctx, "SELECT count(*) FROM access_log WHERE email = $1 AND end_reason = $2", email, reason).Scan(&n)
		return n
	}
	base := env.srv.URL

	// --- 5 erros bloqueiam; até a senha certa é recusada --------------------------
	newUser("ana@test.com")
	for i := range auth.MaxEmailFailures {
		if r := tryLogin(t, base, "ana@test.com", "errada", "10.0.0.1"); r.Status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i+1, r.Status)
		}
	}
	r := tryLogin(t, base, "ana@test.com", "senha-certa-123", "10.0.0.2")
	if r.Status != http.StatusTooManyRequests || r.Minutes != 15 || r.RetryAfter < 890 || r.RetryAfter > 900 {
		t.Fatalf("locked: %+v", r)
	}
	// Insistir durante o bloqueio não renova o bloqueio (só registra).
	tryLogin(t, base, "ana@test.com", "errada", "10.0.0.1")
	if w, l := count("ana@test.com", "wrong_password"), count("ana@test.com", "too_many_attempts"); w != 5 || l != 2 {
		t.Errorf("ana: wrong=%d locked=%d", w, l)
	}
	// Passados 15 minutos (voltamos o relógio das falhas), entra normalmente.
	if _, err := env.pool.Exec(ctx, "UPDATE access_log SET started_at = started_at - interval '16 minutes' WHERE email = 'ana@test.com'"); err != nil {
		t.Fatal(err)
	}
	if r := tryLogin(t, base, "ana@test.com", "senha-certa-123", "10.0.0.1"); r.Status != http.StatusOK {
		t.Fatalf("after window: %+v", r)
	}

	// --- Um login certo zera a contagem ------------------------------------------
	newUser("bia@test.com")
	for range 4 {
		tryLogin(t, base, "bia@test.com", "errada", "10.0.0.3")
	}
	tryLogin(t, base, "bia@test.com", "senha-certa-123", "10.0.0.3")
	for range 4 {
		tryLogin(t, base, "bia@test.com", "errada", "10.0.0.3")
	}
	if r := tryLogin(t, base, "bia@test.com", "senha-certa-123", "10.0.0.3"); r.Status != http.StatusOK {
		t.Fatalf("bia after a success in between: %+v", r)
	}

	// --- Rajada: 30 tentativas ao mesmo tempo, só 5 contam ------------------------
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tryLogin(t, base, "alvo@test.com", "chute", "10.0.0.4")
		}()
	}
	wg.Wait()
	if w, l := count("alvo@test.com", "unknown_email"), count("alvo@test.com", "too_many_attempts"); w != 5 || l != 25 {
		t.Errorf("burst: counted=%d blocked=%d, want 5 and 25", w, l)
	}

	// --- Por IP: 20 falhas com e-mails diferentes bloqueiam o endereço ----------
	for i := range auth.MaxIPFailures {
		tryLogin(t, base, "spray"+strconv.Itoa(i)+"@test.com", "123456", "10.0.0.66")
	}
	newUser("caio@test.com")
	if r := tryLogin(t, base, "caio@test.com", "senha-certa-123", "10.0.0.66"); r.Status != http.StatusTooManyRequests {
		t.Fatalf("ip limit: %+v", r)
	}
	if r := tryLogin(t, base, "caio@test.com", "senha-certa-123", "10.0.0.7"); r.Status != http.StatusOK {
		t.Fatalf("same user from another ip: %+v", r)
	}
}
