package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

// REGRA DO PROJETO: todo acesso ao banco acontece dentro de uma transação.
//
// Por isso os handlers não recebem o pool nem as queries "soltas": recebem um
// DB, e o único jeito de chegar nas queries é por InTx ou InReadTx. Assim a
// regra não depende de lembrar dela: o código sem transação não compila.
// (O teste rules_test.go confere que ninguém fura isso por outro caminho.)
//
// Ganhos:
//   - tudo ou nada: se um passo falha, o ROLLBACK desfaz os anteriores;
//   - jobs da fila (River) entram na MESMA transação dos dados;
//   - leituras com várias consultas veem o banco num único instante.

// DB dá acesso ao banco, sempre por transação.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB embrulha o pool de conexões.
func NewDB(pool *pgxpool.Pool) DB { return DB{pool: pool} }

// Tx é o que a função recebe dentro da transação:
//   - as queries do sqlc, já presas à transação (tx.GetUser(...));
//   - Raw, a transação "crua", para quem precisa dela (ex.: a fila enfileirar
//     um e-mail junto: emails.EnqueueEmail(ctx, tx.Raw, msg)).
//
// *db.Queries está "embutido" (embedding): os métodos dele aparecem direto em Tx.
type Tx struct {
	*db.Queries
	Raw pgx.Tx
}

// InTx roda fn numa transação de leitura e escrita (READ COMMITTED, o padrão
// do PostgreSQL). Se fn devolver erro (ou entrar em pânico), tudo é desfeito
// com ROLLBACK; se devolver nil, é gravado com COMMIT.
//
// Para disputa por uma mesma linha (dois vendedores na mesma caixa, o próximo
// número da nota), use SELECT ... FOR UPDATE na query: a segunda transação
// espera a primeira terminar.
func (d DB) InTx(ctx context.Context, fn func(Tx) error) error {
	return pgx.BeginFunc(ctx, d.pool, func(tx pgx.Tx) error {
		return fn(Tx{Queries: db.New(tx), Raw: tx})
	})
}

// InReadTx roda fn numa transação SÓ DE LEITURA, com uma "foto" única do banco
// (REPEATABLE READ): todas as consultas dentro dela enxergam o mesmo instante,
// mesmo que outra pessoa grave algo no meio. Ideal para telas e relatórios que
// fazem várias consultas (ex.: fornecedor + contatos, totais do dia).
// Uma escrita aqui dentro é recusada pelo próprio PostgreSQL.
func (d DB) InReadTx(ctx context.Context, fn func(Tx) error) error {
	opts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	return pgx.BeginTxFunc(ctx, d.pool, opts, func(tx pgx.Tx) error {
		return fn(Tx{Queries: db.New(tx), Raw: tx})
	})
}
