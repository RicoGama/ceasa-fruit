package supplier

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

// fakeStore is an in-memory Store. Because the handlers depend on the Store
// interface (not on Postgres), these tests run without a database.
type fakeStore struct {
	rows   map[int64]db.Supplier
	nextID int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[int64]db.Supplier{}, nextID: 1}
}

func (f *fakeStore) CreateSupplier(_ context.Context, a db.CreateSupplierParams) (db.Supplier, error) {
	for _, r := range f.rows {
		if r.Document == a.Document {
			return db.Supplier{}, &pgconn.PgError{Code: uniqueViolation}
		}
	}
	now := time.Now()
	s := db.Supplier{
		ID: f.nextID, Name: a.Name, Document: a.Document, Phone: a.Phone, Email: a.Email,
		City: a.City, State: a.State, Notes: a.Notes, Active: true, CreatedAt: now, UpdatedAt: now,
	}
	f.rows[s.ID] = s
	f.nextID++
	return s, nil
}

func (f *fakeStore) GetSupplier(_ context.Context, id int64) (db.Supplier, error) {
	s, ok := f.rows[id]
	if !ok {
		return db.Supplier{}, pgx.ErrNoRows
	}
	return s, nil
}

func (f *fakeStore) ListSuppliers(_ context.Context, a db.ListSuppliersParams) ([]db.Supplier, error) {
	var out []db.Supplier
	for _, r := range f.rows {
		if a.Active == nil || r.Active == *a.Active {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) CountSuppliers(ctx context.Context, a db.CountSuppliersParams) (int64, error) {
	rows, _ := f.ListSuppliers(ctx, db.ListSuppliersParams{Active: a.Active})
	return int64(len(rows)), nil
}

func (f *fakeStore) UpdateSupplier(_ context.Context, a db.UpdateSupplierParams) (db.Supplier, error) {
	s, ok := f.rows[a.ID]
	if !ok {
		return db.Supplier{}, pgx.ErrNoRows
	}
	s.Name, s.Document, s.State, s.Active = a.Name, a.Document, a.State, a.Active
	f.rows[a.ID] = s
	return s, nil
}

func (f *fakeStore) DeactivateSupplier(_ context.Context, id int64) (int64, error) {
	s, ok := f.rows[id]
	if !ok {
		return 0, nil
	}
	s.Active = false
	f.rows[id] = s
	return 1, nil
}

// newTestAPI spins up an in-memory Huma API with only the supplier routes.
func newTestAPI(t *testing.T) (humatest.TestAPI, *fakeStore) {
	_, api := humatest.New(t)
	store := newFakeStore()
	Register(api, store)
	return api, store
}

func TestCreateSupplier(t *testing.T) {
	api, store := newTestAPI(t)

	resp := api.Post("/suppliers", map[string]any{
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
	if len(store.rows) != 1 {
		t.Errorf("expected 1 row in store, got %d", len(store.rows))
	}
}

func TestCreateSupplierValidation(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want string // fragment expected in the error response
	}{
		{"invalid cnpj", map[string]any{"name": "Abc", "document": "11.222.333/0001-80"}, "invalid CPF or CNPJ"},
		{"name too short", map[string]any{"name": "A", "document": "52998224725"}, "body.name"},
		{"invalid state", map[string]any{"name": "Abc", "document": "52998224725", "state": "SPX"}, "body.state"},
		{"missing document", map[string]any{"name": "Abc"}, "document"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api, _ := newTestAPI(t)
			resp := api.Post("/suppliers", c.body)
			if resp.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body = %s", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), c.want) {
				t.Errorf("body %s does not mention %q", resp.Body.String(), c.want)
			}
		})
	}
}

func TestCreateSupplierDuplicateDocument(t *testing.T) {
	api, _ := newTestAPI(t)
	body := map[string]any{"name": "Abc", "document": "52998224725"}

	api.Post("/suppliers", body)
	resp := api.Post("/suppliers", body)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.Code)
	}
}

func TestGetSupplierNotFound(t *testing.T) {
	api, _ := newTestAPI(t)
	if resp := api.Get("/suppliers/99"); resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

func TestDeactivateAndList(t *testing.T) {
	api, _ := newTestAPI(t)
	api.Post("/suppliers", map[string]any{"name": "Abc", "document": "52998224725"})

	if resp := api.Delete("/suppliers/1"); resp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.Code)
	}

	// Default status filter is "active", so the list must now be empty ([] not null).
	resp := api.Get("/suppliers")
	if !strings.Contains(resp.Body.String(), `"items":[]`) {
		t.Errorf("expected empty items, got %s", resp.Body.String())
	}

	resp = api.Get("/suppliers?status=all")
	if !strings.Contains(resp.Body.String(), `"total":1`) {
		t.Errorf("expected total 1, got %s", resp.Body.String())
	}
}
