// Command seed prepara um ambiente de desenvolvimento: cria (ou atualiza) um
// usuário administrador da plataforma, com uma organização onde ele é dono.
// Pode rodar quantas vezes quiser: se o usuário já existe, só troca a senha.
//
// Uso: make seed
//
//	make seed email=voce@exemplo.com password=minha-senha
//
// Sem -password, gera uma senha aleatória e mostra na tela (nada de senha
// fixa conhecida, que costuma acabar indo para produção).
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
	"github.com/RicoGama/ceasa-fruit/api/internal/queue"
)

func main() {
	email := flag.String("email", "admin@ceasafruit.test", "admin email")
	name := flag.String("name", "Administrador", "admin name")
	org := flag.String("org", "Irmãos Faisão Frutícola", "organization created when the admin has none")
	password := flag.String("password", "", "password (random when empty)")
	flag.Parse()

	if err := run(*email, *name, *org, *password); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(email, name, orgName, password string) error {
	ctx := context.Background()
	email = strings.ToLower(strings.TrimSpace(email))
	if password == "" {
		password = rand.Text()[:16] // 16 caracteres aleatórios
	}
	if len(password) < 8 {
		return errors.New("password must have at least 8 characters")
	}

	pool, err := postgres.Connect(ctx, config.Load().DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Garante as tabelas, caso o banco seja novo e a API nunca tenha subido.
	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}
	if err := queue.Migrate(ctx, pool); err != nil {
		return err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	created := false
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)

		user, err := q.GetUserByEmail(ctx, email)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			user, err = q.CreateUser(ctx, db.CreateUserParams{Name: name, Email: email, PasswordHash: hash})
			if err != nil {
				return err
			}
			created = true
		case err != nil:
			return err
		default:
			if err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: user.ID, PasswordHash: hash}); err != nil {
				return err
			}
		}

		if _, err := q.PromotePlatformAdmin(ctx, email); err != nil {
			return err
		}

		// Sem nenhuma organização, cria uma com ele como dono.
		orgs, err := q.ListUserOrganizations(ctx, user.ID)
		if err != nil {
			return err
		}
		if len(orgs) == 0 {
			o, err := q.CreateOrganization(ctx, orgName)
			if err != nil {
				return err
			}
			return q.CreateMembership(ctx, db.CreateMembershipParams{OrganizationID: o.ID, UserID: user.ID, Role: "owner"})
		}
		return nil
	})
	if err != nil {
		return err
	}

	action := "updated"
	if created {
		action = "created"
	}
	fmt.Printf("Platform admin %s.\n\n  E-mail: %s\n  Senha:  %s\n\n", action, email, password)
	return nil
}
