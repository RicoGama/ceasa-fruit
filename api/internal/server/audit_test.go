package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
)

type historyResp struct {
	Events []struct {
		ID         int64  `json:"id"`
		EntityName string `json:"entityName"`
		Actor      *struct {
			Name string `json:"name"`
		} `json:"actor"`
		Changes []struct {
			Table  string `json:"table"`
			Action string `json:"action"`
			Fields []struct {
				Name string          `json:"name"`
				Old  json.RawMessage `json:"old"`
				New  json.RawMessage `json:"new"`
			} `json:"fields"`
		} `json:"changes"`
	} `json:"events"`
	Refs       map[string]string `json:"refs"`
	NextBefore *int64            `json:"nextBefore"`
}

// summary resume um evento como "tabela:ação[campo=antigo>novo ...]" para comparar.
func (h historyResp) summary(i int) []string {
	var out []string
	for _, c := range h.Events[i].Changes {
		s := c.Table + ":" + c.Action
		if c.Action == "updated" {
			for _, f := range c.Fields {
				s += fmt.Sprintf(" %s=%s>%s", f.Name, f.Old, f.New)
			}
		}
		out = append(out, s)
	}
	return out
}

func TestIntegrationAudit(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	ctx := context.Background()
	if _, err := env.pool.Exec(ctx, "TRUNCATE audit_log, products, drivers, vehicles RESTART IDENTITY CASCADE; DELETE FROM roles WHERE key IS NULL"); err != nil {
		t.Fatal(err)
	}
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	// --- Fornecedor: criar e alterar ------------------------------------------------
	body := map[string]any{
		"personType": "company", "legalName": "Sítio do Zé", "phone": "21999990000",
		"contacts": []map[string]any{{"name": "Zé", "phone": "21911112222"}, {"name": "Maria"}},
	}
	var sup struct {
		ID      int64 `json:"id"`
		Version int32 `json:"version"`
	}
	admin.expect(http.StatusCreated, "POST", "/suppliers", body, &sup)
	path := fmt.Sprintf("/audit/party/%d", sup.ID)

	var h historyResp
	admin.expect(http.StatusOK, "GET", path, nil, &h)
	if len(h.Events) != 1 || h.Events[0].Actor == nil || h.Events[0].Actor.Name != "Admin" || h.Events[0].EntityName != "Sítio do Zé" {
		t.Fatalf("create event: %+v", h.Events)
	}
	if got := h.summary(0); !slices.Equal(got, []string{"parties:created", "party_contacts:created", "party_contacts:created", "suppliers:created"}) {
		t.Fatalf("create changes: %v", got)
	}

	// Salvar sem mudar nada não gera histórico.
	body["version"] = sup.Version
	admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/suppliers/%d", sup.ID), body, &sup)
	admin.expect(http.StatusOK, "GET", path, nil, &h)
	if len(h.Events) != 1 {
		t.Fatalf("save without changes created history: %d events", len(h.Events))
	}

	// Troca o telefone da pessoa e o do Zé, tira a Maria, põe o João:
	// só a diferença entra (nada de "apagou e criou todos os contatos").
	body["phone"] = "21988887777"
	body["contacts"] = []map[string]any{{"name": "Zé", "phone": "21933334444"}, {"name": "João"}}
	body["version"] = sup.Version
	admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/suppliers/%d", sup.ID), body, &sup)
	admin.expect(http.StatusOK, "GET", path, nil, &h)
	want := []string{
		`parties:updated phone="21999990000">"21988887777"`,
		`party_contacts:updated phone="21911112222">"21933334444"`,
		`party_contacts:updated name="Maria">"João"`,
	}
	if got := h.summary(0); len(h.Events) != 2 || !slices.Equal(got, want) {
		t.Fatalf("update changes:\n got %v\nwant %v", got, want)
	}

	// --- Fornecedor × produto: o nome do produto vem em refs ----------------------
	var tomato idResp
	admin.expect(http.StatusCreated, "POST", "/products", map[string]any{"name": "Tomate", "variety": "Italiano", "category": "vegetable", "stockUnit": "box"}, &tomato)
	admin.expect(http.StatusCreated, "POST", fmt.Sprintf("/suppliers/%d/products", sup.ID), map[string]any{"productId": tomato.ID}, nil)
	admin.expect(http.StatusOK, "GET", path, nil, &h)
	if got := h.summary(0); !slices.Equal(got, []string{"supplier_products:created"}) || h.Refs[fmt.Sprintf("product:%d", tomato.ID)] != "Tomate Italiano" {
		t.Fatalf("supplier product: %v refs %v", got, h.Refs)
	}

	// --- Papel: só as permissões que entraram e saíram -----------------------------
	var role struct {
		ID      int64 `json:"id"`
		Version int32 `json:"version"`
	}
	admin.expect(http.StatusCreated, "POST", "/roles", map[string]any{"name": "Balcão", "permissions": []string{"suppliers.view", "products.view"}}, &role)
	admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/roles/%d", role.ID), map[string]any{
		"name": "Balcão", "permissions": []string{"products.view", "customers.view"}, "version": role.Version,
	}, nil)
	admin.expect(http.StatusOK, "GET", fmt.Sprintf("/audit/role/%d", role.ID), nil, &h)
	if got := h.summary(0); !slices.Equal(got, []string{"role_permissions:deleted", "role_permissions:created"}) {
		t.Fatalf("role permission changes: %v", got)
	}

	// --- Usuário: a senha nunca entra no histórico ---------------------------------
	admin.expect(http.StatusCreated, "POST", "/users", map[string]any{
		"name": "Vendedor", "email": "vend@test.com", "roleId": roleID(t, env.pool, "seller"), "password": "senha-vend-123",
	}, nil)
	var leaked int
	_ = env.pool.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE changes::text ILIKE '%password%' OR changes::text LIKE '%$argon%' OR changes::text LIKE '%$2a$%'").Scan(&leaked)
	if leaked != 0 {
		t.Fatalf("password found in history (%d rows)", leaked)
	}

	// --- Frota: a sincronização (última posição) não é alteração de cadastro ------
	if _, err := env.pool.Exec(ctx, "INSERT INTO vehicles (plate) VALUES ('KXT1A23')"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, "UPDATE vehicles SET last_fix_at = now(), last_latitude = -22.8, last_longitude = -43.3, traccar_last_update = now()"); err != nil {
		t.Fatal(err)
	}
	var vehicleRows int
	var actorless bool
	_ = env.pool.QueryRow(ctx, "SELECT count(*), bool_and(actor_user_id IS NULL) FROM audit_log WHERE entity_type = 'vehicle'").Scan(&vehicleRows, &actorless)
	if vehicleRows != 1 || !actorless {
		t.Fatalf("vehicle history: %d rows (want only the insert), system actor=%v", vehicleRows, actorless)
	}

	// --- Quem vê o quê ---------------------------------------------------------------
	seller := newClient(t, env.srv.URL)
	seller.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "vend@test.com", "password": "senha-vend-123"}, nil)
	seller.expect(http.StatusOK, "GET", path, nil, &h)                                             // vê fornecedores
	seller.expect(http.StatusForbidden, "GET", fmt.Sprintf("/audit/role/%d", role.ID), nil, nil) // não gerencia papéis
	seller.expect(http.StatusForbidden, "GET", "/audit", nil, nil)                               // geral: só administrador

	var all historyResp
	admin.expect(http.StatusOK, "GET", "/audit?days=1&entityType=party", nil, &all)
	for _, e := range all.Events {
		if e.EntityName != "Sítio do Zé" {
			t.Fatalf("party filter returned %q", e.EntityName)
		}
	}
	if len(all.Events) != 3 {
		t.Fatalf("party events: %d, want 3", len(all.Events))
	}
}

