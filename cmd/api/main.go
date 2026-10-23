// Command api runs the NithiDesk HTTP API.
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

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/briefs"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/courts"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/hearings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/httpapi"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/identity"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/imports"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/matters"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/observability"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/objectstore"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/secretbox"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/proceedings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/registry"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet, so write the failure plainly and exit
		// non-zero. Configuration errors never contain secret values.
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("api stopped",
			slog.String("service", "api"),
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
		Service: "api",
		Env:     cfg.Env,
	})
	logger.Info("starting", attrsFrom(cfg)...)

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
	box, err := secretbox.New(cfg.SecretsKey)
	if err != nil {
		return err
	}
	hearingSvc := hearings.New(pool)
	services := httpapi.Services{
		Pool:        pool,
		Identity:    identity.New(pool, box, cfg.Session, cfg.DefaultFirm),
		Courts:      courts.New(pool),
		Registry:    registry.New(pool),
		Matters:     matters.New(pool),
		Proceedings: proceedings.New(pool),
		Hearings:    hearingSvc,
		Imports:     imports.New(pool),
		Briefs:      briefs.New(pool, hearingSvc, store, cfg.PDF, logger),
	}

	router := httpapi.NewRouter(httpapi.Options{
		Logger:  logger,
		Version: version,
		Checkers: []httpapi.Checker{
			httpapi.CheckerFunc{
				DependencyName: "database",
				Fn: func(ctx context.Context) error {
					return db.Check(ctx, pool, cfg.Database.ConnectTimeout)
				},
			},
			httpapi.CheckerFunc{
				DependencyName: "objectstore",
				Fn: func(ctx context.Context) error {
					return store.Check(ctx, cfg.Database.ConnectTimeout)
				},
			},
		},
		CheckTimeout:  cfg.Database.ConnectTimeout + time.Second,
		API:           &services,
		SecureCookies: cfg.Session.CookieSecure,
		SessionMaxAge: cfg.Session.Absolute,
		StaticDir:     cfg.Web.StaticDir,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("http_addr", cfg.HTTPAddr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			return
		}
		serverErrors <- nil
	}()

	select {
	case err := <-serverErrors:
		return err
	case <-ctx.Done():
		logger.Info("shutting down", slog.String("signal", "terminate"))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("stopped")
	return nil
}

func attrsFrom(cfg config.Config) []any {
	loggable := cfg.Loggable()
	attrs := make([]any, 0, len(loggable))
	for _, key := range []string{"http_addr", "objectstore_endpoint", "objectstore_bucket", "database_max_conns"} {
		attrs = append(attrs, slog.Any(logKeyFor(key), loggable[key]))
	}
	return attrs
}

// logKeyFor maps a configuration key to the log attribute key allowed by the
// identifiers-only logging policy in internal/observability.
func logKeyFor(configKey string) string {
	switch configKey {
	case "objectstore_endpoint":
		return "endpoint"
	case "objectstore_bucket":
		return "bucket"
	case "database_max_conns":
		return "max_conns"
	default:
		return configKey
	}
}
