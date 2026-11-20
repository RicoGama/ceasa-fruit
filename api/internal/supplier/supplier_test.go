package supplier

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/money"
)

// fakeStore é um Store em memória. Como os handlers dependem da interface
// Store (e não do Postgres), estes testes rodam sem banco de dados.
// Ele imita o que importa do banco: documento único e município inexistente
// (chave estrangeira).
type fakeStore struct {
	parties   map[int64]db.Party
	suppliers map[int64]db.Supplier
	contacts  map[int64][]db.PartyContact
	nextID    int64
}

// Os únicos municípios que o banco falso conhece.
var fakeCities = map[int32][2]string{3304557: {"Rio de Janeiro", "RJ"}, 3305802: {"Teresópolis", "RJ"}}

func newFakeStore() *fakeStore {
	return &fakeStore{
		parties: map[int64]db.Party{}, suppliers: map[int64]db.Supplier{},
		contacts: map[int64][]db.PartyContact{}, nextID: 1,
	}
}

func (f *fakeStore) checkParty(id int64, document *string, cityID *int32) error {
	if cityID != nil {
		if _, ok := fakeCities[*cityID]; !ok {
			return &pgconn.PgError{Code: foreignKeyViolation, ConstraintName: "parties_city_id_fkey"}
		}
	}
	if document != nil {
		for _, p := range f.parties {
			if p.ID != id && p.Document != nil && *p.Document == *document {
				return &pgconn.PgError{Code: uniqueViolation}
			}
		}
	}
	return nil
}

func (f *fakeStore) CreateParty(_ context.Context, a db.CreatePartyParams) (int64, error) {
	if err := f.checkParty(0, a.Document, a.CityID); err != nil {
		return 0, err
	}
	id := f.nextID
	f.nextID++
	f.parties[id] = db.Party{
		ID: id, PersonType: a.PersonType, Document: a.Document,
		LegalName: a.LegalName, TradeName: a.TradeName, TaxpayerType: a.TaxpayerType,
		Phone: a.Phone, PostalCode: a.PostalCode, CityID: a.CityID,
		PixKeyType: a.PixKeyType, PixKey: a.PixKey, Version: 1,
	}
	return id, nil
}

// ClaimPartyVersion imita o UPDATE ... WHERE version = @version do banco.
func (f *fakeStore) ClaimPartyVersion(_ context.Context, a db.ClaimPartyVersionParams) (int64, error) {
	p, ok := f.parties[a.ID]
	if !ok || p.Version != a.Version {
		return 0, nil
	}
	p.Version++
	f.parties[a.ID] = p
	return 1, nil
}

func (f *fakeStore) BumpPartyVersion(_ context.Context, id int64) error {
	p := f.parties[id]
	p.Version++
	f.parties[id] = p
	return nil
}

func (f *fakeStore) UpdateParty(_ context.Context, a db.UpdatePartyParams) (int64, error) {
	p, ok := f.parties[a.ID]
	if !ok {
		return 0, nil
	}
	if err := f.checkParty(a.ID, a.Document, a.CityID); err != nil {
		return 0, err
	}
	p.LegalName, p.TradeName, p.Document, p.CityID = a.LegalName, a.TradeName, a.Document, a.CityID
	f.parties[a.ID] = p
	return 1, nil
}

func (f *fakeStore) CreateSupplier(_ context.Context, a db.CreateSupplierParams) error {
	now := time.Now()
	f.suppliers[a.PartyID] = db.Supplier{
		PartyID: a.PartyID, SupplierType: a.SupplierType,
		DefaultPurchaseMode: a.DefaultPurchaseMode, ConsignmentCommission: a.ConsignmentCommission,
		PaymentTermDays: a.PaymentTermDays, CeasaPavilion: a.CeasaPavilion, CeasaBox: a.CeasaBox,
		Status: a.Status, BlockedReason: a.BlockedReason, CreatedAt: now, UpdatedAt: now,
	}
	return nil
}

