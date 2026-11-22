package server

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/RicoGama/ceasa-fruit/api/internal/cnpj"
)

// Respostas no formato documentado de cada serviço (abreviadas).
const brasilAPIPetrobras = `{
  "cnpj": "33000167000101", "razao_social": "PETROLEO BRASILEIRO S A PETROBRAS", "nome_fantasia": "PETROBRAS",
  "descricao_situacao_cadastral": "ATIVA", "data_inicio_atividade": "1966-09-28",
  "cnae_fiscal_descricao": "Extração de petróleo e gás natural", "opcao_pelo_simples": false,
  "cep": "20231030", "descricao_tipo_de_logradouro": "AVENIDA", "logradouro": "REPUBLICA DO CHILE", "numero": "65",
  "complemento": "", "bairro": "CENTRO", "municipio": "RIO DE JANEIRO", "uf": "RJ", "codigo_municipio_ibge": 3304557,
  "ddd_telefone_1": "2132242164", "email": null
}`

// Sem código do IBGE: o município é achado pelo nome (sem acento) + UF.
const brasilAPISemIBGE = `{
  "cnpj": "11222333000181", "razao_social": "HORTIFRUTI DO GONCALO LTDA", "nome_fantasia": "",
  "descricao_situacao_cadastral": "BAIXADA", "cep": "24400000", "logradouro": "RUA DR NILO PECANHA", "numero": "S/N",
  "bairro": "CENTRO", "municipio": "SAO GONCALO", "uf": "RJ", "ddd_telefone_1": ""
}`

const cnpjaPetrobras = `{
  "taxId": "33000167000101", "alias": "Petrobras", "founded": "1966-09-28",
  "company": { "name": "PETROLEO BRASILEIRO S A PETROBRAS", "simples": { "optant": false } },
  "status": { "id": 2, "text": "Ativa" },
  "address": { "municipality": 3304557, "street": "Avenida Republica do Chile", "number": "65", "details": null,
               "district": "Centro", "city": "Rio de Janeiro", "state": "RJ", "zip": "20231030" },
  "phones": [ { "area": "21", "number": "32242164" } ],
  "emails": [ { "address": "Contato@Petrobras.com.br" } ],
  "mainActivity": { "id": 600001, "text": "Extração de petróleo e gás natural" }
}`

