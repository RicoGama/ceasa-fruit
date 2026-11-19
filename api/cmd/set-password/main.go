// Command set-password redefine a senha de um usuário direto no servidor,
// sem precisar de e-mail. As sessões abertas dele são encerradas.
//
// Uso: make set-password email=voce@exemplo.com
//
//	make set-password email=voce@exemplo.com password=nova-senha
//
// Sem -password, gera uma senha aleatória e mostra na tela.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/config"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

func main() {
	email := flag.String("email", "", "user email")
	password := flag.String("password", "", "new password (random when empty)")
	flag.Parse()
	if *email == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*email, *password); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(email, password string) error {
	ctx := context.Background()
	email = strings.ToLower(strings.TrimSpace(email))
	if password == "" {
		password = rand.Text()[:16]
	}
	if len(password) < 8 {
		return errors.New("password must have at least 8 characters")
	}

	pool, err := postgres.Connect(ctx, config.Load().DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		user, err := q.GetUserByEmail(ctx, email)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no user with email %q", email)
		}
		if err != nil {
			return err
		}
		if err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: user.ID, PasswordHash: hash}); err != nil {
			return err
		}
		if err := q.InvalidatePasswordResetTokens(ctx, user.ID); err != nil {
			return err
		}
		return q.DeleteUserSessions(ctx, user.ID)
	})
	if err != nil {
		return err
	}
	fmt.Printf("Password updated; open sessions were ended.\n\n  E-mail: %s\n  Senha:  %s\n\n", email, password)
	return nil
}