func (f *fakeStore) UpdateSupplier(_ context.Context, a db.UpdateSupplierParams) (int64, error) {
	s, ok := f.suppliers[a.PartyID]
	if !ok {
		return 0, nil
	}
	s.SupplierType, s.DefaultPurchaseMode, s.Status = a.SupplierType, a.DefaultPurchaseMode, a.Status
	s.ConsignmentCommission, s.BlockedReason = a.ConsignmentCommission, a.BlockedReason
	f.suppliers[a.PartyID] = s
	return 1, nil
}

func (f *fakeStore) GetSupplier(_ context.Context, id int64) (db.GetSupplierRow, error) {
	s, ok := f.suppliers[id]
	if !ok {
		return db.GetSupplierRow{}, pgx.ErrNoRows
	}
	p := f.parties[id]
	row := db.GetSupplierRow{Party: p, Supplier: s}
	if p.CityID != nil {
		c := fakeCities[*p.CityID]
		row.CityName, row.CityState = &c[0], &c[1]
	}
	return row, nil
}

func (f *fakeStore) ListSuppliers(_ context.Context, a db.ListSuppliersParams) ([]db.ListSuppliersRow, error) {
	var out []db.ListSuppliersRow
	for id, s := range f.suppliers {
		if a.Status != nil && s.Status != *a.Status {
			continue
		}
		p := f.parties[id]
		out = append(out, db.ListSuppliersRow{
			PartyID: id, PersonType: p.PersonType, Document: p.Document, LegalName: p.LegalName,
			TradeName: p.TradeName, SupplierType: s.SupplierType,
			DefaultPurchaseMode: s.DefaultPurchaseMode, Status: s.Status,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PartyID < out[j].PartyID })
	return out, nil
}

func (f *fakeStore) CountSuppliers(ctx context.Context, a db.CountSuppliersParams) (int64, error) {
	rows, _ := f.ListSuppliers(ctx, db.ListSuppliersParams{Status: a.Status})
	return int64(len(rows)), nil
}

func (f *fakeStore) DeactivateSupplier(_ context.Context, partyID int64) (int64, error) {
	s, ok := f.suppliers[partyID]
	if !ok {
		return 0, nil
	}
	s.Status, s.BlockedReason = "inactive", nil
	f.suppliers[partyID] = s
	return 1, nil
}

func (f *fakeStore) ListPartyContacts(_ context.Context, partyID int64) ([]db.PartyContact, error) {
	return f.contacts[partyID], nil
}

func (f *fakeStore) DeletePartyContacts(_ context.Context, partyID int64) error {
	delete(f.contacts, partyID)
	return nil
}

func (f *fakeStore) CreatePartyContact(_ context.Context, a db.CreatePartyContactParams) error {
	f.contacts[a.PartyID] = append(f.contacts[a.PartyID], db.PartyContact{
		ID: int64(len(f.contacts[a.PartyID]) + 1), PartyID: a.PartyID, Name: a.Name,
		Role: a.Role, Phone: a.Phone, IsWhatsapp: a.IsWhatsapp, Email: a.Email, Position: a.Position,
	})
	return nil
}

// testEnv guarda a API de teste e o banco falso.
type testEnv struct {
	api   humatest.TestAPI
	store *fakeStore
}

// newTestEnv sobe uma API Huma em memória só com as rotas de fornecedor.
// (O login é testado no pacote auth e no teste de integração.)
func newTestEnv(t *testing.T) *testEnv {
	_, api := humatest.New(t)
	env := &testEnv{api: api, store: newFakeStore()}

	// As rotas exigem permissão: aqui todo pedido entra como administrador.
	adminKey := auth.RoleKeyAdmin
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		s := &auth.Session{UserID: 1, RoleKey: &adminKey, RoleName: "Administrador"}
		next(huma.WithContext(ctx, auth.WithSession(ctx.Context(), s)))
	})

	// TxFunc falso: sem transação, só entrega o store em memória.
	tx := func(_ context.Context, fn func(Store) error) error { return fn(env.store) }
	Register(api, tx)
	return env
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	return v
}