type cnpjResp struct {
	LegalName string  `json:"legalName"`
	TradeName *string `json:"tradeName"`
	Status    string  `json:"status"`
	Active    bool    `json:"active"`
	Phone     *string `json:"phone"`
	Email     *string `json:"email"`
	Source    string  `json:"source"`
	Address   struct {
		Street     *string `json:"street"`
		Number     *string `json:"number"`
		Complement *string `json:"complement"`
		District   *string `json:"district"`
		PostalCode *string `json:"postalCode"`
		City       *struct {
			ID    int32  `json:"id"`
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"city"`
	} `json:"address"`
}

func TestIntegrationCNPJLookup(t *testing.T) {
	var brasilCalls, cnpjaCalls atomic.Int32
	brasilDown := atomic.Bool{}
	brasil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		brasilCalls.Add(1)
		if brasilDown.Load() {
			w.WriteHeader(http.StatusTooManyRequests) // limite de consultas
			return
		}
		switch r.URL.Path {
		case "/33000167000101":
			_, _ = w.Write([]byte(brasilAPIPetrobras))
		case "/11222333000181":
			_, _ = w.Write([]byte(brasilAPISemIBGE))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer brasil.Close()
	cnpjaDown := atomic.Bool{}
	cnpja := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cnpjaCalls.Add(1)
		if cnpjaDown.Load() || r.URL.Path != "/33000167000101" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(cnpjaPetrobras))
	}))
	defer cnpja.Close()

	env := setupIntegrationEnv(t, func(d *Deps) {
		d.CNPJProviders = []cnpj.Provider{cnpj.BrasilAPI{BaseURL: brasil.URL}, cnpj.CNPJa{BaseURL: cnpja.URL}}
	})
	resetDatabase(t, env.pool)
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	// BrasilAPI responde: nomes, endereço "bonito" e município do IBGE.
	var c cnpjResp
	admin.expect(http.StatusOK, "GET", "/cnpj/33.000.167%2F0001-01", nil, &c)
	if c.LegalName != "PETROLEO BRASILEIRO S A PETROBRAS" || *c.TradeName != "PETROBRAS" || !c.Active || c.Source != "BrasilAPI" ||
		*c.Address.Street != "Avenida Republica do Chile" || *c.Address.District != "Centro" || c.Address.Complement != nil ||
		c.Address.City == nil || c.Address.City.ID != 3304557 || *c.Phone != "2132242164" || c.Email != nil {
		t.Fatalf("brasilapi: %+v city %+v", c, c.Address.City)
	}
	// Segunda consulta: vem do cache, sem chamar o serviço.
	admin.expect(http.StatusOK, "GET", "/cnpj/33000167000101", nil, &c)
	if brasilCalls.Load() != 1 {
		t.Fatalf("cache not used: %d calls", brasilCalls.Load())
	}

	// Sem código do IBGE: acha "São Gonçalo" pelo nome; situação BAIXADA.
	admin.expect(http.StatusOK, "GET", "/cnpj/11222333000181", nil, &c)
	if c.Active || c.Status != "BAIXADA" || c.TradeName != nil || c.Address.City == nil || c.Address.City.Name != "São Gonçalo" {
		t.Fatalf("city by name: %+v city %+v", c, c.Address.City)
	}

	// Não existe na Receita.
	admin.expect(http.StatusNotFound, "GET", "/cnpj/11444777000161", nil, nil)
	admin.expect(http.StatusUnprocessableEntity, "GET", "/cnpj/11111111111111", nil, nil)

	// BrasilAPI no limite de consultas: usa a CNPJá.
	env2 := setupIntegrationEnv(t, func(d *Deps) {
		d.CNPJProviders = []cnpj.Provider{cnpj.BrasilAPI{BaseURL: brasil.URL}, cnpj.CNPJa{BaseURL: cnpja.URL}}
	})
	admin2 := newClient(t, env2.srv.URL)
	admin2.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)
	brasilDown.Store(true)
	admin2.expect(http.StatusOK, "GET", "/cnpj/33000167000101", nil, &c)
	if c.Source != "CNPJá" || *c.TradeName != "Petrobras" || c.Address.City == nil || c.Address.City.ID != 3304557 ||
		*c.Phone != "2132242164" || *c.Email != "contato@petrobras.com.br" || !c.Active {
		t.Fatalf("cnpja fallback: %+v", c)
	}

	// Os dois fora (sem internet): 503, para a tela pedir o preenchimento à mão.
	env3 := setupIntegrationEnv(t, func(d *Deps) {
		d.CNPJProviders = []cnpj.Provider{cnpj.BrasilAPI{BaseURL: brasil.URL}, cnpj.CNPJa{BaseURL: cnpja.URL}}
	})
	admin3 := newClient(t, env3.srv.URL)
	admin3.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)
	cnpjaDown.Store(true)
	admin3.expect(http.StatusServiceUnavailable, "GET", "/cnpj/33000167000101", nil, nil)

	// Sem permissão de cadastrar: 403.
	admin.expect(http.StatusCreated, "POST", "/users", map[string]any{
		"name": "Conferente", "email": "conf@test.com", "roleId": roleID(t, env.pool, "checker"), "password": "senha-conf-123",
	}, nil)
	checker := newClient(t, env.srv.URL)
	checker.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "conf@test.com", "password": "senha-conf-123"}, nil)
	checker.expect(http.StatusForbidden, "GET", "/cnpj/33000167000101", nil, nil)
}
