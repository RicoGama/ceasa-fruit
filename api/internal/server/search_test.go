package server

import (
	"net/http"
	"net/url"
	"testing"
)

// TestIntegrationSearchWildcards: "%" e "_" digitados na busca valem como
// texto, não como coringa do LIKE (like_escape, migration 00015). E texto com
// cara de SQL é só texto: a busca vai como parâmetro.
func TestIntegrationSearchWildcards(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	for _, name := range []string{"Sítio 100% Orgânico", "Sítio Bom Jardim", "Fazenda_Azul"} {
		admin.expect(http.StatusCreated, "POST", "/suppliers", map[string]any{"personType": "individual", "legalName": name}, nil)
	}

	var list struct {
		Total int `json:"total"`
	}
	cases := []struct {
		search string
		want   int
	}{
		{"sitio", 2},      // sem acento também acha
		{"%", 1},          // antes: "qualquer coisa" (3); agora: só quem tem "%"
		{"100%", 1},       //
		{"_", 1},          // antes: "qualquer letra" (3); agora: só "Fazenda_Azul"
		{"Bom%Jardim", 0}, // "%" no meio não pula letras
		{`\`, 0},          // a barra também é só texto
		{"'; DROP TABLE suppliers; --", 0},
		{"' OR '1'='1", 0},
	}
	for _, c := range cases {
		admin.expect(http.StatusOK, "GET", "/suppliers?status=all&search="+url.QueryEscape(c.search), nil, &list)
		if list.Total != c.want {
			t.Errorf("search %q: %d results, want %d", c.search, list.Total, c.want)
		}
	}
	// A tabela continua lá, com os três.
	admin.expect(http.StatusOK, "GET", "/suppliers?status=all", nil, &list)
	if list.Total != 3 {
		t.Fatalf("suppliers after the searches: %d, want 3", list.Total)
	}
}
