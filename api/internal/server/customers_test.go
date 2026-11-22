package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type customerResp struct {
	ID            int64   `json:"id"`
	LegalName     string  `json:"legalName"`
	TradeName     *string `json:"tradeName"`
	CustomerType  string  `json:"customerType"`
	Status        string  `json:"status"`
	BlockedReason *string `json:"blockedReason"`
	AlsoSupplier  bool    `json:"alsoSupplier"`
	Version       int32   `json:"version"`
	Contacts      []struct {
		Name string `json:"name"`
	} `json:"contacts"`
}

func TestIntegrationCustomers(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	// --- Cadastro simples ---------------------------------------------------------
	var sacolao customerResp
	admin.expect(http.StatusCreated, "POST", "/customers", map[string]any{
		"personType": "company", "document": "11.222.333/0001-81", "legalName": "Sacolão Boa Vista Ltda",
		"tradeName": "Sacolão Boa Vista", "customerType": "retailer",
		"contacts": []map[string]any{{"name": "Comprador", "isWhatsapp": true}},
	}, &sacolao)
	if sacolao.Status != "active" || sacolao.AlsoSupplier || sacolao.Version != 1 || len(sacolao.Contacts) != 1 {
		t.Fatalf("created: %+v", sacolao)
	}
	// Mesmo CNPJ de novo, sem aproveitar o cadastro -> 409.
	admin.expect(http.StatusConflict, "POST", "/customers", map[string]any{
		"personType": "company", "document": "11222333000181", "legalName": "Outro",
	}, nil)
	// Bloqueado precisa de motivo.
	admin.expect(http.StatusUnprocessableEntity, "POST", "/customers", map[string]any{
		"personType": "individual", "legalName": "Feirante", "status": "blocked",
	}, nil)

	// --- Editar com versão --------------------------------------------------------
	base := fmt.Sprintf("/customers/%d", sacolao.ID)
	var edited customerResp
	admin.expect(http.StatusOK, "PUT", base, map[string]any{
		"personType": "company", "document": "11222333000181", "legalName": "Sacolão Boa Vista Ltda",
		"customerType": "supermarket", "status": "blocked", "blockedReason": " cheque devolvido ", "version": 1,
	}, &edited)
	if edited.CustomerType != "supermarket" || *edited.BlockedReason != "cheque devolvido" || edited.Version != 2 || len(edited.Contacts) != 0 {
		t.Fatalf("edited: %+v", edited)
	}
	admin.expect(http.StatusConflict, "PUT", base, map[string]any{ // tela antiga
		"personType": "company", "legalName": "XX", "version": 1,
	}, nil)
	admin.expect(http.StatusNotFound, "PUT", "/customers/99999", map[string]any{
		"personType": "company", "legalName": "XX", "version": 1,
	}, nil)

	// --- Fornecedor que também vira cliente ---------------------------------------
	var sup idResp
	admin.expect(http.StatusCreated, "POST", "/suppliers", map[string]any{
		"personType": "company", "document": "11444777000161", "legalName": "Box 21 Hortifruti Ltda",
		"address": map[string]any{"cityId": 3304557},
	}, &sup)
	// Fornecedor (pessoa) não é cliente: GET /customers/{id} -> 404.
	admin.expect(http.StatusNotFound, "GET", fmt.Sprintf("/customers/%d", sup.ID), nil, nil)

	var match struct {
		ID         int64  `json:"id"`
		LegalName  string `json:"legalName"`
		Version    int32  `json:"version"`
		IsSupplier bool   `json:"isSupplier"`
		IsCustomer bool   `json:"isCustomer"`
		Address    struct {
			City *struct {
				Name string `json:"name"`
			} `json:"city"`
		} `json:"address"`
	}
	admin.expect(http.StatusOK, "GET", "/parties/by-document/11.444.777%2F0001-61", nil, &match)
	if match.ID != sup.ID || !match.IsSupplier || match.IsCustomer || match.Address.City == nil {
		t.Fatalf("lookup: %+v", match)
	}
	admin.expect(http.StatusNotFound, "GET", "/parties/by-document/52998224725", nil, nil)
	admin.expect(http.StatusUnprocessableEntity, "GET", "/parties/by-document/123", nil, nil)

	sameParty := map[string]any{
		"personType": "company", "document": "11444777000161", "legalName": "Box 21 Hortifruti Ltda",
		"tradeName": "Box 21", "address": map[string]any{"cityId": 3304557}, "customerType": "wholesaler",
		"partyId": sup.ID,
	}
	admin.expect(http.StatusUnprocessableEntity, "POST", "/customers", sameParty, nil) // falta a versão
	sameParty["partyVersion"] = match.Version + 5
	admin.expect(http.StatusConflict, "POST", "/customers", sameParty, nil) // versão velha
	sameParty["partyVersion"] = match.Version
	var box21 customerResp
	admin.expect(http.StatusCreated, "POST", "/customers", sameParty, &box21)
	if box21.ID != sup.ID || !box21.AlsoSupplier || *box21.TradeName != "Box 21" {
		t.Fatalf("customer on top of supplier: %+v", box21)
	}
	// O nome fantasia mudou no cadastro único: o fornecedor também vê.
	var supNow struct {
		TradeName    *string `json:"tradeName"`
		AlsoCustomer bool    `json:"alsoCustomer"`
		Version      int32   `json:"version"`
	}
	admin.expect(http.StatusOK, "GET", fmt.Sprintf("/suppliers/%d", sup.ID), nil, &supNow)
	if supNow.TradeName == nil || *supNow.TradeName != "Box 21" || !supNow.AlsoCustomer || supNow.Version != box21.Version {
		t.Fatalf("supplier after customer: %+v", supNow)
	}
	// Já é cliente -> 409.
	sameParty["partyVersion"] = box21.Version
	admin.expect(http.StatusConflict, "POST", "/customers", sameParty, nil)

	// --- Cliente que também vira fornecedor (o caminho inverso) -------------------
	admin.expect(http.StatusOK, "GET", "/parties/by-document/11222333000181", nil, &match)
	var supFromCustomer struct {
		ID           int64 `json:"id"`
		AlsoCustomer bool  `json:"alsoCustomer"`
	}
	admin.expect(http.StatusCreated, "POST", "/suppliers", map[string]any{
		"personType": "company", "document": "11222333000181", "legalName": "Sacolão Boa Vista Ltda",
		"partyId": match.ID, "partyVersion": match.Version,
	}, &supFromCustomer)
	if supFromCustomer.ID != sacolao.ID || !supFromCustomer.AlsoCustomer {
		t.Fatalf("supplier on top of customer: %+v", supFromCustomer)
	}

	// --- Lista e filtros ------------------------------------------------------------
	var list struct {
		Items []struct {
			ID           int64 `json:"id"`
			AlsoSupplier bool  `json:"alsoSupplier"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	admin.expect(http.StatusOK, "GET", "/customers?status=all", nil, &list)
	if list.Total != 2 || !list.Items[0].AlsoSupplier {
		t.Fatalf("list all: %+v", list)
	}
	admin.expect(http.StatusOK, "GET", "/customers", nil, &list) // padrão: só ativos
	if list.Total != 1 || list.Items[0].ID != box21.ID {
		t.Fatalf("list active: %+v", list)
	}
	admin.expect(http.StatusOK, "GET", "/customers?status=all&customerType=supermarket", nil, &list)
	if list.Total != 1 || list.Items[0].ID != sacolao.ID {
		t.Fatalf("list by type: %+v", list)
	}
	admin.expect(http.StatusOK, "GET", "/customers?status=all&search=boa%20vista", nil, &list)
	if list.Total != 1 {
		t.Fatalf("search: %+v", list)
	}

	// Inativar o cliente NÃO inativa o fornecedor (são papéis separados).
	admin.expect(http.StatusNoContent, "DELETE", fmt.Sprintf("/customers/%d", box21.ID), nil, nil)
	var supStatus struct {
		Status string `json:"status"`
	}
	admin.expect(http.StatusOK, "GET", fmt.Sprintf("/suppliers/%d", sup.ID), nil, &supStatus)
	if supStatus.Status != "active" {
		t.Fatalf("supplier status after deactivating customer: %s", supStatus.Status)
	}
	admin.expect(http.StatusNotFound, "DELETE", "/customers/99999", nil, nil)

	// --- Permissões -----------------------------------------------------------------
	// Conferente: vê clientes, não cadastra, não consulta CNPJ.
	admin.expect(http.StatusCreated, "POST", "/users", map[string]any{
		"name": "Conferente", "email": "conf@test.com", "roleId": roleID(t, env.pool, "checker"), "password": "senha-conf-123",
	}, nil)
	checker := newClient(t, env.srv.URL)
	checker.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "conf@test.com", "password": "senha-conf-123"}, nil)
	checker.expect(http.StatusOK, "GET", "/customers", nil, nil)
	checker.expect(http.StatusForbidden, "POST", "/customers", map[string]any{"personType": "company", "legalName": "X"}, nil)
	checker.expect(http.StatusForbidden, "GET", "/parties/by-document/11222333000181", nil, nil)

	// Vendedor: cadastra cliente (padrão da migration 00011).
	admin.expect(http.StatusCreated, "POST", "/users", map[string]any{
		"name": "Vendedor", "email": "vend@test.com", "roleId": roleID(t, env.pool, "seller"), "password": "senha-vend-123",
	}, nil)
	seller := newClient(t, env.srv.URL)
	seller.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "vend@test.com", "password": "senha-vend-123"}, nil)
	seller.expect(http.StatusCreated, "POST", "/customers", map[string]any{"personType": "individual", "legalName": "Dona Maria", "customerType": "street_vendor"}, nil)
	seller.expect(http.StatusOK, "GET", "/parties/by-document/11222333000181", nil, nil)
}

// Editar a pessoa pelo cliente avisa também quem está com o fornecedor aberto.
func TestCustomerChangeNotifiesSupplierScreens(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	var sup idResp
	admin.expect(http.StatusCreated, "POST", "/suppliers", map[string]any{"personType": "company", "document": "11444777000161", "legalName": "Box 21"}, &sup)
	var c customerResp
	admin.expect(http.StatusCreated, "POST", "/customers", map[string]any{
		"personType": "company", "document": "11444777000161", "legalName": "Box 21", "partyId": sup.ID, "partyVersion": 1,
	}, &c)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, env.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "LISTEN app_events"); err != nil {
		t.Fatal(err)
	}

	admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/customers/%d", c.ID), map[string]any{
		"personType": "company", "document": "11444777000161", "legalName": "Box 21 Hortifruti", "version": c.Version,
	}, nil)

	got := map[string]bool{}
	for !(got["suppliers"] && got["customers"]) {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			t.Fatalf("waiting for notifications, got %v: %v", got, err)
		}
		var ev struct {
			Topic string `json:"topic"`
			ID    int64  `json:"id"`
		}
		_ = json.Unmarshal([]byte(n.Payload), &ev)
		if ev.ID == sup.ID {
			got[ev.Topic] = true
		}
	}
}
