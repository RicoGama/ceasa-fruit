package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

type productResp struct {
	ID              int64   `json:"id"`
	DisplayName     string  `json:"displayName"`
	Code            *string `json:"code"`
	Ncm             *string `json:"ncm"`
	NominalWeightKg *string `json:"nominalWeightKg"`
	Version         int32   `json:"version"`
	Active          bool    `json:"active"`
	Packagings      []struct {
		ID       int64   `json:"id"`
		Name     string  `json:"name"`
		Quantity *string `json:"quantity"`
	} `json:"packagings"`
}

func TestIntegrationProducts(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	if _, err := env.pool.Exec(context.Background(), "TRUNCATE products RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	tomato := map[string]any{
		"code": " tom-ita ", "name": " Tomate ", "variety": "Italiano", "grade": "Extra AA",
		"category": "vegetable", "stockUnit": "box", "nominalWeightKg": "18",
		"sellsByWeight": true, "ncm": "0702.00.00",
		"packagings": []map[string]any{
			{"name": "Pilha", "quantity": "8"},
			{"name": "Pallet"}, // varia: informada na venda
		},
	}

	// --- Criar -------------------------------------------------------------------
	var p productResp
	admin.expect(http.StatusCreated, "POST", "/products", tomato, &p)
	if p.DisplayName != "Tomate Italiano Extra AA" || *p.Code != "TOM-ITA" || *p.Ncm != "07020000" ||
		*p.NominalWeightKg != "18" || len(p.Packagings) != 2 || p.Packagings[1].Quantity != nil || p.Version != 1 {
		t.Fatalf("created: %+v", p)
	}

	// Mesmo nome + variedade + classificação (sem diferenciar maiúsculas) -> 409.
	dup := map[string]any{"name": "TOMATE", "variety": "italiano", "grade": "extra aa", "category": "vegetable", "stockUnit": "box"}
	admin.expect(http.StatusConflict, "POST", "/products", dup, nil)
	// Mesmo código -> 409.
	admin.expect(http.StatusConflict, "POST", "/products",
		map[string]any{"code": "TOM-ITA", "name": "Outro", "category": "vegetable", "stockUnit": "box"}, nil)

	// --- Regras -------------------------------------------------------------------
	for name, body := range map[string]map[string]any{
		"sell by weight without weight": {"name": "Chuchu", "category": "vegetable", "stockUnit": "box", "sellsByWeight": true},
		"NCM with 7 digits":             {"name": "Chuchu", "category": "vegetable", "stockUnit": "box", "ncm": "0709999"},
		"duplicate packaging name":      {"name": "Chuchu", "category": "vegetable", "stockUnit": "box", "packagings": []map[string]any{{"name": "Pilha"}, {"name": "pilha"}}},
		"zero weight":                   {"name": "Chuchu", "category": "vegetable", "stockUnit": "box", "nominalWeightKg": "0"},
	} {
		if st := admin.do("POST", "/products", body, nil); st != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", name, st)
		}
	}
	// Vendido a quilo com unidade kg não precisa de peso médio.
	admin.expect(http.StatusCreated, "POST", "/products",
		map[string]any{"name": "Gengibre", "category": "vegetable", "stockUnit": "kg", "sellsByWeight": true}, nil)
	var egg productResp
	admin.expect(http.StatusCreated, "POST", "/products",
		map[string]any{"name": "Ovo", "variety": "Branco", "grade": "Extra", "category": "egg", "stockUnit": "unit",
			"packagings": []map[string]any{{"name": "Caixa de 30", "quantity": "30"}, {"name": "Caixa de 20", "quantity": "20"}}}, &egg)

	// --- Listar e buscar ----------------------------------------------------------
	var list struct {
		Total int64 `json:"total"`
		Items []struct {
			DisplayName string `json:"displayName"`
		} `json:"items"`
	}
	admin.expect(http.StatusOK, "GET", "/products?search=italiano", nil, &list)
	if list.Total != 1 || list.Items[0].DisplayName != "Tomate Italiano Extra AA" {
		t.Errorf("search by variety: %+v", list)
	}
	admin.expect(http.StatusOK, "GET", "/products?search=tom-", nil, &list) // prefixo do código
	if list.Total != 1 {
		t.Errorf("search by code: %+v", list)
	}
	admin.expect(http.StatusOK, "GET", "/products?category=egg", nil, &list)
	if list.Total != 1 || list.Items[0].DisplayName != "Ovo Branco Extra" {
		t.Errorf("filter by category: %+v", list)
	}

	// --- Editar: embalagens atualizadas, criadas e removidas -------------------
	pilha := p.Packagings[0]
	tomato["version"] = 1
	tomato["packagings"] = []map[string]any{
		{"id": pilha.ID, "name": "Pilha", "quantity": "10"}, // muda a quantidade, mantém o id
		{"name": "Meia pilha", "quantity": "5"},             // nova
		// "Pallet" saiu da lista: é removida
	}
	var updated productResp
	admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/products/%d", p.ID), tomato, &updated)
	if updated.Version != 2 || len(updated.Packagings) != 2 || updated.Packagings[0].ID != pilha.ID ||
		*updated.Packagings[0].Quantity != "10" || updated.Packagings[1].Name != "Meia pilha" {
		t.Fatalf("updated: %+v", updated)
	}

	// Embalagem de outro produto -> 422 (não dá para "roubar" pelo id).
	tomato["version"] = 2
	tomato["packagings"] = []map[string]any{{"id": egg.Packagings[0].ID, "name": "Roubada"}}
	admin.expect(http.StatusUnprocessableEntity, "PUT", fmt.Sprintf("/products/%d", p.ID), tomato, nil)

	// Tela antiga (versão 1) -> 409.
	tomato["version"] = 1
	tomato["packagings"] = []map[string]any{}
	admin.expect(http.StatusConflict, "PUT", fmt.Sprintf("/products/%d", p.ID), tomato, nil)
	admin.expect(http.StatusNotFound, "PUT", "/products/999", tomato, nil)

	// --- Papéis -------------------------------------------------------------------
	hash, _ := auth.HashPassword("senha-do-vendedor")
	if _, err := db.New(env.pool).CreateUser(context.Background(), db.CreateUserParams{
		Name: "Vendedor", Email: "v@test.com", PasswordHash: hash, RoleID: roleID(t, env.pool, "seller"),
	}); err != nil {
		t.Fatal(err)
	}
	seller := newClient(t, env.srv.URL)
	seller.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "v@test.com", "password": "senha-do-vendedor"}, nil)
	seller.expect(http.StatusOK, "GET", "/products", nil, nil)
	seller.expect(http.StatusOK, "GET", fmt.Sprintf("/products/%d", p.ID), nil, nil)
	seller.expect(http.StatusForbidden, "POST", "/products", dup, nil)
	seller.expect(http.StatusForbidden, "DELETE", fmt.Sprintf("/products/%d", p.ID), nil, nil)

	// --- Inativar -------------------------------------------------------------------
	admin.expect(http.StatusNoContent, "DELETE", fmt.Sprintf("/products/%d", p.ID), nil, nil)
	admin.expect(http.StatusOK, "GET", "/products?status=inactive", nil, &list)
	if list.Total != 1 {
		t.Errorf("inactive list: %+v", list)
	}
	admin.expect(http.StatusOK, "GET", "/products", nil, &list) // padrão: só ativos
	if list.Total != 2 {
		t.Errorf("active list: %+v", list)
	}
}