// "Carregar mais": nenhum evento repetido nem perdido entre as páginas.
func TestIntegrationAuditPagination(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	ctx := context.Background()
	if _, err := env.pool.Exec(ctx, "TRUNCATE audit_log, products RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	var p struct {
		ID      int64 `json:"id"`
		Version int32 `json:"version"`
	}
	// Cada salvar muda o produto e uma embalagem: 2 linhas por evento.
	admin.expect(http.StatusCreated, "POST", "/products", map[string]any{"name": "Tomate", "category": "vegetable", "stockUnit": "box",
		"packagings": []map[string]any{{"name": "Caixa", "quantity": "1"}}}, &p)
	const saves = 130
	for i := 1; i <= saves; i++ {
		var full struct {
			Version    int32 `json:"version"`
			Packagings []struct {
				ID int64 `json:"id"`
			} `json:"packagings"`
		}
		admin.expect(http.StatusOK, "GET", fmt.Sprintf("/products/%d", p.ID), nil, &full)
		admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/products/%d", p.ID), map[string]any{
			"name": "Tomate", "category": "vegetable", "stockUnit": "box", "notes": fmt.Sprintf("versão %d", i),
			"packagings": []map[string]any{{"id": full.Packagings[0].ID, "name": fmt.Sprintf("Caixa %d", i), "quantity": "1"}},
			"version":    full.Version,
		}, nil)
	}

	seen := map[int64]bool{}
	pages := 0
	url := fmt.Sprintf("/audit/product/%d", p.ID)
	for {
		var h historyResp
		admin.expect(http.StatusOK, "GET", url, nil, &h)
		pages++
		for _, e := range h.Events {
			if seen[e.ID] {
				t.Fatalf("event %d repeated across pages", e.ID)
			}
			seen[e.ID] = true
			if len(e.Changes) != 2 && len(seen) <= saves {
				t.Fatalf("event %d split across pages: %d changes", e.ID, len(e.Changes))
			}
		}
		if h.NextBefore == nil {
			break
		}
		url = fmt.Sprintf("/audit/product/%d?before=%d", p.ID, *h.NextBefore)
	}
	if len(seen) != saves+1 || pages < 2 {
		t.Fatalf("events: %d in %d pages, want %d in 2+", len(seen), pages, saves+1)
	}
}
