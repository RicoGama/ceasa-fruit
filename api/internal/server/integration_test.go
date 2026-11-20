package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/mail"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/queue"
)

// Teste de integração: sobe a API inteira contra um PostgreSQL de verdade.
// Só roda com TEST_DATABASE_URL definida (ver `make test-integration`);
// sem ela é pulado, então `go test ./...` continua funcionando sem banco.

// fakeMailer é o "carteiro" dos testes: em vez de enviar por SMTP, guarda as
// mensagens. O mutex protege a lista, porque os workers da fila rodam em
// outras goroutines ao mesmo tempo que o teste lê.
type fakeMailer struct {
	mu   sync.Mutex
	msgs []mail.Message
}

func (f *fakeMailer) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, m)
	return nil
}

// waitFor espera (até 15s) chegar um e-mail para `to` com `subject` no assunto.
func (f *fakeMailer) waitFor(t *testing.T, to, subject string) mail.Message {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, m := range f.msgs {
			if m.To == to && strings.Contains(m.Subject, subject) {
				f.mu.Unlock()
				return m
			}
		}
		f.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no email %q to %s", subject, to)
	return mail.Message{}
}

func (f *fakeMailer) count(to string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.msgs {
		if m.To == to {
			n++
		}
	}
	return n
}

type integrationEnv struct {
	srv    *httptest.Server
	pool   *pgxpool.Pool
	queue  *queue.Queue
	mailer *fakeMailer
}

func setupIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper() // erros apontam para a linha de quem chamou, não para cá

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close) // roda ao final do teste

	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := queue.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Fila de verdade (River no Postgres), com o carteiro falso.
	mailer := &fakeMailer{}
	q, err := queue.New(pool, queue.Options{Mailer: mailer, Logger: logger, RunWorkers: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = q.Stop(stopCtx)
	})

	srv := httptest.NewServer(New(Deps{Logger: logger, Pool: pool, Queue: q, AppURL: "http://app.test"}))
	t.Cleanup(srv.Close)
	return &integrationEnv{srv: srv, pool: pool, queue: q, mailer: mailer}
}

// client simula um navegador: guarda os cookies entre requisições.
type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

// do envia a requisição e decodifica o JSON da resposta em out (se não for nil).
func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func (c *client) expect(want int, method, path string, body any, out any) {
	c.t.Helper()
	if got := c.do(method, path, body, out); got != want {
		c.t.Fatalf("%s %s: status = %d, want %d", method, path, got, want)
	}
}

type me struct {
	User struct {
		ID   int64  `json:"id"`
		Role string `json:"role"`
	} `json:"user"`
	Company *struct {
		Name string `json:"name"`
	} `json:"company"`
}

type idResp struct {
	ID int64 `json:"id"`
}

// resetDatabase apaga os dados de negócio, para cada teste começar do zero
// (os municípios ficam: vêm da migration).
func resetDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"TRUNCATE users, parties, company, river_job RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatal(err)
	}
}

