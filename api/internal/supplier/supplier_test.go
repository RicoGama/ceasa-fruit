package supplier

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

// fakeStore é um Store em memória. Como os handlers dependem da interface
// Store (e não do Postgres), estes testes rodam sem banco de dados.
// Ele imita o filtro por organization_id das queries reais.
type fakeStore struct {
	rows   map[int64]db.Supplier
	nextID int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[int64]db.Supplier{}, nextID: 1}
}

func (f *fakeStore) CreateSupplier(_ context.Context, a db.CreateSupplierParams) (db.Supplier, error) {
	for _, r := range f.rows {
		if r.OrganizationID == a.OrganizationID && r.Document == a.Document {
			return db.Supplier{}, &pgconn.PgError{Code: uniqueViolation}
		}
	}
	now := time.Now()
	s := db.Supplier{
		ID: f.nextID, OrganizationID: a.OrganizationID, Name: a.Name, Document: a.Document,
		Phone: a.Phone, Email: a.Email, City: a.City, State: a.State, Notes: a.Notes,
		Active: true, CreatedAt: now, UpdatedAt: now,
	}
	f.rows[s.ID] = s
	f.nextID++
	return s, nil
}

func (f *fakeStore) GetSupplier(_ context.Context, a db.GetSupplierParams) (db.Supplier, error) {
	s, ok := f.rows[a.ID]
	if !ok || s.OrganizationID != a.OrganizationID {
		return db.Supplier{}, pgx.ErrNoRows
	}
	return s, nil
}

func (f *fakeStore) ListSuppliers(_ context.Context, a db.ListSuppliersParams) ([]db.Supplier, error) {
	var out []db.Supplier
	for _, r := range f.rows {
		if r.OrganizationID == a.OrganizationID && (a.Active == nil || r.Active == *a.Active) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) CountSuppliers(ctx context.Context, a db.CountSuppliersParams) (int64, error) {
	rows, _ := f.ListSuppliers(ctx, db.ListSuppliersParams{OrganizationID: a.OrganizationID, Active: a.Active})
	return int64(len(rows)), nil
}

func (f *fakeStore) UpdateSupplier(_ context.Context, a db.UpdateSupplierParams) (db.Supplier, error) {
	s, ok := f.rows[a.ID]
	if !ok || s.OrganizationID != a.OrganizationID {
		return db.Supplier{}, pgx.ErrNoRows
	}
	s.Name, s.Document, s.State, s.Active = a.Name, a.Document, a.State, a.Active
	f.rows[a.ID] = s
	return s, nil
}

func (f *fakeStore) DeactivateSupplier(_ context.Context, a db.DeactivateSupplierParams) (int64, error) {
	s, ok := f.rows[a.ID]
	if !ok || s.OrganizationID != a.OrganizationID {
		return 0, nil
	}
	s.Active = false
	f.rows[a.ID] = s
	return 1, nil
}

// testEnv guarda a API de teste e permite trocar a organização "logada".
type testEnv struct {
	api   humatest.TestAPI
	store *fakeStore
	orgID int64
}

// newTestEnv sobe uma API Huma em memória só com as rotas de fornecedor.
// Um middleware coloca no context uma sessão falsa da organização env.orgID,
// fazendo o papel do login.
func newTestEnv(t *testing.T) *testEnv {
	_, api := humatest.New(t)
	env := &testEnv{api: api, store: newFakeStore(), orgID: 1}

	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		orgID := env.orgID
		s := &auth.Session{UserID: 1, OrganizationID: &orgID, OrganizationStatus: "active"}
		next(huma.WithContext(ctx, auth.WithSession(ctx.Context(), s)))
	})

	// TxFunc falso: sem transação, só entrega o store em memória.
	tx := func(_ context.Context, _ int64, fn func(Store) error) error { return fn(env.store) }
	Register(api, tx)
	return env
}

