// Command worker runs NithiDesk background jobs.
//
// The queue is PostgreSQL-backed so the local stack stays at one datastore
// (see NITHIDESK_PLANNING_SUMMARY.md §11). Job handlers are registered by kind in the
// Mux below.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/briefs"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/hearings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/observability"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/objectstore"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("worker stopped",
			slog.String("service", "worker"),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := observability.New(os.Stdout, observability.Options{
		Level:   slog.LevelInfo,
		Service: "worker",
		Env:     cfg.Env,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	store, err := objectstore.OpenBlob(cfg.ObjectStore)
	if err != nil {
		return err
	}
	briefSvc := briefs.New(pool, hearings.New(pool), store, cfg.PDF, logger)
	jobs := worker.Mux{
		"brief.render": briefSvc.RenderJob,
	}

	runner := worker.New(worker.Options{
		Logger:  logger,
		Queue:   worker.NewPostgresQueue(pool),
		Config:  cfg.Worker,
		Handler: jobs.Handle,
	})

	logger.Info("starting",
		slog.String("interval", cfg.Worker.PollInterval.String()),
		slog.Int("batch_size", cfg.Worker.BatchSize),
	)
	runner.Run(ctx)
	logger.Info("stopped")
	return nil
}