// createAdmin faz o mesmo que o `make seed`: empresa + primeiro admin.
func createAdmin(t *testing.T, pool *pgxpool.Pool, email, password string) {
	t.Helper()
	ctx := context.Background()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(pool)
	if err := q.EnsureCompany(ctx, "Irmãos Faisão Frutícola"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateUser(ctx, db.CreateUserParams{
		Name: "Admin", Email: email, PasswordHash: hash, Role: auth.RoleAdmin,
	}); err != nil {
		t.Fatal(err)
	}
}

// tokenInLink pega o token do link de senha (rand.Text usa A-Z e 2-7).
var tokenInLink = regexp.MustCompile(`reset-password\?token=([A-Z2-7]+)(&invite=1)?`)

func TestIntegrationUsersAndRoles(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	createAdmin(t, env.pool, "admin@test.com", "admin-password")

	// --- Sem login: 401 ---------------------------------------------------------
	anon := newClient(t, env.srv.URL)
	anon.expect(http.StatusUnauthorized, "GET", "/users", nil, nil)
	anon.expect(http.StatusUnauthorized, "GET", "/auth/me", nil, nil)
	anon.expect(http.StatusUnauthorized, "POST", "/auth/login",
		map[string]any{"email": "admin@test.com", "password": "errada-123"}, nil)

	// --- Admin entra (e-mail em maiúsculas também vale) -------------------------
	admin := newClient(t, env.srv.URL)
	var adminMe me
	admin.expect(http.StatusOK, "POST", "/auth/login",
		map[string]any{"email": "ADMIN@test.com", "password": "admin-password"}, &adminMe)
	if adminMe.User.Role != "admin" || adminMe.Company == nil || adminMe.Company.Name != "Irmãos Faisão Frutícola" {
		t.Fatalf("me after login: %+v", adminMe)
	}

	// --- Empresa ------------------------------------------------------------------
	admin.expect(http.StatusOK, "GET", "/company", nil, nil)
	admin.expect(http.StatusUnprocessableEntity, "PUT", "/company",
		map[string]any{"name": "Irmãos Faisão", "document": "11.111.111/1111-11", "version": 1}, nil)
	admin.expect(http.StatusUnprocessableEntity, "PUT", "/company",
		map[string]any{"name": "Irmãos Faisão", "cityId": 9999999, "version": 1}, nil)
	var company struct {
		Version   int32   `json:"version"`
		TradeName *string `json:"tradeName"`
		City      *struct {
			Name string `json:"name"`
		} `json:"city"`
	}
	admin.expect(http.StatusOK, "PUT", "/company", map[string]any{
		"name": "Irmãos Faisão Frutícola Ltda", "tradeName": "Irmãos Faisão",
		"document": "11.222.333/0001-81", "taxRegime": "simples", "cityId": 3304557, // Rio de Janeiro
		"version": 1,
	}, &company)
	if company.City == nil || company.City.Name != "Rio de Janeiro" || company.TradeName == nil || company.Version != 2 {
		t.Fatalf("company: %+v", company)
	}
	// Outra tela ainda com a versão 1: 409 em vez de apagar o que foi salvo.
	admin.expect(http.StatusConflict, "PUT", "/company", map[string]any{"name": "Tela antiga", "version": 1}, nil)

	// --- Criar vendedor sem senha: vai convite por e-mail -----------------------
	var seller idResp
	admin.expect(http.StatusCreated, "POST", "/users",
		map[string]any{"name": "Vendedor", "email": "Vendedor@Test.com", "role": "seller"}, &seller)
	admin.expect(http.StatusConflict, "POST", "/users",
		map[string]any{"name": "Outro", "email": "vendedor@test.com", "role": "seller"}, nil)

	invite := env.mailer.waitFor(t, "vendedor@test.com", "Seu acesso")
	m := tokenInLink.FindStringSubmatch(invite.Text)
	if m == nil || m[2] == "" {
		t.Fatalf("no invite link in email: %s", invite.Text)
	}
	anon.expect(http.StatusNoContent, "POST", "/auth/password/reset",
		map[string]any{"token": m[1], "password": "senha-do-vendedor"}, nil)

	sellerClient := newClient(t, env.srv.URL)
	var sellerMe me
	sellerClient.expect(http.StatusOK, "POST", "/auth/login",
		map[string]any{"email": "vendedor@test.com", "password": "senha-do-vendedor"}, &sellerMe)
	if sellerMe.User.Role != "seller" {
		t.Fatalf("seller role = %q", sellerMe.User.Role)
	}

	// --- Papéis: vendedor não mexe em usuários nem na empresa ------------------
	sellerClient.expect(http.StatusForbidden, "GET", "/users", nil, nil)
	sellerClient.expect(http.StatusForbidden, "PUT", "/company", map[string]any{"name": "Hack", "version": 2}, nil)
	sellerClient.expect(http.StatusOK, "GET", "/company", nil, nil)
	sellerClient.expect(http.StatusOK, "GET", "/suppliers", nil, nil)

	// --- Proteções do admin ------------------------------------------------------
	admin.expect(http.StatusConflict, "PUT", fmt.Sprintf("/users/%d", adminMe.User.ID),
		map[string]any{"name": "Admin", "role": "admin", "active": false, "version": 1}, nil)
	admin.expect(http.StatusConflict, "PUT", fmt.Sprintf("/users/%d", adminMe.User.ID),
		map[string]any{"name": "Admin", "role": "manager", "active": true, "version": 1}, nil)
	admin.expect(http.StatusNotFound, "PUT", "/users/9999",
		map[string]any{"name": "Ninguém", "role": "seller", "active": true, "version": 1}, nil)

	// --- Desativar o vendedor derruba a sessão dele -----------------------------
	admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/users/%d", seller.ID),
		map[string]any{"name": "Vendedor", "role": "seller", "active": false, "version": 1}, nil)
	sellerClient.expect(http.StatusUnauthorized, "GET", "/auth/me", nil, nil)
	anon.expect(http.StatusUnauthorized, "POST", "/auth/login",
		map[string]any{"email": "vendedor@test.com", "password": "senha-do-vendedor"}, nil)
	admin.expect(http.StatusConflict, "POST", fmt.Sprintf("/users/%d/invite", seller.ID), nil, nil)

	var users struct {
		Items []struct {
			Email  string `json:"email"`
			Active bool   `json:"active"`
		} `json:"items"`
	}
	admin.expect(http.StatusOK, "GET", "/users", nil, &users)
	if len(users.Items) != 2 {
		t.Fatalf("users: %+v", users.Items)
	}

	// --- Logout -------------------------------------------------------------------
	admin.expect(http.StatusNoContent, "POST", "/auth/logout", nil, nil)
	admin.expect(http.StatusUnauthorized, "GET", "/auth/me", nil, nil)
}

