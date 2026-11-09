// Command api sobe a API HTTP do CEASA Fruit.
//
// Convenção: executáveis ficam em cmd/<nome>/main.go e o resto do código em
// internal/ (pacotes dentro de internal/ não podem ser importados por outros módulos).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RicoGama/ceasa-fruit/api/internal/config"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// O main fica pequeno: o trabalho de verdade acontece em run, que devolve um erro.
	// Assim o tratamento de erro fica uniforme (sem os.Exit espalhado pelo código).
	if err := run(logger); err != nil {
		logger.Error("api stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := config.Load()

	// ctx é cancelado quando o processo recebe Ctrl+C (SIGINT) ou SIGTERM
	// (o que o Docker/Kubernetes enviam ao desligar).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}
	logger.Info("database ready")

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: server.New(server.Deps{
			Logger:      logger,
			DB:          pool,
			Pinger:      pool,
			CORSOrigins: cfg.CORSOrigins,
		}),
		// Timeouts protegem o servidor de clientes lentos ou travados.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// ListenAndServe bloqueia, então roda numa goroutine (uma "thread" leve).
	// Os erros voltam por um channel.
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr, "docs", "http://localhost:"+cfg.Port+"/docs")
		errCh <- srv.ListenAndServe()
	}()

	// select espera o que acontecer primeiro.
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutting down...")
	}

	// Graceful shutdown: para de aceitar novas requisições e espera até 10s
	// para as que estão em andamento terminarem.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
