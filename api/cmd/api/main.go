// Command api starts the SEASA Fruit HTTP API.
//
// Convention: executables live in cmd/<name>/main.go and the rest of the code
// in internal/ (packages under internal/ cannot be imported by other modules).
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

	"github.com/RicoGama/seasa-fruit/api/internal/config"
	"github.com/RicoGama/seasa-fruit/api/internal/postgres"
	"github.com/RicoGama/seasa-fruit/api/internal/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// main stays tiny: the real work happens in run, which returns an error.
	// That makes error handling uniform (no os.Exit scattered around).
	if err := run(logger); err != nil {
		logger.Error("api stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := config.Load()

	// ctx is canceled when the process receives Ctrl+C (SIGINT) or SIGTERM
	// (what Docker/Kubernetes send on shutdown).
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
		// Timeouts protect the server from slow or stuck clients.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// ListenAndServe blocks, so it runs in a goroutine (a lightweight thread).
	// Errors come back through a channel.
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr, "docs", "http://localhost:"+cfg.Port+"/docs")
		errCh <- srv.ListenAndServe()
	}()

	// select waits for whichever happens first.
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutting down...")
	}

	// Graceful shutdown: stop accepting new requests and wait up to 10s
	// for the in-flight ones to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
