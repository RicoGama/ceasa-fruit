package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

func TestIntegrationSupplierProducts(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	if _, err := env.pool.Exec(context.Background(), "TRUNCATE products RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	var sup, tomato, chuchu, old idResp
	admin.expect(http.StatusCreated, "POST", "/suppliers", map[string]any{
		"personType": "company", "legalName": "Sítio do Zé",
		"defaultPurchaseMode": "consignment", "consignmentCommission": "10",
	}, &sup)
	admin.expect(http.StatusCreated, "POST", "/products", map[string]any{"name": "Tomate", "variety": "Italiano", "category": "vegetable", "stockUnit": "box"}, &tomato)
	admin.expect(http.StatusCreated, "POST", "/products", map[string]any{"name": "Chuchu", "category": "vegetable", "stockUnit": "box"}, &chuchu)
	admin.expect(http.StatusCreated, "POST", "/products", map[string]any{"name": "Fora de linha", "category": "fruit", "stockUnit": "box", "active": false}, &old)
	base := fmt.Sprintf("/suppliers/%d/products", sup.ID)

	type link struct {
		ProductID int64 `json:"productId"`
		Product   struct {
			DisplayName string `json:"displayName"`
		} `json:"product"`
		SupplierProductName   *string `json:"supplierProductName"`
		Months                []int   `json:"months"`
		PurchaseMode          *string `json:"purchaseMode"`
		ConsignmentCommission *string `json:"consignmentCommission"`
		Preferred             bool    `json:"preferred"`
		Version               int32   `json:"version"`
	}

	// --- Ligar --------------------------------------------------------------------
	var l link
	admin.expect(http.StatusCreated, "POST", base, map[string]any{
		"productId": tomato.ID, "supplierProductName": " Tomate saladete ",
		"months": []int{12, 1, 2, 3, 1}, "preferred": true,
	}, &l)
	if l.Product.DisplayName != "Tomate Italiano" || *l.SupplierProductName != "Tomate saladete" ||
		!slices.Equal(l.Months, []int{1, 2, 3, 12}) || !l.Preferred || l.Version != 1 || l.PurchaseMode != nil {
		t.Fatalf("created link: %+v", l)
	}
	admin.expect(http.StatusConflict, "POST", base, map[string]any{"productId": tomato.ID}, nil)         // já ligado
	admin.expect(http.StatusUnprocessableEntity, "POST", base, map[string]any{"productId": old.ID}, nil) // produto inativo
	admin.expect(http.StatusUnprocessableEntity, "POST", base, map[string]any{"productId": 99999}, nil)  // produto não existe
	admin.expect(http.StatusNotFound, "POST", "/suppliers/99999/products", map[string]any{"productId": tomato.ID}, nil)
	admin.expect(http.StatusUnprocessableEntity, "POST", base, map[string]any{"productId": chuchu.ID, "months": []int{13}}, nil)
	admin.expect(http.StatusUnprocessableEntity, "POST", base, map[string]any{"productId": chuchu.ID, "purchaseMode": "consignment"}, nil)

	// Compra direta com comissão: a comissão é descartada. 12 meses = o ano todo.
	admin.expect(http.StatusCreated, "POST", base, map[string]any{
		"productId": chuchu.ID, "purchaseMode": "direct", "consignmentCommission": "5",
		"months": []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
	}, &l)
	if l.ConsignmentCommission != nil || len(l.Months) != 0 || *l.PurchaseMode != "direct" {
		t.Fatalf("chuchu link: %+v", l)
	}

	// --- Listar -------------------------------------------------------------------
	var list struct{ Items []link }
	admin.expect(http.StatusOK, "GET", base, nil, &list)
	if len(list.Items) != 2 || list.Items[0].ProductID != tomato.ID { // preferido primeiro
		t.Fatalf("list: %+v", list.Items)
	}
	admin.expect(http.StatusNotFound, "GET", "/suppliers/99999/products", nil, nil)

	// --- Alterar com versão -------------------------------------------------------
	admin.expect(http.StatusOK, "PUT", fmt.Sprintf("%s/%d", base, tomato.ID), map[string]any{
		"supplierProductName": "Tomate saladete", "months": []int{11, 12}, "purchaseMode": "consignment",
		"consignmentCommission": "12.5", "preferred": true, "version": 1,
	}, &l)
	if l.Version != 2 || *l.ConsignmentCommission != "12.5" {
		t.Fatalf("updated: %+v", l)
	}
	admin.expect(http.StatusConflict, "PUT", fmt.Sprintf("%s/%d", base, tomato.ID), map[string]any{"version": 1}, nil)
	admin.expect(http.StatusNotFound, "PUT", fmt.Sprintf("%s/%d", base, old.ID), map[string]any{"version": 1}, nil)

	// --- Pelo produto: quem fornece -------------------------------------------------
	var ps struct {
		Items []struct {
			LegalName             string  `json:"legalName"`
			PurchaseMode          string  `json:"purchaseMode"`
			ConsignmentCommission *string `json:"consignmentCommission"`
		} `json:"items"`
	}
	admin.expect(http.StatusOK, "GET", fmt.Sprintf("/products/%d/suppliers", tomato.ID), nil, &ps)
	if len(ps.Items) != 1 || ps.Items[0].LegalName != "Sítio do Zé" || *ps.Items[0].ConsignmentCommission != "12.5" {
		t.Fatalf("product suppliers: %+v", ps.Items)
	}
	admin.expect(http.StatusNotFound, "GET", "/products/99999/suppliers", nil, nil)

	// --- "Quem tem chuchu?" na lista de fornecedores --------------------------------
	var sl struct {
		Total int64 `json:"total"`
	}
	admin.expect(http.StatusOK, "GET", fmt.Sprintf("/suppliers?productId=%d", chuchu.ID), nil, &sl)
	if sl.Total != 1 {
		t.Errorf("suppliers with chuchu = %d", sl.Total)
	}
	admin.expect(http.StatusOK, "GET", fmt.Sprintf("/suppliers?productId=%d", old.ID), nil, &sl)
	if sl.Total != 0 {
		t.Errorf("suppliers with old product = %d", sl.Total)
	}

	// --- Papéis ---------------------------------------------------------------------
	login := func(email, role string) *client {
		hash, _ := auth.HashPassword("senha-qualquer-1")
		if _, err := db.New(env.pool).CreateUser(context.Background(), db.CreateUserParams{
			Name: email, Email: email, PasswordHash: hash, RoleID: roleID(t, env.pool, role),
		}); err != nil {
			t.Fatal(err)
		}
		c := newClient(t, env.srv.URL)
		c.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": email, "password": "senha-qualquer-1"}, nil)
		return c
	}
	seller := login("v@test.com", "seller") // vê fornecedores e produtos
	seller.expect(http.StatusOK, "GET", base, nil, nil)
	seller.expect(http.StatusOK, "GET", fmt.Sprintf("/products/%d/suppliers", tomato.ID), nil, nil)
	seller.expect(http.StatusForbidden, "POST", base, map[string]any{"productId": chuchu.ID}, nil)
	driver := login("m@test.com", "driver") // só vê produtos: não vê quem fornece
	driver.expect(http.StatusForbidden, "GET", fmt.Sprintf("/products/%d/suppliers", tomato.ID), nil, nil)

	// --- Desligar -------------------------------------------------------------------
	admin.expect(http.StatusNoContent, "DELETE", fmt.Sprintf("%s/%d", base, chuchu.ID), nil, nil)
	admin.expect(http.StatusNotFound, "DELETE", fmt.Sprintf("%s/%d", base, chuchu.ID), nil, nil)
}