func fullBody() map[string]any {
	return map[string]any{
		"personType":   "individual",
		"document":     "529.982.247-25",
		"legalName":    "José Antônio da Silva",
		"tradeName":    "Zé do Tomate",
		"phone":        "(21) 99999-0000",
		"address":      map[string]any{"postalCode": "25960-000", "street": "Estrada da Barragem", "number": "s/n", "cityId": 3305802},
		"payment":      map[string]any{"pixKeyType": "cpf", "pixKey": "529.982.247-25"},
		"contacts":     []map[string]any{{"name": "José", "role": "dono", "phone": "21999990000", "isWhatsapp": true}, {"name": "Maria", "role": "filha"}},
		"supplierType": "producer", "defaultPurchaseMode": "consignment", "consignmentCommission": "12.5",
		"paymentTermDays": 7, "ceasaPavilion": "21", "ceasaBox": "14",
	}
}

func TestCreateFullSupplier(t *testing.T) {
	env := newTestEnv(t)

	resp := env.api.Post("/suppliers", fullBody())
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}

	got := decode[Supplier](t, resp.Body.Bytes())
	if *got.Document != "52998224725" || *got.Phone != "21999990000" || *got.Address.PostalCode != "25960000" {
		t.Errorf("fields not normalized: doc=%v phone=%v cep=%v", *got.Document, *got.Phone, *got.Address.PostalCode)
	}
	if got.Address.City == nil || got.Address.City.Name != "Teresópolis" || got.Address.City.State != "RJ" {
		t.Errorf("city not resolved: %+v", got.Address.City)
	}
	if got.DefaultPurchaseMode != "consignment" || *got.ConsignmentCommission != money.MustParsePercent("12.5") {
		t.Errorf("purchase mode not saved: %s %v", got.DefaultPurchaseMode, got.ConsignmentCommission)
	}
	if len(got.Contacts) != 2 || got.Contacts[0].Name != "José" || !got.Contacts[0].IsWhatsapp {
		t.Errorf("contacts not saved in order: %+v", got.Contacts)
	}
	if got.Status != "active" {
		t.Errorf("status = %s, want active", got.Status)
	}
}

func TestCreateMinimalSupplierUsesDefaults(t *testing.T) {
	env := newTestEnv(t)

	// Cadastro mínimo: produtor sem documento, só nome e tipo de pessoa.
	resp := env.api.Post("/suppliers", map[string]any{"personType": "individual", "legalName": "Seu Joaquim"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	got := decode[Supplier](t, resp.Body.Bytes())
	if got.SupplierType != "producer" || got.DefaultPurchaseMode != "direct" || got.Status != "active" ||
		got.TaxpayerType != "non_taxpayer" || got.Document != nil || got.Contacts == nil {
		t.Errorf("defaults not applied: %+v", got)
	}
}

func TestCreateSupplierValidation(t *testing.T) {
	cases := []struct {
		name   string
		modify func(b map[string]any)
		want   string // trecho esperado na resposta de erro
	}{
		{"consignment without commission", func(b map[string]any) { delete(b, "consignmentCommission") }, "body.consignmentCommission"},
		{"commission over 100", func(b map[string]any) { b["consignmentCommission"] = "150" }, "body.consignmentCommission"},
		{"commission as JSON number", func(b map[string]any) { b["consignmentCommission"] = 12.5 }, "body.consignmentCommission"},
		{"commission with comma", func(b map[string]any) { b["consignmentCommission"] = "12,5" }, "body.consignmentCommission"},
		{"blocked without reason", func(b map[string]any) { b["status"] = "blocked" }, "body.blockedReason"},
		{"document does not match person type", func(b map[string]any) { b["personType"] = "company" }, "a company must have a CNPJ"},
		{"invalid postal code", func(b map[string]any) { b["address"].(map[string]any)["postalCode"] = "123" }, "body.address.postalCode"},
		{"unknown supplier type", func(b map[string]any) { b["supplierType"] = "alien" }, "body.supplierType"},
		{"payment term over a year", func(b map[string]any) { b["paymentTermDays"] = 400 }, "body.paymentTermDays"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newTestEnv(t)
			body := fullBody()
			c.modify(body)
			resp := env.api.Post("/suppliers", body)
			if resp.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body = %s", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), c.want) {
				t.Errorf("body %s does not mention %q", resp.Body.String(), c.want)
			}
		})
	}
}

