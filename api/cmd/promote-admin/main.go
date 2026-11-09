// Command promote-admin transforma um usuário já cadastrado em administrador
// da plataforma. Não existe rota HTTP para isso de propósito: só quem tem
// acesso ao servidor/banco consegue criar um admin.
//
// Uso: go run ./cmd/promote-admin -email voce@exemplo.com
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/RicoGama/ceasa-fruit/api/internal/config"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

func main() {
	// O pacote flag lê argumentos de linha de comando (-email ...).
	email := flag.String("email", "", "email of the user to promote")
	flag.Parse()

	if *email == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*email); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(email string) error {
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, config.Load().DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	n, err := db.New(pool).PromotePlatformAdmin(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no user with email %q (sign up first)", email)
	}
	fmt.Printf("%s is now a platform admin\n", email)
	return nil
}
