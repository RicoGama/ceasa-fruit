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
		fmt.Fprintln(os.Stderr, "erro:", err)
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
		return errors.New("a senha precisa ter pelo menos 8 caracteres")
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

	err = postgres.NewDB(pool).InTx(ctx, func(q postgres.Tx) error {
		user, err := q.GetUserByEmail(ctx, email)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("nenhum usuário com o e-mail %q", email)
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
		reason := "password_changed"
		return q.EndUserSessions(ctx, db.EndUserSessionsParams{UserID: user.ID, Reason: &reason})
	})
	if err != nil {
		return err
	}
	fmt.Printf("Senha trocada; as sessões abertas foram encerradas.\n\n  E-mail: %s\n  Senha:  %s\n\n", email, password)
	return nil
}
