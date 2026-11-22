// Command seed prepara a instalação: cria a empresa (se ainda não existir) e
// cria ou atualiza um usuário administrador.
// Pode rodar quantas vezes quiser: se o usuário já existe, troca a senha e
// garante que ele é administrador e está ativo.
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
	company := flag.String("company", "Irmãos Faisão Frutícola", "company name (only used if the company does not exist yet)")
	password := flag.String("password", "", "password (random when empty)")
	flag.Parse()

	if err := run(*email, *name, *company, *password); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run(email, name, companyName, password string) error {
	ctx := context.Background()
	email = strings.ToLower(strings.TrimSpace(email))
	if password == "" {
		password = rand.Text()[:16] // 16 caracteres aleatórios
	}
	if len(password) < 8 {
		return errors.New("a senha precisa ter pelo menos 8 caracteres")
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
	err = postgres.NewDB(pool).InTx(ctx, func(q postgres.Tx) error {

		if err := q.EnsureCompany(ctx, companyName); err != nil {
			return err
		}

		adminKey := auth.RoleKeyAdmin
		adminRole, err := q.GetRoleIDByKey(ctx, &adminKey)
		if err != nil {
			return fmt.Errorf("papel de administrador não encontrado (migration 00006): %w", err)
		}
		user, err := q.GetUserByEmail(ctx, email)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			_, err = q.CreateUser(ctx, db.CreateUserParams{Name: name, Email: email, PasswordHash: hash, RoleID: adminRole})
			created = true
			return err
		case err != nil:
			return err
		}
		if err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: user.ID, PasswordHash: hash}); err != nil {
			return err
		}
		_, err = q.UpdateUser(ctx, db.UpdateUserParams{ID: user.ID, Name: user.Name, RoleID: adminRole, Active: true, Version: user.Version})
		return err
	})
	if err != nil {
		return err
	}

	// Mensagens em português: aparecem para quem instala o servidor.
	action := "atualizado"
	if created {
		action = "criado"
	}
	fmt.Printf("Administrador %s.\n\n  E-mail: %s\n  Senha:  %s\n\n", action, email, password)
	return nil
}
