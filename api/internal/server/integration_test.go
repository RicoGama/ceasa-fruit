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
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// Teste de integração: sobe a API inteira contra um PostgreSQL de verdade.
// Só roda com TEST_DATABASE_URL definida (ver `make test-integration`);
// sem ela é pulado, então `go test ./...` continua funcionando sem banco.

func setupIntegration(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(Deps{Logger: logger, Pool: pool}))
	t.Cleanup(srv.Close)
	return srv, pool
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
		ID int64 `json:"id"`
	} `json:"user"`
	Organizations []struct {
		ID int64 `json:"id"`
	} `json:"organizations"`
	CurrentOrganizationID *int64 `json:"currentOrganizationId"`
}

type idResp struct {
	ID int64 `json:"id"`
}

type listResp struct {
	Total int64 `json:"total"`
}

func TestIntegrationMultiTenancy(t *testing.T) {
	srv, pool := setupIntegration(t)
	ctx := context.Background()
	q := db.New(pool)

	// E-mails únicos por execução, para o teste poder rodar várias vezes no mesmo banco.
	suffix := time.Now().UnixNano()
	aliceEmail := fmt.Sprintf("alice-%d@test.com", suffix)
	bobEmail := fmt.Sprintf("bob-%d@test.com", suffix)

	// --- Sem login: rotas de negócio respondem 401 ----------------------------
	anon := newClient(t, srv.URL)
	anon.expect(http.StatusUnauthorized, "GET", "/suppliers", nil, nil)
	anon.expect(http.StatusUnauthorized, "GET", "/auth/me", nil, nil)

	// --- Alice cria conta + organização A e cadastra um fornecedor ------------
	alice := newClient(t, srv.URL)
	var aliceMe me
	alice.expect(http.StatusCreated, "POST", "/auth/signup", map[string]any{
		"name": "Alice", "email": aliceEmail, "password": "secret-password",
		"organizationName": "Distribuidora A",
	}, &aliceMe)
	orgA := *aliceMe.CurrentOrganizationID

	var supplierA idResp
	alice.expect(http.StatusCreated, "POST", "/suppliers", map[string]any{
		"personType": "individual", "legalName": "Sítio A", "document": "52998224725",
		"address":  map[string]any{"cityId": 3305802}, // Teresópolis - RJ
		"contacts": []map[string]any{{"name": "Dono do Sítio A", "isWhatsapp": true}},
	}, &supplierA)

	// Fornecedor completo lido de volta, com cidade e contato.
	var fullA struct {
		Address struct {
			City *struct {
				Name  string `json:"name"`
				State string `json:"state"`
			} `json:"city"`
		} `json:"address"`
		Contacts []struct {
			Name string `json:"name"`
		} `json:"contacts"`
	}
	alice.expect(http.StatusOK, "GET", fmt.Sprintf("/suppliers/%d", supplierA.ID), nil, &fullA)
	if fullA.Address.City == nil || fullA.Address.City.Name != "Teresópolis" || len(fullA.Contacts) != 1 {
		t.Fatalf("supplier not stored completely: %+v", fullA)
	}

	// Município inexistente vira erro no campo (chave estrangeira do banco).
	alice.expect(http.StatusUnprocessableEntity, "POST", "/suppliers", map[string]any{
		"personType": "company", "legalName": "Box X", "address": map[string]any{"cityId": 9999999},
	}, nil)

	// Busca de municípios: sem acento, e quem começa com o texto vem primeiro.
	var cities []struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
	}
	alice.expect(http.StatusOK, "GET", "/cities?search=teresop", nil, &cities)
	if len(cities) == 0 || cities[0].Name != "Teresópolis" || cities[0].ID != 3305802 {
		t.Fatalf("city search: %+v", cities)
	}
	anon.expect(http.StatusUnauthorized, "GET", "/cities?search=rio", nil, nil)

	// E-mail repetido no cadastro -> 409.
	newClient(t, srv.URL).expect(http.StatusConflict, "POST", "/auth/signup", map[string]any{
		"name": "Alice 2", "email": aliceEmail, "password": "secret-password",
		"organizationName": "Outra",
	}, nil)

	// --- Bob cria a organização B: não enxerga nada da A ----------------------
	bob := newClient(t, srv.URL)
	var bobMe me
	bob.expect(http.StatusCreated, "POST", "/auth/signup", map[string]any{
		"name": "Bob", "email": bobEmail, "password": "secret-password",
		"organizationName": "Distribuidora B",
	}, &bobMe)
	orgB := *bobMe.CurrentOrganizationID

	var list listResp
	bob.expect(http.StatusOK, "GET", "/suppliers", nil, &list)
	if list.Total != 0 {
		t.Fatalf("org B sees %d suppliers from org A", list.Total)
	}
	bob.expect(http.StatusNotFound, "GET", fmt.Sprintf("/suppliers/%d", supplierA.ID), nil, nil)
	// Mesmo documento em outra organização é permitido.
	bob.expect(http.StatusCreated, "POST", "/suppliers", map[string]any{
		"personType": "individual", "legalName": "Sítio A (cadastro da B)", "document": "52998224725",
	}, nil)

	// --- RLS: mesmo SQL cru, SEM filtro, só enxerga a própria organização ---
	t.Run("rls hides other organizations", func(t *testing.T) {
		var visible, foreign int64
		err := postgres.InTenantTx(ctx, pool, orgB, func(tx pgx.Tx) error {
			// parties: a tabela com os dados pessoais, a mais sensível.
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM parties").Scan(&visible); err != nil {
				return err
			}
			return tx.QueryRow(ctx,
				"SELECT count(*) FROM party_contacts WHERE organization_id = $1", orgA).Scan(&foreign)
		})
		if err != nil {
			t.Fatal(err)
		}
		if visible != 1 || foreign != 0 {
			t.Errorf("org B tx sees %d rows total and %d rows of org A; want 1 and 0", visible, foreign)
		}
	})
	t.Run("rls blocks writing into another organization", func(t *testing.T) {
		err := postgres.InTenantTx(ctx, pool, orgB, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				"INSERT INTO parties (organization_id, person_type, legal_name) VALUES ($1, 'company', 'x')", orgA)
			return err
		})
		if err == nil {
			t.Error("insert into org A from org B transaction should fail")
		}
	})

	// --- Bob passa a participar da organização A e troca de organização -----
	if err := q.CreateMembership(ctx, db.CreateMembershipParams{
		OrganizationID: orgA, UserID: bobMe.User.ID, Role: "member",
	}); err != nil {
		t.Fatal(err)
	}
	bob.expect(http.StatusOK, "PUT", "/auth/organization", map[string]any{"organizationId": orgA}, nil)
	bob.expect(http.StatusOK, "GET", fmt.Sprintf("/suppliers/%d", supplierA.ID), nil, nil)

	// Alice não participa da B.
	alice.expect(http.StatusForbidden, "PUT", "/auth/organization", map[string]any{"organizationId": orgB}, nil)

	// Login novo de Bob: com 2 organizações, nenhuma é selecionada automaticamente.
	bob2 := newClient(t, srv.URL)
	var bob2Me me
	bob2.expect(http.StatusOK, "POST", "/auth/login",
		map[string]any{"email": bobEmail, "password": "secret-password"}, &bob2Me)
	if bob2Me.CurrentOrganizationID != nil || len(bob2Me.Organizations) != 2 {
		t.Errorf("login with 2 orgs: current=%v orgs=%d", bob2Me.CurrentOrganizationID, len(bob2Me.Organizations))
	}
	bob2.expect(http.StatusForbidden, "GET", "/suppliers", nil, nil) // precisa escolher antes

	// Senha errada e e-mail inexistente dão a mesma resposta.
	anon.expect(http.StatusUnauthorized, "POST", "/auth/login",
		map[string]any{"email": bobEmail, "password": "wrong-password"}, nil)
	anon.expect(http.StatusUnauthorized, "POST", "/auth/login",
		map[string]any{"email": "nobody@test.com", "password": "wrong-password"}, nil)

	// --- Admin da plataforma -------------------------------------------------
	alice.expect(http.StatusForbidden, "GET", "/admin/organizations", nil, nil)
	if _, err := q.PromotePlatformAdmin(ctx, aliceEmail); err != nil {
		t.Fatal(err)
	}
	alice.expect(http.StatusOK, "GET", "/admin/organizations", nil, nil)

	// Suspender a organização B bloqueia quem está nela.
	bob.expect(http.StatusOK, "PUT", "/auth/organization", map[string]any{"organizationId": orgB}, nil)
	alice.expect(http.StatusOK, "PUT", fmt.Sprintf("/admin/organizations/%d/status", orgB),
		map[string]any{"status": "suspended"}, nil)
	bob.expect(http.StatusForbidden, "GET", "/suppliers", nil, nil)
	bob.expect(http.StatusForbidden, "PUT", "/auth/organization", map[string]any{"organizationId": orgB}, nil)

	// --- Logout --------------------------------------------------------------
	alice.expect(http.StatusNoContent, "POST", "/auth/logout", nil, nil)
	alice.expect(http.StatusUnauthorized, "GET", "/auth/me", nil, nil)
}
