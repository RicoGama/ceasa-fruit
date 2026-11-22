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

	"riverqueue.com/riverui"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/cnpj"
	"github.com/RicoGama/ceasa-fruit/api/internal/config"
	"github.com/RicoGama/ceasa-fruit/api/internal/fleet"
	"github.com/RicoGama/ceasa-fruit/api/internal/fleet/traccar"
	"github.com/RicoGama/ceasa-fruit/api/internal/mail"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/queue"
	"github.com/RicoGama/ceasa-fruit/api/internal/realtime"
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
	if err := queue.Migrate(ctx, pool); err != nil {
		return err
	}
	logger.Info("database ready")

	// Fila de jobs. Com WORKERS_ENABLED=true (padrão) este mesmo processo
	// executa os jobs; senão ele só enfileira e o cmd/worker executa.
	mailer, err := mail.NewSMTPMailer(cfg.SMTP)
	if err != nil {
		return err
	}
	q, err := queue.New(pool, queue.Options{Mailer: mailer, Logger: logger, RunWorkers: cfg.WorkersEnabled})
	if err != nil {
		return err
	}
	if cfg.WorkersEnabled {
		if err := q.Start(ctx); err != nil {
			return err
		}
		logger.Info("queue workers started", "smtp", cfg.SMTP.Host)
	}

	// Painel das filas (River UI), servido pela própria API em /admin/queues.
	queueUI, err := riverui.NewHandler(&riverui.HandlerOpts{
		Endpoints: riverui.NewEndpoints(q.Client, nil),
		Logger:    logger,
		Prefix:    "/admin/queues",
		// Os argumentos dos jobs podem ter dados sensíveis (ex.: o link de
		// redefinição de senha dentro do e-mail): ficam escondidos por padrão.
		JobListHideArgsByDefault: true,
	})
	if err != nil {
		return err
	}
	if err := queueUI.Start(ctx); err != nil {
		return err
	}

	// Tempo real: escuta as mudanças do banco e repassa às telas abertas.
	hub := realtime.New(pool.Config().ConnConfig, logger)
	database := postgres.NewDB(pool)
	hub.OnAlive = func(ctx context.Context, s *auth.Session) {
		if s.AccessLogID == nil {
			return
		}
		if err := auth.Touch(ctx, database, *s.AccessLogID); err != nil && ctx.Err() == nil {
			logger.Warn("touching session from realtime", "error", err)
		}
	}
	go hub.Run(ctx)

	// Frota: busca as posições dos caminhões no Traccar (se configurado).
	if cfg.Fleet.TraccarURL != "" {
		syncer := &fleet.Syncer{
			DB: database, Client: traccar.New(cfg.Fleet.TraccarURL, cfg.Fleet.TraccarToken), Logger: logger,
			Interval: cfg.Fleet.SyncInterval, Retention: time.Duration(cfg.Fleet.RetentionDays) * 24 * time.Hour,
		}
		go syncer.Run(ctx)
	} else {
		logger.Info("fleet: TRACCAR_URL not set, tracking disabled")
	}

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: server.New(server.Deps{
			Logger:            logger,
			Pool:              pool,
			CORSOrigins:       cfg.CORSOrigins,
			TrustedProxies:    cfg.TrustedProxies,
			CookieSecure:      cfg.CookieSecure,
			AppURL:            cfg.AppURL,
			Queue:             q,
			QueueUI:           queueUI,
			Realtime:          hub,
			TraccarConfigured: cfg.Fleet.TraccarURL != "",
			CNPJProviders: []cnpj.Provider{
				cnpj.BrasilAPI{BaseURL: cfg.CNPJBrasilAPIURL}, cnpj.CNPJa{BaseURL: cfg.CNPJCNPJaURL},
			},
		}),
		// Timeouts protegem o servidor de clientes lentos ou travados.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// As conexões de tempo real nunca terminam sozinhas: no desligamento o Hub
	// as encerra, senão o Shutdown esperaria por elas até o limite.
	srv.RegisterOnShutdown(hub.Close)

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
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	// Depois do HTTP, a fila: espera os jobs em andamento terminarem.
	if cfg.WorkersEnabled {
		return q.Stop(shutdownCtx)
	}
	return nil
}
