package user

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// TestLastAdminRace força, de forma determinística, a corrida que deixaria o
// sistema sem administrador:
//
//	A: rebaixa o admin B ......................... (segura o COMMIT)
//	B:                    rebaixa o admin A ...... (roda no meio)
//	A:                                       COMMIT
//
// Sem trava, B ainda enxerga A como admin (A não gravou), passa na checagem,
// e as duas gravam: zero administradores. Com a trava, B espera A terminar,
// vê que só sobrou ele e é recusada.
func TestLastAdminRace(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	database := postgres.NewDB(pool)

	if _, err := pool.Exec(ctx, "TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	var adminRole, managerRole int64
	err = database.InReadTx(ctx, func(tx postgres.Tx) error {
		admin, manager := auth.RoleKeyAdmin, "manager"
		var err error
		if adminRole, err = tx.GetRoleIDByKey(ctx, &admin); err != nil {
			return err
		}
		managerRole, err = tx.GetRoleIDByKey(ctx, &manager)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var adminA, adminB db.User
	err = database.InTx(ctx, func(tx postgres.Tx) error {
		var err error
		if adminA, err = tx.CreateUser(ctx, db.CreateUserParams{Name: "Admin A", Email: "a@test.com", PasswordHash: "x", RoleID: adminRole}); err != nil {
			return err
		}
		adminB, err = tx.CreateUser(ctx, db.CreateUserParams{Name: "Admin B", Email: "b@test.com", PasswordHash: "x", RoleID: adminRole})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	applied := make(chan struct{}) // A já alterou, mas não gravou
	release := make(chan struct{}) // libera o COMMIT de A
	doneA := make(chan error, 1)
	go func() {
		doneA <- database.InTx(ctx, func(tx postgres.Tx) error {
			if err := applyUpdate(ctx, tx, adminB.ID, adminB.Version, adminB.Name, managerRole, true); err != nil {
				return err
			}
			close(applied)
			<-release
			return nil // COMMIT
		})
	}()

	<-applied
	doneB := make(chan error, 1)
	go func() {
		doneB <- database.InTx(ctx, func(tx postgres.Tx) error {
			return applyUpdate(ctx, tx, adminA.ID, adminA.Version, adminA.Name, managerRole, true)
		})
	}()
	time.Sleep(300 * time.Millisecond) // dá tempo de B chegar na trava (se houver)
	close(release)

	if err := <-doneA; err != nil {
		t.Fatalf("A: %v", err)
	}
	errB := <-doneB

	var n int64
	_ = database.InReadTx(ctx, func(tx postgres.Tx) error {
		n, err = tx.CountActiveAdmins(ctx)
		return err
	})
	if n < 1 {
		t.Fatalf("no active admin left (B returned %v)", errB)
	}
	if !errors.Is(errB, errLastAdmin) {
		t.Fatalf("B: err = %v, want errLastAdmin", errB)
	}
}
