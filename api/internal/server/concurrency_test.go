package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

// Testes de concorrência: várias requisições AO MESMO TEMPO contra o banco
// de verdade. Rode com -race (make test-integration) para pegar também
// disputas de memória dentro do Go.

// Dois administradores rebaixam um ao outro no mesmo instante. Sem trava,
// cada transação vê "o outro ainda é admin" e as duas passam: o sistema
// ficaria sem nenhum administrador. Com trava, uma delas tem que ser recusada.
func TestConcurrentLastAdmin(t *testing.T) {
	env := setupIntegrationEnv(t)
	ctx := context.Background()

	for round := range 15 {
		resetDatabase(t, env.pool)
		createAdmin(t, env.pool, "a@test.com", "senha-do-a-123")
		hash, _ := auth.HashPassword("senha-do-b-123")
		if _, err := db.New(env.pool).CreateUser(ctx, db.CreateUserParams{
			Name: "Admin B", Email: "b@test.com", PasswordHash: hash, RoleID: roleID(t, env.pool, auth.RoleKeyAdmin),
		}); err != nil {
			t.Fatal(err)
		}

		a, b := newClient(t, env.srv.URL), newClient(t, env.srv.URL)
		var meA, meB me
		a.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "a@test.com", "password": "senha-do-a-123"}, &meA)
		b.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "b@test.com", "password": "senha-do-b-123"}, &meB)
		var users struct {
			Items []struct {
				ID      int64 `json:"id"`
				Version int32 `json:"version"`
			} `json:"items"`
		}
		a.expect(http.StatusOK, "GET", "/users", nil, &users)
		version := map[int64]int32{}
		for _, u := range users.Items {
			version[u.ID] = u.Version
		}

		// A rebaixa B e B rebaixa A, disparados juntos.
		start := make(chan struct{})
		var wg sync.WaitGroup
		statuses := make([]int, 2)
		demote := func(i int, c *client, target me, name string) {
			defer wg.Done()
			<-start
			statuses[i] = c.do("PUT", fmt.Sprintf("/users/%d", target.User.ID),
				map[string]any{"name": name, "roleId": roleID(t, env.pool, "manager"), "active": true, "version": version[target.User.ID]}, nil)
		}
		wg.Add(2)
		go demote(0, a, meB, "Admin B")
		go demote(1, b, meA, "Admin A")
		close(start)
		wg.Wait()

		n, err := db.New(env.pool).CountActiveAdmins(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n < 1 {
			t.Fatalf("round %d: no active admin left (statuses %v)", round, statuses)
		}
	}
}

// Dez pessoas salvam o MESMO fornecedor no mesmo instante, todas com a versão
// que leram (1). Exatamente uma pode vencer; as outras recebem 409 e nenhuma
// alteração some sem aviso.
func TestConcurrentSupplierEdits(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	c := newClient(t, env.srv.URL)
	c.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	var created idResp
	c.expect(http.StatusCreated, "POST", "/suppliers", map[string]any{"personType": "company", "legalName": "Sítio Disputado"}, &created)

	const people = 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	statuses := make([]int, people)
	for i := range people {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			statuses[i] = c.do("PUT", fmt.Sprintf("/suppliers/%d", created.ID), map[string]any{
				"personType": "company", "legalName": fmt.Sprintf("Versão da pessoa %d", i), "version": 1,
			}, nil)
		}()
	}
	close(start)
	wg.Wait()

	wins, conflicts := 0, 0
	for _, st := range statuses {
		switch st {
		case http.StatusOK:
			wins++
		case http.StatusConflict:
			conflicts++
		default:
			t.Errorf("unexpected status %d", st)
		}
	}
	if wins != 1 || conflicts != people-1 {
		t.Fatalf("wins = %d, conflicts = %d (statuses %v)", wins, conflicts, statuses)
	}
	var got struct {
		Version int32 `json:"version"`
	}
	c.expect(http.StatusOK, "GET", fmt.Sprintf("/suppliers/%d", created.ID), nil, &got)
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
}
