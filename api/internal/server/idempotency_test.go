package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
)

// withKey envia um pedido com Idempotency-Key e devolve status, corpo e se a
// resposta veio do guardado.
func (c *client) withKey(key, method, path string, body any) (int, []byte, bool) {
	c.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, c.base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out, resp.Header.Get("Idempotent-Replayed") == "true"
}

func countRows(t *testing.T, env *integrationEnv, sql string) int {
	t.Helper()
	var n int
	if err := env.pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIntegrationIdempotency(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	if _, err := env.pool.Exec(context.Background(), "TRUNCATE idempotency_keys"); err != nil {
		t.Fatal(err)
	}
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)
	suppliers := func() int { return countRows(t, env, "SELECT count(*) FROM suppliers") }

	// --- Resposta perdida: a tela tenta de novo com a mesma chave ------------------
	body := map[string]any{"personType": "individual", "legalName": "Sítio sem CPF"} // nada único: duplicaria
	st1, b1, rep1 := admin.withKey("chave-teste-0000000001", "POST", "/suppliers", body)
	st2, b2, rep2 := admin.withKey("chave-teste-0000000001", "POST", "/suppliers", body)
	if st1 != http.StatusCreated || st2 != http.StatusCreated || rep1 || !rep2 || !bytes.Equal(b1, b2) {
		t.Fatalf("replay: %d/%v then %d/%v, same body=%v", st1, rep1, st2, rep2, bytes.Equal(b1, b2))
	}
	if n := suppliers(); n != 1 {
		t.Fatalf("suppliers after repeated request: %d, want 1", n)
	}
	// Sem chave (scripts, testes antigos): funciona como sempre.
	admin.expect(http.StatusCreated, "POST", "/suppliers", body, nil)
	if n := suppliers(); n != 2 {
		t.Fatalf("suppliers without key: %d, want 2", n)
	}

	// --- Edição repetida: devolve o sucesso, e não "outra pessoa alterou" ----------
	var created struct {
		ID      int64 `json:"id"`
		Version int32 `json:"version"`
	}
	_ = json.Unmarshal(b1, &created)
	edit := map[string]any{"personType": "individual", "legalName": "Sítio do Zé", "version": created.Version}
	path := fmt.Sprintf("/suppliers/%d", created.ID)
	stA, _, _ := admin.withKey("chave-teste-edicao-0001", "PUT", path, edit)
	stB, _, repB := admin.withKey("chave-teste-edicao-0001", "PUT", path, edit)
	if stA != http.StatusOK || stB != http.StatusOK || !repB {
		t.Fatalf("repeated edit: %d then %d (replayed %v), want 200 twice", stA, stB, repB)
	}
	// O mesmo PUT com chave nova (outro clique, de verdade): versão velha -> 409.
	if st, _, _ := admin.withKey("chave-teste-edicao-0002", "PUT", path, edit); st != http.StatusConflict {
		t.Fatalf("new key with stale version: %d, want 409", st)
	}

	// --- Mesma chave, outro conteúdo: erro de programa ------------------------------
	if st, _, _ := admin.withKey("chave-teste-0000000001", "POST", "/suppliers", map[string]any{"personType": "individual", "legalName": "Outro"}); st != http.StatusUnprocessableEntity {
		t.Fatalf("same key, different body: %d, want 422", st)
	}
	if st, _, _ := admin.withKey("curta", "POST", "/suppliers", body); st != http.StatusBadRequest {
		t.Fatalf("invalid key: %d, want 400", st)
	}

	// --- Erro antes de gravar não "gasta" a chave --------------------------------
	bad := map[string]any{"personType": "individual", "legalName": "X"} // nome curto: 422
	if st, _, _ := admin.withKey("chave-teste-validacao-01", "POST", "/suppliers", bad); st != http.StatusUnprocessableEntity {
		t.Fatalf("validation: %d", st)
	}
	fixed := map[string]any{"personType": "individual", "legalName": "Xavier"}
	if st, _, _ := admin.withKey("chave-teste-validacao-01", "POST", "/suppliers", fixed); st != http.StatusCreated {
		t.Fatalf("corrected request with the same key: %d, want 201", st)
	}

	// --- Dez pedidos iguais ao mesmo tempo: grava um ----------------------------
	before := suppliers()
	same := map[string]any{"personType": "individual", "legalName": "Clique nervoso"}
	var wg sync.WaitGroup
	statuses := make(chan int, 10)
	for range 10 {
		wg.Go(func() {
			st, _, _ := admin.withKey("chave-teste-concorrente1", "POST", "/suppliers", same)
			statuses <- st
		})
	}
	wg.Wait()
	close(statuses)
	for st := range statuses {
		if st != http.StatusCreated && st != http.StatusConflict {
			t.Fatalf("concurrent duplicate got %d (want 201 or 409 'in progress')", st)
		}
	}
	if n := suppliers() - before; n != 1 {
		t.Fatalf("concurrent duplicates created %d suppliers, want 1", n)
	}
	// Depois de terminar, a repetição devolve o resultado.
	if st, _, rep := admin.withKey("chave-teste-concorrente1", "POST", "/suppliers", same); st != http.StatusCreated || !rep {
		t.Fatalf("after concurrent burst: %d replayed=%v", st, rep)
	}

	// --- A chave é do usuário: outro usuário não "pega" a resposta ----------------
	admin.expect(http.StatusCreated, "POST", "/users", map[string]any{
		"name": "Gerente", "email": "ger@test.com", "roleId": roleID(t, env.pool, "manager"), "password": "senha-ger-123",
	}, nil)
	manager := newClient(t, env.srv.URL)
	manager.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "ger@test.com", "password": "senha-ger-123"}, nil)
	if st, _, _ := manager.withKey("chave-teste-0000000001", "POST", "/suppliers", body); st != http.StatusUnprocessableEntity {
		t.Fatalf("other user's key: %d, want 422", st)
	}

	// --- API caiu entre gravar e responder: a gravação existe ---------------------
	if _, err := env.pool.Exec(context.Background(), `INSERT INTO idempotency_keys (key, user_id, method, path, request_hash, created_at)
		SELECT 'chave-teste-sem-resposta', user_id, method, path, request_hash, now() - interval '5 minutes'
		FROM idempotency_keys WHERE key = 'chave-teste-0000000001'`); err != nil {
		t.Fatal(err)
	}
	st, out, _ := admin.withKey("chave-teste-sem-resposta", "POST", "/suppliers", body)
	if st != http.StatusConflict || !bytes.Contains(out, []byte("already recorded")) {
		t.Fatalf("stuck key: %d %s", st, out)
	}

	// Login não entra (a resposta tem o cookie da sessão).
	if st, _, rep := admin.withKey("chave-teste-login-00001", "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}); st != http.StatusOK || rep {
		t.Fatalf("login with key: %d replayed=%v", st, rep)
	}
	if n := countRows(t, env, "SELECT count(*) FROM idempotency_keys WHERE key = 'chave-teste-login-00001'"); n != 0 {
		t.Fatal("login stored an idempotency key")
	}
}
