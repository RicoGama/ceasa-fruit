package postgres

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

// tenantRole é o papel do Postgres sujeito ao RLS (criado na migration 00002).
const tenantRole = "ceasa_app"

// InTenant roda fn numa transação "presa" a uma organização, entregando as
// queries do sqlc. É o que as rotas de negócio usam.
func InTenant(ctx context.Context, pool *pgxpool.Pool, organizationID int64, fn func(q *db.Queries) error) error {
	return InTenantTx(ctx, pool, organizationID, func(tx pgx.Tx) error {
		return fn(db.New(tx))
	})
}

// InTenantTx é a versão que entrega a transação crua (útil para testes e SQL
// fora do sqlc). Dentro dela:
//
//  1. SET LOCAL ROLE ceasa_app -> a partir daqui o RLS vale para esta transação
//  2. app.organization_id = id -> as policies só enxergam linhas dessa organização
//
// O LOCAL faz as duas configurações valerem só até o fim da transação, então
// a conexão volta limpa para o pool e não "vaza" para a próxima requisição.
func InTenantTx(ctx context.Context, pool *pgxpool.Pool, organizationID int64, fn func(tx pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+tenantRole); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)",
			strconv.FormatInt(organizationID, 10))
		if err != nil {
			return err
		}
		return fn(tx)
	})
}
