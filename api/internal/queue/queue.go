// Package queue é a fila de tarefas em segundo plano (o equivalente às Jobs
// do Laravel), feita com o River (https://riverqueue.com).
//
// O River guarda os jobs no próprio PostgreSQL (tabela river_job). Isso
// permite enfileirar DENTRO da mesma transação dos dados de negócio: se a
// transação falhar, o job some junto; se gravar, o job com certeza existe.
//
// Vocabulário (comparando com o Laravel):
//   - JobArgs  (ex.: EmailArgs)   ~ a classe Job, com os dados da tarefa
//   - Worker   (ex.: EmailWorker) ~ o método handle(), que executa a tarefa
//   - Insert/InsertTx             ~ dispatch()
//   - Client.Start                ~ php artisan queue:work
package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/RicoGama/ceasa-fruit/api/internal/mail"
)

// Nomes das filas. Cada fila tem seu próprio limite de workers: um pico de
// e-mails não atrasa as outras tarefas.
const (
	QueueEmail = "email"
)

// Migrate cria/atualiza as tabelas do River (river_job, river_queue...).
// Elas são do River, por isso ficam fora das nossas migrations do goose.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("migrating river: %w", err)
	}
	// Permite enfileirar de dentro de uma transação de organização
	// (postgres.InTenant troca para o papel ceasa_app, sujeito ao RLS).
	_, err = pool.Exec(ctx, `
		GRANT SELECT, INSERT, UPDATE ON river_job TO ceasa_app;
		GRANT USAGE ON SEQUENCE river_job_id_seq TO ceasa_app;
		GRANT SELECT, INSERT, UPDATE ON river_queue TO ceasa_app;`)
	return err
}

// ---------------------------------------------------------------------------
// Job: enviar e-mail
// ---------------------------------------------------------------------------

// EmailArgs é o job "enviar este e-mail". A mensagem já vai pronta (assunto,
// HTML e texto): se precisar tentar de novo, o conteúdo é exatamente o mesmo.
type EmailArgs struct {
	Message mail.Message `json:"message"`
}

// Kind é o nome do tipo de job, gravado no banco. Não mude depois de usar:
// jobs antigos na fila deixariam de ter quem os execute.
func (EmailArgs) Kind() string { return "send_email" }

// InsertOpts define os padrões deste tipo de job.
func (EmailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueEmail,
		// Se o servidor SMTP estiver fora do ar, tenta de novo com espera
		// crescente (segundos, minutos, horas...) até 10 vezes.
		MaxAttempts: 10,
	}
}

// EmailWorker executa os jobs EmailArgs.
type EmailWorker struct {
	// WorkerDefaults dá implementações padrão para os métodos opcionais.
	river.WorkerDefaults[EmailArgs]
	Mailer mail.Mailer
}

// Work envia o e-mail. Devolver erro = o River agenda uma nova tentativa.
func (w *EmailWorker) Work(ctx context.Context, job *river.Job[EmailArgs]) error {
	return w.Mailer.Send(ctx, job.Args.Message)
}

// Timeout: um envio travado é cancelado depois de 1 minuto (e tentado de novo).
func (w *EmailWorker) Timeout(*river.Job[EmailArgs]) time.Duration { return time.Minute }

// ---------------------------------------------------------------------------
// Fila
// ---------------------------------------------------------------------------

// Queue embrulha o cliente do River com as operações que o sistema usa.
type Queue struct {
	Client *river.Client[pgx.Tx]
}

// Options configura a fila.
type Options struct {
	Mailer mail.Mailer
	Logger *slog.Logger
	// RunWorkers: true = este processo também executa os jobs. Com false, o
	// processo só enfileira (ex.: API escalada separada dos workers).
	RunWorkers bool
}

// New cria a fila. Os workers só começam a trabalhar depois de Start.
func New(pool *pgxpool.Pool, opts Options) (*Queue, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &EmailWorker{Mailer: opts.Mailer})

	cfg := &river.Config{
		Logger:  opts.Logger,
		Workers: workers,
		// Jobs concluídos ficam 24h visíveis no painel e depois são apagados.
		CompletedJobRetentionPeriod: 24 * time.Hour,
	}
	if opts.RunWorkers {
		cfg.Queues = map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
			QueueEmail:         {MaxWorkers: 5},
		}
	}

	client, err := river.NewClient(riverpgxv5.New(pool), cfg)
	if err != nil {
		return nil, err
	}
	return &Queue{Client: client}, nil
}

// Start liga os workers (se configurados) em segundo plano.
func (q *Queue) Start(ctx context.Context) error { return q.Client.Start(ctx) }

// Stop espera os jobs em andamento terminarem (até o ctx expirar) e para.
func (q *Queue) Stop(ctx context.Context) error { return q.Client.Stop(ctx) }

// EnqueueEmail coloca um e-mail na fila DENTRO da transação tx: ele só passa
// a existir se a transação fizer COMMIT.
func (q *Queue) EnqueueEmail(ctx context.Context, tx pgx.Tx, msg mail.Message) error {
	_, err := q.Client.InsertTx(ctx, tx, EmailArgs{Message: msg}, nil)
	return err
}
