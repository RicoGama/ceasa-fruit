package realtime

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

func TestRoute(t *testing.T) {
	admin := auth.RoleKeyAdmin
	seller := &auth.Session{UserID: 7, RoleID: 3, Permissions: []auth.Permission{auth.SuppliersView}}
	boss := &auth.Session{UserID: 1, RoleID: 1, RoleKey: &admin}

	cases := []struct {
		name string
		s    *auth.Session
		ev   Event
		want []Event
	}{
		{"has permission", seller, Event{"suppliers", 5}, []Event{{"suppliers", 5}}},
		{"no permission", seller, Event{"products", 5}, nil},
		{"company goes to everyone", seller, Event{"company", 1}, []Event{{"company", 1}}},
		{"resync goes to everyone", seller, Event{Topic: "resync"}, []Event{{Topic: "resync"}}},
		// mudanças no próprio usuário ou papel viram "me", mesmo sem users.manage
		{"own user changed", seller, Event{"users", 7}, []Event{{"me", 7}}},
		{"own role changed", seller, Event{"roles", 3}, []Event{{"me", 7}}},
		{"other user changed", seller, Event{"users", 8}, nil},
		{"admin sees users and me", boss, Event{"users", 1}, []Event{{"users", 1}, {"me", 1}}},
	}
	for _, c := range cases {
		if got := route(c.s, c.ev); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// --- Com o banco de verdade --------------------------------------------------------

func setup(t *testing.T) (*Hub, postgres.DB, context.Context) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	hub := New(pool.Config().ConnConfig, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go hub.Run(ctx)
	return hub, postgres.NewDB(pool), ctx
}

// next espera o próximo aviso que não seja "resync" (ou nada, até o limite).
func next(ch <-chan Event, within time.Duration) (Event, bool) {
	deadline := time.After(within)
	for {
		select {
		case ev := <-ch:
			if ev.Topic != "resync" {
				return ev, true
			}
		case <-deadline:
			return Event{}, false
		}
	}
}

func TestNotifyAfterCommitOnly(t *testing.T) {
	hub, database, ctx := setup(t)
	events, cancel := hub.subscribe()
	defer cancel()
	time.Sleep(300 * time.Millisecond) // o Hub conecta e começa a ouvir

	insert := func(tx postgres.Tx, name string) (int64, error) {
		var id int64
		err := tx.Raw.QueryRow(ctx, `INSERT INTO products (name, category, stock_unit)
			VALUES ($1, 'fruit', 'box') RETURNING id`, name).Scan(&id)
		return id, err
	}
	name := "Teste realtime " + time.Now().Format("150405.000000")
	// O histórico de alterações (audit_log) também avisa a cada gravação:
	// aqui interessa só o aviso do produto.
	nextChange := func(d time.Duration) (Event, bool) {
		deadline := time.Now().Add(d)
		for {
			ev, ok := next(events, time.Until(deadline))
			if !ok || ev.Topic != "audit_log" {
				return ev, ok
			}
		}
	}

	// 1. ROLLBACK: ninguém é avisado de algo que não aconteceu.
	_ = database.InTx(ctx, func(tx postgres.Tx) error {
		if _, err := insert(tx, name+" desfeito"); err != nil {
			return err
		}
		return pgx.ErrTxClosed // força o ROLLBACK
	})
	if ev, got := nextChange(700 * time.Millisecond); got {
		t.Fatalf("rolled back insert produced %v", ev)
	}

	// 2. COMMIT: o aviso chega, e só depois do COMMIT. Produto + 3 embalagens
	//    na mesma transação viram UM aviso (o Postgres junta os iguais).
	var id int64
	committed := make(chan struct{})
	go func() {
		_ = database.InTx(ctx, func(tx postgres.Tx) error {
			var err error
			if id, err = insert(tx, name); err != nil {
				return err
			}
			for _, p := range []string{"Pilha", "Pallet", "Meia pilha"} {
				if _, err := tx.Raw.Exec(ctx, "INSERT INTO product_packagings (product_id, name) VALUES ($1, $2)", id, p); err != nil {
					return err
				}
			}
			if ev, got := nextChange(500 * time.Millisecond); got {
				t.Errorf("event before commit: %v", ev)
			}
			return nil
		})
		close(committed)
	}()
	<-committed
	ev, got := nextChange(3 * time.Second)
	if !got || ev != (Event{"products", id}) {
		t.Fatalf("after commit: got %v (%v), want products/%d", ev, got, id)
	}
	if extra, got := nextChange(500 * time.Millisecond); got {
		t.Errorf("expected one event per transaction, also got %v", extra)
	}
	_ = database.InTx(ctx, func(tx postgres.Tx) error {
		_, err := tx.Raw.Exec(ctx, "DELETE FROM products WHERE id = $1", id)
		return err
	})
}

// O caminho completo pelo HTTP: GET /events recebe "data: {...}".
func TestServeSSE(t *testing.T) {
	hub, database, ctx := setup(t)
	s := &auth.Session{UserID: 1, Permissions: []auth.Permission{auth.ProductsView}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), s)))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}
	time.Sleep(300 * time.Millisecond)

	var id int64
	err = database.InTx(ctx, func(tx postgres.Tx) error {
		return tx.Raw.QueryRow(ctx, `INSERT INTO products (name, category, stock_unit)
			VALUES ($1, 'fruit', 'box') RETURNING id`, "SSE "+time.Now().Format("150405.000000")).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = database.InTx(ctx, func(tx postgres.Tx) error {
			_, err := tx.Raw.Exec(ctx, "DELETE FROM products WHERE id = $1", id)
			return err
		})
	}()

	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case line := <-lines:
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok {
				continue
			}
			var ev Event
			_ = json.Unmarshal([]byte(data), &ev)
			if ev == (Event{"products", id}) {
				return // chegou
			}
		case <-deadline:
			t.Fatal("no SSE event for the new product")
		}
	}
}