func TestUnknownCityIsAFieldError(t *testing.T) {
	env := newTestEnv(t)
	body := fullBody()
	body["address"].(map[string]any)["cityId"] = 9999999

	resp := env.api.Post("/suppliers", body)
	if resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "body.address.cityId") {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestDuplicateDocument(t *testing.T) {
	env := newTestEnv(t)

	env.api.Post("/suppliers", fullBody())
	if resp := env.api.Post("/suppliers", fullBody()); resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.Code)
	}
}

func TestUpdateReplacesContacts(t *testing.T) {
	env := newTestEnv(t)
	env.api.Post("/suppliers", fullBody())

	body := fullBody()
	body["tradeName"] = "Zé Tomateiro"
	body["contacts"] = []map[string]any{{"name": "Carlos", "role": "motorista"}}
	body["version"] = 1
	resp := env.api.Put("/suppliers/1", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	got := decode[Supplier](t, resp.Body.Bytes())
	if *got.TradeName != "Zé Tomateiro" || len(got.Contacts) != 1 || got.Contacts[0].Name != "Carlos" {
		t.Errorf("update not applied: trade=%v contacts=%+v", *got.TradeName, got.Contacts)
	}
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
}

// Duas pessoas abrem o cadastro na versão 1. A primeira salva (vira 2); a
// segunda, ainda com a versão 1, recebe 409 em vez de apagar a alteração.
func TestStaleUpdateIsRejected(t *testing.T) {
	env := newTestEnv(t)
	env.api.Post("/suppliers", fullBody())

	first := fullBody()
	first["tradeName"] = "Primeira pessoa"
	first["version"] = 1
	if resp := env.api.Put("/suppliers/1", first); resp.Code != http.StatusOK {
		t.Fatalf("first save: %d %s", resp.Code, resp.Body.String())
	}

	second := fullBody()
	second["tradeName"] = "Segunda pessoa"
	second["version"] = 1
	resp := env.api.Put("/suppliers/1", second)
	if resp.Code != http.StatusConflict {
		t.Fatalf("stale save: status = %d, want 409", resp.Code)
	}
	if got := decode[Supplier](t, env.api.Get("/suppliers/1").Body.Bytes()); *got.TradeName != "Primeira pessoa" {
		t.Errorf("first change was lost: %s", *got.TradeName)
	}

	// Inativar também muda a versão: a tela aberta não reativa sem querer.
	env.api.Delete("/suppliers/1")
	third := fullBody()
	third["version"] = 2
	if resp := env.api.Put("/suppliers/1", third); resp.Code != http.StatusConflict {
		t.Errorf("save after deactivate: status = %d, want 409", resp.Code)
	}
}

func withVersion(b map[string]any, v int) map[string]any {
	b["version"] = v
	return b
}

func TestUnknownSupplierIs404(t *testing.T) {
	env := newTestEnv(t)
	if resp := env.api.Get("/suppliers/99"); resp.Code != http.StatusNotFound {
		t.Errorf("get: status = %d, want 404", resp.Code)
	}
	if resp := env.api.Put("/suppliers/99", withVersion(fullBody(), 1)); resp.Code != http.StatusNotFound {
		t.Errorf("update: status = %d, want 404", resp.Code)
	}
	if resp := env.api.Delete("/suppliers/99"); resp.Code != http.StatusNotFound {
		t.Errorf("delete: status = %d, want 404", resp.Code)
	}
}

func TestDeactivateAndListByStatus(t *testing.T) {
	env := newTestEnv(t)
	env.api.Post("/suppliers", fullBody())

	if resp := env.api.Delete("/suppliers/1"); resp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.Code)
	}

	// O filtro padrão de status é "active", então a lista agora deve vir vazia ([] e não null).
	if resp := env.api.Get("/suppliers"); !strings.Contains(resp.Body.String(), `"items":[]`) {
		t.Errorf("expected empty items, got %s", resp.Body.String())
	}
	if resp := env.api.Get("/suppliers?status=inactive"); !strings.Contains(resp.Body.String(), `"total":1`) {
		t.Errorf("expected 1 inactive, got %s", resp.Body.String())
	}
}
