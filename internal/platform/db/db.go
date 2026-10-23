// Package db opens the PostgreSQL pool.
//
// Data access is SQL-first: pgx with explicit queries and explicit
// transactions, no ORM. Only standard PostgreSQL features are used, so the
// database is portable to whichever host the firm chooses.
package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
)

// Pool is the subset of the pgx pool the application depends on. Keeping it
// narrow makes the modules above it testable without a database.
type Pool interface {
	Ping(ctx context.Context) error
	Close()
}

// Open creates a connection pool and verifies it with one ping, so a
// misconfigured database fails at start-up rather than on first request.
func Open(ctx context.Context, cfg config.Database) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		// The URL carries a password, so it must never appear in the error.
		return nil, fmt.Errorf("parse database configuration: %w", redact(err, cfg.URL))
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = map[string]string{}
	}
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", cfg.StatementTimeout.Milliseconds())
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "nithidesk"

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", redact(err, cfg.URL))
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", redact(err, cfg.URL))
	}
	return pool, nil
}

// Check reports whether the database is reachable within the deadline. It is
// used by the readiness endpoint.
func Check(ctx context.Context, pool Pool, timeout time.Duration) error {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := pool.Ping(checkCtx); err != nil {
		return fmt.Errorf("database not ready: %w", err)
	}
	return nil
}

// redact removes the connection string from an error so a password cannot
// travel into a log line or an HTTP response.
func redact(err error, url string) error {
	if url == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), url, config.Redacted))
}
