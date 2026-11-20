package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

func connect(t *testing.T) postgres.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// Uma tabelinha só para o teste (apagada no fim).
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS tx_test_rows;
		CREATE TABLE tx_test_rows (id INT PRIMARY KEY, n INT NOT NULL);
		INSERT INTO tx_test_rows VALUES (1, 0), (2, 0);`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP TABLE tx_test_rows") })
	return postgres.NewDB(pool)
}

// Duas transações travam as mesmas duas linhas em ordem contrária:
//
//	A: trava 1 ........ quer 2 (espera B)
//	B: ....... trava 2 ........ quer 1 (espera A)  -> deadlock
//
// O PostgreSQL derruba uma delas (40P01). O InTx repete a derrubada sozinho,
// e no fim as DUAS gravam.
func TestDeadlockIsRetried(t *testing.T) {
	database := connect(t)
	ctx := context.Background()

	lockedA, lockedB := make(chan struct{}), make(chan struct{})
	var attemptsA, attemptsB atomic.Int32
	run := func(first, second int, attempts *atomic.Int32, mine, other chan struct{}) error {
		return database.InTx(ctx, func(tx postgres.Tx) error {
			n := attempts.Add(1)
			if _, err := tx.Raw.Exec(ctx, "UPDATE tx_test_rows SET n = n + 1 WHERE id = $1", first); err != nil {
				return err
			}
			if n == 1 { // só na primeira tentativa forçamos o cruzamento
				close(mine)
				<-other
			}
			_, err := tx.Raw.Exec(ctx, "UPDATE tx_test_rows SET n = n + 1 WHERE id = $1", second)
			return err
		})
	}
	errA, errB := make(chan error, 1), make(chan error, 1)
	go func() { errA <- run(1, 2, &attemptsA, lockedA, lockedB) }()
	go func() { errB <- run(2, 1, &attemptsB, lockedB, lockedA) }()

	if err := <-errA; err != nil {
		t.Fatalf("A: %v", err)
	}
	if err := <-errB; err != nil {
		t.Fatalf("B: %v", err)
	}
	if attemptsA.Load()+attemptsB.Load() < 3 {
		t.Errorf("expected a retry (attempts A=%d B=%d)", attemptsA.Load(), attemptsB.Load())
	}
	var n1, n2 int
	_ = database.InReadTx(ctx, func(tx postgres.Tx) error {
		return tx.Raw.QueryRow(ctx, "SELECT (SELECT n FROM tx_test_rows WHERE id = 1), (SELECT n FROM tx_test_rows WHERE id = 2)").Scan(&n1, &n2)
	})
	if n1 != 2 || n2 != 2 {
		t.Errorf("rows = %d, %d; want 2, 2 (both transactions applied once)", n1, n2)
	}
}

// Uma transação segura a trava; a outra desiste depois do lock_timeout e a
// API responde 503 ("tente de novo"), em vez de a tela ficar carregando.
func TestLockTimeoutBecomesBusy(t *testing.T) {
	database := connect(t)
	ctx := context.Background()

	locked, release := make(chan struct{}), make(chan struct{})
	holder := make(chan error, 1)
	go func() {
		holder <- database.InTx(ctx, func(tx postgres.Tx) error {
			if _, err := tx.Raw.Exec(ctx, "SELECT * FROM tx_test_rows WHERE id = 1 FOR UPDATE"); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	start := time.Now()
	err := database.InTx(ctx, func(tx postgres.Tx) error {
		// Encurta a espera só nesta transação (o padrão é 5s).
		if _, err := tx.Raw.Exec(ctx, "SET LOCAL lock_timeout = '200ms'"); err != nil {
			return err
		}
		_, err := tx.Raw.Exec(ctx, "SELECT * FROM tx_test_rows WHERE id = 1 FOR UPDATE")
		return err
	})
	close(release)
	if err := <-holder; err != nil {
		t.Fatal(err)
	}

	var se huma.StatusError
	if !errors.As(err, &se) || se.GetStatus() != 503 {
		t.Fatalf("err = %v, want 503 busy", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("waited %s; lock_timeout not applied", time.Since(start))
	}
}
