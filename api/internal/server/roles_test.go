package server

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

type roleResp struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	IsAdmin     bool     `json:"isAdmin"`
	Permissions []string `json:"permissions"`
	ActiveUsers int64    `json:"activeUsers"`
	Version     int32    `json:"version"`
}

func TestIntegrationRoles(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	if _, err := env.pool.Exec(t.Context(), "DELETE FROM roles WHERE key IS NULL"); err != nil {
		t.Fatal(err)
	}
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	// --- Papéis que vêm com o sistema -------------------------------------------
	var list struct{ Items []roleResp }
	admin.expect(http.StatusOK, "GET", "/roles", nil, &list)
	if len(list.Items) != 6 || !list.Items[0].IsAdmin || list.Items[0].ActiveUsers != 1 || len(list.Items[0].Permissions) < 7 {
		t.Fatalf("seeded roles: %+v", list.Items)
	}

	// --- Criar papel novo --------------------------------------------------------
	var external roleResp
	admin.expect(http.StatusCreated, "POST", "/roles", map[string]any{
		"name": " Vendedor externo ", "description": "Visita clientes na rua",
		"permissions": []string{"suppliers.edit"},
	}, &external)
	// suppliers.edit traz suppliers.view junto (não faz sentido editar sem ver).
	if external.Name != "Vendedor externo" || !slices.Equal(external.Permissions, []string{"suppliers.edit", "suppliers.view"}) {
		t.Fatalf("created role: %+v", external)
	}
	admin.expect(http.StatusConflict, "POST", "/roles", map[string]any{"name": "vendedor EXTERNO", "permissions": []string{}}, nil)
	admin.expect(http.StatusUnprocessableEntity, "POST", "/roles", map[string]any{"name": "Hacker", "permissions": []string{"tudo.liberado"}}, nil)

	// --- Usuário com o papel novo -------------------------------------------------
	admin.expect(http.StatusCreated, "POST", "/users", map[string]any{
		"name": "Rua", "email": "rua@test.com", "roleId": external.ID, "password": "senha-da-rua-1",
	}, nil)
	rua := newClient(t, env.srv.URL)
	var me me
	rua.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "rua@test.com", "password": "senha-da-rua-1"}, &me)
	if me.User.Role.Name != "Vendedor externo" || !slices.Equal(me.User.Permissions, []string{"suppliers.edit", "suppliers.view"}) {
		t.Fatalf("me: %+v", me.User)
	}
	rua.expect(http.StatusCreated, "POST", "/suppliers", map[string]any{"personType": "company", "legalName": "Sítio da Rua"}, nil)
	rua.expect(http.StatusForbidden, "GET", "/products", nil, nil)
	rua.expect(http.StatusForbidden, "GET", "/roles", nil, nil)

	// --- Mudar as permissões do papel vale NA HORA (mesma sessão) ---------------
	admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/roles/%d", external.ID), map[string]any{
		"name": "Vendedor externo", "permissions": []string{"suppliers.view", "products.view"}, "version": 1,
	}, &external)
	rua.expect(http.StatusOK, "GET", "/products", nil, nil)
	rua.expect(http.StatusForbidden, "POST", "/suppliers", map[string]any{"personType": "company", "legalName": "Outro"}, nil)

	// Tela antiga (versão 1) -> 409.
	admin.expect(http.StatusConflict, "PUT", fmt.Sprintf("/roles/%d", external.ID), map[string]any{
		"name": "Vendedor externo", "permissions": []string{}, "version": 1,
	}, nil)

	// --- O Administrador é do sistema ---------------------------------------------
	adminRole := list.Items[0]
	admin.expect(http.StatusConflict, "PUT", fmt.Sprintf("/roles/%d", adminRole.ID), map[string]any{
		"name": "Chefe", "permissions": []string{}, "version": adminRole.Version,
	}, nil)
	admin.expect(http.StatusConflict, "DELETE", fmt.Sprintf("/roles/%d", adminRole.ID), nil, nil)

	// --- Papel com usuário não pode ser apagado ---------------------------------
	admin.expect(http.StatusConflict, "DELETE", fmt.Sprintf("/roles/%d", external.ID), nil, nil)
	var users struct {
		Items []struct {
			ID      int64  `json:"id"`
			Email   string `json:"email"`
			Version int32  `json:"version"`
		} `json:"items"`
	}
	admin.expect(http.StatusOK, "GET", "/users", nil, &users)
	for _, u := range users.Items {
		if u.Email == "rua@test.com" {
			admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/users/%d", u.ID), map[string]any{
				"name": "Rua", "roleId": roleID(t, env.pool, "seller"), "active": true, "version": u.Version,
			}, nil)
		}
	}
	admin.expect(http.StatusNoContent, "DELETE", fmt.Sprintf("/roles/%d", external.ID), nil, nil)
	admin.expect(http.StatusNotFound, "GET", fmt.Sprintf("/roles/%d", external.ID), nil, nil)

	// Papel inexistente ao criar usuário -> 422.
	admin.expect(http.StatusUnprocessableEntity, "POST", "/users", map[string]any{
		"name": "Ninguém", "email": "n@test.com", "roleId": 99999,
	}, nil)
}