func TestIntegrationSuppliers(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	createAdmin(t, env.pool, "admin@test.com", "admin-password")

	c := newClient(t, env.srv.URL)
	c.expect(http.StatusOK, "POST", "/auth/login",
		map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	body := map[string]any{
		"personType": "individual", "legalName": "Sítio A", "document": "52998224725",
		"address":  map[string]any{"cityId": 3305802}, // Teresópolis - RJ
		"contacts": []map[string]any{{"name": "Dono do Sítio A", "isWhatsapp": true}},
	}
	var created idResp
	c.expect(http.StatusCreated, "POST", "/suppliers", body, &created)
	// Documento repetido -> 409.
	c.expect(http.StatusConflict, "POST", "/suppliers", body, nil)

	// Fornecedor completo lido de volta, com cidade e contato.
	var full struct {
		Address struct {
			City *struct {
				Name string `json:"name"`
			} `json:"city"`
		} `json:"address"`
		Contacts []struct {
			Name string `json:"name"`
		} `json:"contacts"`
	}
	c.expect(http.StatusOK, "GET", fmt.Sprintf("/suppliers/%d", created.ID), nil, &full)
	if full.Address.City == nil || full.Address.City.Name != "Teresópolis" || len(full.Contacts) != 1 {
		t.Fatalf("supplier not stored completely: %+v", full)
	}

	// Município inexistente vira erro no campo (chave estrangeira do banco).
	c.expect(http.StatusUnprocessableEntity, "POST", "/suppliers", map[string]any{
		"personType": "company", "legalName": "Box X", "address": map[string]any{"cityId": 9999999},
	}, nil)

	// Busca de municípios: sem acento, e quem começa com o texto vem primeiro.
	var cities []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	c.expect(http.StatusOK, "GET", "/cities?search=teresop", nil, &cities)
	if len(cities) == 0 || cities[0].ID != 3305802 {
		t.Fatalf("city search: %+v", cities)
	}

	// Desativar e listar por status.
	c.expect(http.StatusNoContent, "DELETE", fmt.Sprintf("/suppliers/%d", created.ID), nil, nil)
	var list struct {
		Total int64 `json:"total"`
	}
	c.expect(http.StatusOK, "GET", "/suppliers?status=inactive", nil, &list)
	if list.Total != 1 {
		t.Errorf("inactive total = %d, want 1", list.Total)
	}
}

func TestIntegrationPasswordReset(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	ctx := context.Background()
	email := "carla@test.com"
	createAdmin(t, env.pool, email, "senha-antiga-1")

	carla := newClient(t, env.srv.URL)
	carla.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": email, "password": "senha-antiga-1"}, nil)

	// --- Esqueci a senha: e-mail desconhecido responde igual, sem enviar nada -
	anon := newClient(t, env.srv.URL)
	anon.expect(http.StatusAccepted, "POST", "/auth/password/forgot", map[string]any{"email": "ninguem@test.com"}, nil)
	anon.expect(http.StatusAccepted, "POST", "/auth/password/forgot", map[string]any{"email": strings.ToUpper(email)}, nil)
	reset := env.mailer.waitFor(t, email, "Redefinição de senha")
	m := tokenInLink.FindStringSubmatch(reset.Text)
	if m == nil || m[2] != "" {
		t.Fatalf("no reset link in email: %s", reset.Text)
	}
	token := m[1]
	if env.mailer.count("ninguem@test.com") != 0 {
		t.Error("email sent to unknown address")
	}

	// --- Redefinir ------------------------------------------------------------
	anon.expect(http.StatusBadRequest, "POST", "/auth/password/reset",
		map[string]any{"token": "TOKENINVALIDO234", "password": "senha-nova-123"}, nil)
	anon.expect(http.StatusNoContent, "POST", "/auth/password/reset",
		map[string]any{"token": token, "password": "senha-nova-123"}, nil)
	// Uso único.
	anon.expect(http.StatusBadRequest, "POST", "/auth/password/reset",
		map[string]any{"token": token, "password": "outra-senha-123"}, nil)

	// A sessão aberta antes da troca foi encerrada.
	carla.expect(http.StatusUnauthorized, "GET", "/auth/me", nil, nil)
	// Senha antiga não entra mais; a nova entra.
	anon.expect(http.StatusUnauthorized, "POST", "/auth/login", map[string]any{"email": email, "password": "senha-antiga-1"}, nil)
	anon.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": email, "password": "senha-nova-123"}, nil)

	// --- Enfileirar dentro de uma transação -----------------------------------
	err := pgx.BeginFunc(ctx, env.pool, func(tx pgx.Tx) error {
		return env.queue.EnqueueEmail(ctx, tx, mail.Message{To: email, Subject: "Teste na transação", Text: "x", HTML: "x"})
	})
	if err != nil {
		t.Fatalf("enqueue inside transaction: %v", err)
	}
	env.mailer.waitFor(t, email, "Teste na transação")

	// --- Rollback: o job some junto com a transação --------------------------
	_ = pgx.BeginFunc(ctx, env.pool, func(tx pgx.Tx) error {
		if err := env.queue.EnqueueEmail(ctx, tx, mail.Message{To: email, Subject: "Nunca deve chegar", Text: "x", HTML: "x"}); err != nil {
			return err
		}
		return fmt.Errorf("simulated failure") // força o ROLLBACK
	})
	time.Sleep(2 * time.Second)
	env.mailer.mu.Lock()
	for _, msg := range env.mailer.msgs {
		if msg.Subject == "Nunca deve chegar" {
			t.Error("job from rolled back transaction was executed")
		}
	}
	env.mailer.mu.Unlock()
}
