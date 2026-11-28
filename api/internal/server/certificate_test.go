package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RicoGama/ceasa-fruit/api/internal/certificate"
	"github.com/RicoGama/ceasa-fruit/api/internal/certificate/certtest"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/secrets"
)

// sendCertificate envia o .pfx como a tela envia (multipart/form-data).
func (c *client) sendCertificate(pfx []byte, password string) (int, string) {
	c.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "certificado.pfx")
	_, _ = fw.Write(pfx)
	_ = w.WriteField("password", password)
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, c.base+"/company/certificate", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

type certInfo struct {
	ID         int64  `json:"id"`
	HolderName string `json:"holderName"`
	Document   string `json:"document"`
	Status     string `json:"status"`
	DaysLeft   int    `json:"daysLeft"`
	UploadedBy *struct {
		Name string `json:"name"`
	} `json:"uploadedBy"`
	ReplacedAt *time.Time `json:"replacedAt"`
}

type certList struct {
	Current       *certInfo  `json:"current"`
	History       []certInfo `json:"history"`
	KeyConfigured bool       `json:"keyConfigured"`
}

const testSecretsKey = "chave-de-teste-dos-segredos-com-mais-de-32"

func TestIntegrationCertificate(t *testing.T) {
	box, _ := secrets.New(testSecretsKey)
	env := setupIntegrationEnv(t, func(d *Deps) { d.Secrets = box })
	resetDatabase(t, env.pool)
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)
	ctx := context.Background()

	var list certList
	admin.expect(http.StatusOK, "GET", "/company/certificate", nil, &list)
	if list.Current != nil || !list.KeyConfigured || len(list.History) != 0 {
		t.Fatalf("empty: %+v", list)
	}

	pfx, pass := certtest.PFX(certtest.Options{}) // CNPJ 11222333000181

	// Sem o CNPJ da empresa preenchido, não dá para conferir o certificado.
	if st, body := admin.sendCertificate(pfx, pass); st != http.StatusUnprocessableEntity || !strings.Contains(body, "fill in the company CNPJ") {
		t.Fatalf("without company CNPJ: %d %s", st, body)
	}
	if _, err := env.pool.Exec(ctx, "UPDATE company SET document = '11222333000181'"); err != nil {
		t.Fatal(err)
	}

	// Senha errada, arquivo qualquer, CNPJ de outra empresa: recusados.
	for _, c := range []struct {
		name, want string
		pfx        []byte
		pass       string
	}{
		{"wrong password", "wrong certificate password", pfx, "errada"},
		{"not a certificate", "not a valid A1 certificate", []byte("PK\x03\x04 planilha.xlsx"), pass},
		{"other company", "does not match the company CNPJ", first(certtest.PFX(certtest.Options{CNPJ: "11444777000161"})), "senha-teste"},
	} {
		if st, body := admin.sendCertificate(c.pfx, c.pass); st != http.StatusUnprocessableEntity || !strings.Contains(body, c.want) {
			t.Fatalf("%s: %d %s", c.name, st, body)
		}
	}
	// Filial (mesma raiz de CNPJ) é aceita: a matriz assina pelas filiais.
	branch, branchPass := certtest.PFX(certtest.Options{CNPJ: "11222333000262"})
	if st, body := admin.sendCertificate(branch, branchPass); st != http.StatusCreated {
		t.Fatalf("branch certificate: %d %s", st, body)
	}

	// --- Envio válido ----------------------------------------------------------------
	if st, body := admin.sendCertificate(pfx, pass); st != http.StatusCreated {
		t.Fatalf("upload: %d %s", st, body)
	}
	admin.expect(http.StatusOK, "GET", "/company/certificate", nil, &list)
	if list.Current == nil || list.Current.HolderName != "IRMAOS FAISAO FRUTICOLA LTDA" || list.Current.Document != "11222333000181" ||
		list.Current.Status != "valid" || list.Current.UploadedBy == nil || list.Current.UploadedBy.Name != "Admin" {
		t.Fatalf("current: %+v", list.Current)
	}
	if len(list.History) != 1 || list.History[0].ReplacedAt == nil {
		t.Fatalf("the branch certificate should be in the history: %+v", list.History)
	}

	// No banco: criptografado. Nem o arquivo nem a senha aparecem.
	var sealedPFX, sealedPass []byte
	if err := env.pool.QueryRow(ctx, "SELECT encrypted_pfx, encrypted_password FROM digital_certificates WHERE active").Scan(&sealedPFX, &sealedPass); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealedPFX, pfx[20:60]) || bytes.Contains(sealedPass, []byte(pass)) {
		t.Fatal("certificate or password stored in clear")
	}
	// O substituído não guarda mais o arquivo.
	if n := countRows(t, env, "SELECT count(*) FROM digital_certificates WHERE NOT active AND encrypted_pfx IS NULL AND encrypted_password IS NULL"); n != 1 {
		t.Fatalf("retired certificates still holding the file: %d", 1-n)
	}
	// O histórico de alterações registra o envio, sem o arquivo nem a senha.
	var changes string
	if err := env.pool.QueryRow(ctx, "SELECT string_agg(changes::text, ' ') FROM audit_log WHERE table_name = 'digital_certificates'").Scan(&changes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(changes, "holder_name") || strings.Contains(changes, "encrypted") || strings.Contains(changes, "key_id") {
		t.Fatalf("audit: %s", changes)
	}

	// --- Pronto para assinar (o que a nota fiscal vai usar) ---------------------------
	database := postgres.NewDB(env.pool)
	err := database.InReadTx(ctx, func(tx postgres.Tx) error {
		p, err := certificate.Load(ctx, tx, box, time.Now())
		if err != nil {
			return err
		}
		if p.Key == nil || p.Document != "11222333000181" {
			t.Fatalf("loaded: %+v", p.Info)
		}
		// Outra chave (banco restaurado sem a SECRETS_KEY antiga): erro claro.
		other, _ := secrets.New(testSecretsKey + "-outra")
		if _, err := certificate.Load(ctx, tx, other, time.Now()); !errors.Is(err, certificate.ErrOtherKey) {
			t.Fatalf("other key: %v", err)
		}
		// Depois de vencer, não assina mais.
		if _, err := certificate.Load(ctx, tx, box, time.Now().AddDate(2, 0, 0)); !errors.Is(err, certificate.ErrExpired) {
			t.Fatalf("after expiration: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// --- Troca por um que vence em 10 dias: aviso de vencimento ------------------------
	soon, soonPass := certtest.PFX(certtest.Options{NotAfter: time.Now().Add(10*24*time.Hour + time.Hour)})
	if st, body := admin.sendCertificate(soon, soonPass); st != http.StatusCreated {
		t.Fatalf("replace: %d %s", st, body)
	}
	admin.expect(http.StatusOK, "GET", "/company/certificate", nil, &list)
	if list.Current.Status != "expiring" || list.Current.DaysLeft != 10 || len(list.History) != 2 {
		t.Fatalf("expiring: %+v, history %d", list.Current, len(list.History))
	}

	// --- Cinco envios ao mesmo tempo: um ativo no fim, nenhum erro ---------------------
	var wg sync.WaitGroup
	statuses := make(chan int, 5)
	for range 5 {
		wg.Go(func() {
			st, _ := admin.sendCertificate(pfx, pass)
			statuses <- st
		})
	}
	wg.Wait()
	close(statuses)
	for st := range statuses {
		if st != http.StatusCreated {
			t.Fatalf("concurrent upload: %d", st)
		}
	}
	if n := countRows(t, env, "SELECT count(*) FROM digital_certificates WHERE active"); n != 1 {
		t.Fatalf("active certificates: %d", n)
	}

	// --- Sem a permissão: nada ---------------------------------------------------------
	admin.expect(http.StatusCreated, "POST", "/users", map[string]any{
		"name": "Gerente", "email": "ger@test.com", "roleId": roleID(t, env.pool, "manager"), "password": "senha-ger-123",
	}, nil)
	manager := newClient(t, env.srv.URL)
	manager.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "ger@test.com", "password": "senha-ger-123"}, nil)
	manager.expect(http.StatusForbidden, "GET", "/company/certificate", nil, nil)
	if st, _ := manager.sendCertificate(pfx, pass); st != http.StatusForbidden {
		t.Fatalf("manager upload: %d", st)
	}

	// --- Remover --------------------------------------------------------------------------
	admin.expect(http.StatusNoContent, "DELETE", "/company/certificate", nil, nil)
	admin.expect(http.StatusOK, "GET", "/company/certificate", nil, &list)
	if list.Current != nil {
		t.Fatalf("after delete: %+v", list.Current)
	}
	admin.expect(http.StatusNotFound, "DELETE", "/company/certificate", nil, nil)
	if n := countRows(t, env, "SELECT count(*) FROM digital_certificates WHERE encrypted_pfx IS NOT NULL"); n != 0 {
		t.Fatalf("files left after delete: %d", n)
	}
}

// Sem SECRETS_KEY o servidor sobe, mas recusa o certificado e a tela explica.
func TestIntegrationCertificateWithoutKey(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	var list certList
	admin.expect(http.StatusOK, "GET", "/company/certificate", nil, &list)
	if list.KeyConfigured {
		t.Fatal("keyConfigured without a key")
	}
	pfx, pass := certtest.PFX(certtest.Options{})
	if st, body := admin.sendCertificate(pfx, pass); st != http.StatusUnprocessableEntity || !strings.Contains(body, "SECRETS_KEY") {
		t.Fatalf("upload without key: %d %s", st, body)
	}
}

func first(b []byte, _ string) []byte { return b }
