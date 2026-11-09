// Command worker executa só os jobs da fila, sem servidor HTTP.
//
// Em desenvolvimento não precisa dele: a API já executa os jobs
// (WORKERS_ENABLED=true). Em produção, dá para rodar a API com
// WORKERS_ENABLED=false e um ou mais workers separados, escalando cada parte
// conforme a necessidade.
//
// Uso: go run ./cmd/worker   (ou: make worker)
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RicoGama/ceasa-fruit/api/internal/config"
	"github.com/RicoGama/ceasa-fruit/api/internal/mail"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/queue"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("worker stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	mailer, err := mail.NewSMTPMailer(cfg.SMTP)
	if err != nil {
		return err
	}
	q, err := queue.New(pool, queue.Options{Mailer: mailer, Logger: logger, RunWorkers: true})
	if err != nil {
		return err
	}
	if err := q.Start(ctx); err != nil {
		return err
	}
	logger.Info("worker running; Ctrl+C to stop", "smtp", cfg.SMTP.Host)

	<-ctx.Done() // espera o sinal de desligar

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return q.Stop(shutdownCtx)
}
