// Package postgres cuida da conexão com o banco e das migrations.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// A diretiva //go:embed copia os arquivos .sql para DENTRO do binário na compilação,
// então o executável final não precisa da pasta migrations no servidor.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect abre um pool de conexões (as conexões são reaproveitadas entre requisições).
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		// %w "embrulha" o erro original: quem chamar pode inspecioná-lo com errors.Is/As.
		return nil, fmt.Errorf("creating pool: %w", err)
	}

	// pgxpool.New não conecta de fato; o Ping garante que o banco está acessível.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel() // defer roda quando a função retornar, aconteça o que acontecer.

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return pool, nil
}

// Migrate aplica as migrations pendentes (o goose registra as já aplicadas
// na tabela goose_db_version).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	// O goose trabalha com database/sql; o pacote stdlib do pgx faz essa ponte.
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	return nil
}
