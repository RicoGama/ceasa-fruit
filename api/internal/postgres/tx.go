package postgres

import (
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
// Concorrência:
//   - disputa pela mesma linha (dois vendedores na mesma caixa, o próximo
//     número da nota): use SELECT ... FOR UPDATE; a segunda transação ESPERA
//     a primeira terminar e então enxerga o valor novo;
//   - edição de cadastro: concorrência otimista com a coluna version
//     (ver migration 00004 e StaleError);
//   - deadlock ou falha de serialização: a transação inteira é repetida
//     sozinha (até 3 tentativas). Por isso fn pode rodar MAIS DE UMA VEZ:
//     dentro dela, só mexa no banco. Nada de chamar SEFAZ, banco ou enviar
//     e-mail direto; isso vai para a fila (River), que entra na transação.
func (d DB) InTx(ctx context.Context, fn func(Tx) error) error {
	return d.run(ctx, pgx.TxOptions{}, fn)
}

// InReadTx roda fn numa transação SÓ DE LEITURA, com uma "foto" única do banco
// (REPEATABLE READ): todas as consultas dentro dela enxergam o mesmo instante,
// mesmo que outra pessoa grave algo no meio. Ideal para telas e relatórios que
// fazem várias consultas (ex.: fornecedor + contatos, totais do dia).
// Uma escrita aqui dentro é recusada pelo próprio PostgreSQL.
func (d DB) InReadTx(ctx context.Context, fn func(Tx) error) error {
	return d.run(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, fn)
}

// actorKey guarda no context quem está fazendo a requisição.
type actorKey struct{}

// WithActor marca o context com o usuário logado (feito pelo middleware de
// sessão). As transações abertas com esse context registram o usuário no
// histórico de alterações.
func WithActor(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, actorKey{}, userID)
}

// ActorFrom devolve o usuário logado guardado por WithActor.
func ActorFrom(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(actorKey{}).(int64)
	return id, ok
}

// maxAttempts: quantas vezes uma transação é tentada em caso de deadlock.
const maxAttempts = 3

func (d DB) run(ctx context.Context, opts pgx.TxOptions, fn func(Tx) error) error {
	var err error
	actor, hasActor := ActorFrom(ctx)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = pgx.BeginTxFunc(ctx, d.pool, opts, func(tx pgx.Tx) error {
			// Quem está gravando: o gatilho do histórico (audit_row, migration
			// 00013) lê daqui. "true" = só vale nesta transação.
			if hasActor && opts.AccessMode != pgx.ReadOnly {
				if _, err := tx.Exec(ctx, "SELECT set_config('app.actor_id', $1, true)", strconv.FormatInt(actor, 10)); err != nil {
					return err
				}
			}
			return fn(Tx{Queries: db.New(tx), Raw: tx})
		})
		if !isRetryable(err) || attempt == maxAttempts {
			break
		}
		// Espera um pouco (com um sorteio, para as duas transações que
		// brigaram não voltarem exatamente juntas) e tenta de novo.
		wait := time.Duration(attempt*20+rand.IntN(30)) * time.Millisecond
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if isLockTimeout(err) {
		return BusyError()
	}
	return err
}

// Códigos de erro do PostgreSQL ligados a concorrência.
const (
	codeSerializationFailure = "40001" // a transação enxergou dados que mudaram
	codeDeadlock             = "40P01" // duas transações esperando uma pela outra
	codeLockNotAvailable     = "55P03" // passou do lock_timeout esperando uma trava
)

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func isRetryable(err error) bool {
	c := pgCode(err)
	return c == codeSerializationFailure || c == codeDeadlock
}

func isLockTimeout(err error) bool { return pgCode(err) == codeLockNotAvailable }

// StaleError é o erro de concorrência otimista: o registro foi salvo por outra
// pessoa depois que a tela o leu (a versão não bate). Vira 409 na API, e o
// front oferece recarregar os dados.
func StaleError() error {
	return huma.Error409Conflict("this record was changed by someone else")
}

// BusyError: a trava de um registro demorou demais (lock_timeout). Vira 503,
// e o front pede para tentar de novo em instantes.
func BusyError() error {
	return huma.Error503ServiceUnavailable("the record is busy, try again")
}