func TestCreateSupplier(t *testing.T) {
	env := newTestEnv(t)

	resp := env.api.Post("/suppliers", map[string]any{
		"name":     "  Sítio Boa Colheita ",
		"document": "11.222.333/0001-81",
		"state":    "sp",
		"phone":    "",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}

	var got Supplier
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Sítio Boa Colheita" {
		t.Errorf("name not trimmed: %q", got.Name)
	}
	if got.Document != "11222333000181" {
		t.Errorf("document not normalized: %q", got.Document)
	}
	if got.State == nil || *got.State != "SP" {
		t.Errorf("state not uppercased: %v", got.State)
	}
	if got.Phone != nil {
		t.Errorf("empty phone should be stored as null, got %q", *got.Phone)
	}
	if env.store.rows[got.ID].OrganizationID != 1 {
		t.Errorf("supplier not bound to the session organization")
	}
}

func TestCreateSupplierValidation(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want string // trecho esperado na resposta de erro
	}{
		{"invalid cnpj", map[string]any{"name": "Abc", "document": "11.222.333/0001-80"}, "invalid CPF or CNPJ"},
		{"name too short", map[string]any{"name": "A", "document": "52998224725"}, "body.name"},
		{"invalid state", map[string]any{"name": "Abc", "document": "52998224725", "state": "SPX"}, "body.state"},
		{"missing document", map[string]any{"name": "Abc"}, "document"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newTestEnv(t)
			resp := env.api.Post("/suppliers", c.body)
			if resp.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body = %s", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), c.want) {
				t.Errorf("body %s does not mention %q", resp.Body.String(), c.want)
			}
		})
	}
}

func TestDuplicateDocumentIsPerOrganization(t *testing.T) {
	env := newTestEnv(t)
	body := map[string]any{"name": "Abc", "document": "52998224725"}

	env.api.Post("/suppliers", body)
	if resp := env.api.Post("/suppliers", body); resp.Code != http.StatusConflict {
		t.Fatalf("same org: status = %d, want 409", resp.Code)
	}

	env.orgID = 2 // outra organização pode cadastrar o mesmo produtor
	if resp := env.api.Post("/suppliers", body); resp.Code != http.StatusCreated {
		t.Fatalf("other org: status = %d, want 201", resp.Code)
	}
}

func TestOrganizationsAreIsolated(t *testing.T) {
	env := newTestEnv(t)
	env.api.Post("/suppliers", map[string]any{"name": "Abc", "document": "52998224725"})

	env.orgID = 2
	if resp := env.api.Get("/suppliers/1"); resp.Code != http.StatusNotFound {
		t.Errorf("get from other org: status = %d, want 404", resp.Code)
	}
	if resp := env.api.Delete("/suppliers/1"); resp.Code != http.StatusNotFound {
		t.Errorf("delete from other org: status = %d, want 404", resp.Code)
	}
	if resp := env.api.Get("/suppliers"); !strings.Contains(resp.Body.String(), `"total":0`) {
		t.Errorf("list from other org should be empty, got %s", resp.Body.String())
	}
}

func TestGetSupplierNotFound(t *testing.T) {
	env := newTestEnv(t)
	if resp := env.api.Get("/suppliers/99"); resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

func TestDeactivateAndList(t *testing.T) {
	env := newTestEnv(t)
	env.api.Post("/suppliers", map[string]any{"name": "Abc", "document": "52998224725"})

	if resp := env.api.Delete("/suppliers/1"); resp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.Code)
	}

	// O filtro padrão de status é "active", então a lista agora deve vir vazia ([] e não null).
	resp := env.api.Get("/suppliers")
	if !strings.Contains(resp.Body.String(), `"items":[]`) {
		t.Errorf("expected empty items, got %s", resp.Body.String())
	}

	resp = env.api.Get("/suppliers?status=all")
	if !strings.Contains(resp.Body.String(), `"total":1`) {
		t.Errorf("expected total 1, got %s", resp.Body.String())
	}
}
