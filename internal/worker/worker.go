// Package worker runs background jobs from a PostgreSQL-backed queue
// (ADR 0001): jobs are rows, claimed with SELECT … FOR UPDATE SKIP LOCKED so
// several workers never take the same job, retried with backoff, and failed
// permanently after max_attempts. A job's firm_id tells its handler which
// tenant context to use.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
)

// Job is one claimed unit of work.
type Job struct {
	ID       int64
	FirmID   uuid.UUID
	Kind     string
	Payload  json.RawMessage
	Attempts int
}

// Queue is the storage the runner needs.
type Queue interface {
	Claim(ctx context.Context, worker string, n int) ([]Job, error)
	Complete(ctx context.Context, id int64) error
	Fail(ctx context.Context, id int64, code string, retryAt *time.Time) error
}

// Handler runs one job. A returned *Permanent error fails the job at once.
type Handler func(ctx context.Context, job Job) error

// Permanent marks an error that retrying cannot fix.
type Permanent struct{ Code string }

func (p *Permanent) Error() string { return "permanent: " + p.Code }

// Options configures the runner.
type Options struct {
	Logger  *slog.Logger
	Queue   Queue
	Config  config.Worker
	Handler Handler
}

// Runner polls the queue and dispatches jobs.
type Runner struct {
	opts Options
	name string
}

// New builds a runner.
func New(opts Options) *Runner {
	host, _ := os.Hostname()
	return &Runner{opts: opts, name: fmt.Sprintf("%s-%d", host, os.Getpid())}
}

// Run polls until ctx is cancelled, finishing the job in hand first.
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.opts.Config.PollInterval)
	defer ticker.Stop()
	for {
		r.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) drain(ctx context.Context) {
	for ctx.Err() == nil {
		jobs, err := r.opts.Queue.Claim(ctx, r.name, r.opts.Config.BatchSize)
		if err != nil {
			r.opts.Logger.Error("claim jobs", slog.String("error", err.Error()))
			return
		}
		if len(jobs) == 0 {
			return
		}
		for _, job := range jobs {
			r.runOne(context.WithoutCancel(ctx), job)
		}
	}
}

func (r *Runner) runOne(ctx context.Context, job Job) {
	started := time.Now()
	err := r.opts.Handler(ctx, job)
	logger := r.opts.Logger.With(slog.Int64("job_id", job.ID), slog.String("job", job.Kind),
		slog.String("firm_id", job.FirmID.String()), slog.Int("attempt", job.Attempts))
	if err == nil {
		if cerr := r.opts.Queue.Complete(ctx, job.ID); cerr != nil {
			logger.Error("complete job", slog.String("error", cerr.Error()))
		}
		logger.Info("job done", slog.String("duration", time.Since(started).String()))
		return
	}
	var perm *Permanent
	code := "error"
	var retryAt *time.Time
	if errors.As(err, &perm) {
		code = perm.Code
	} else {
		// Exponential backoff: 30s, 1m, 2m, 4m … capped at 30m.
		delay := time.Duration(30<<min(job.Attempts-1, 6)) * time.Second
		t := time.Now().Add(min(delay, 30*time.Minute))
		retryAt = &t
	}
	if ferr := r.opts.Queue.Fail(ctx, job.ID, code, retryAt); ferr != nil {
		logger.Error("record job failure", slog.String("error", ferr.Error()))
	}
	logger.Warn("job failed", slog.String("code", code), slog.String("error", err.Error()))
}

// PostgresQueue is the job table.
type PostgresQueue struct {
	pool *pgxpool.Pool
}

// NewPostgresQueue builds the queue.
func NewPostgresQueue(pool *pgxpool.Pool) *PostgresQueue { return &PostgresQueue{pool: pool} }

// Claim locks up to n ready jobs. A job locked more than ten minutes ago is
// assumed abandoned by a crashed worker and becomes claimable again.
func (q *PostgresQueue) Claim(ctx context.Context, worker string, n int) ([]Job, error) {
	rows, err := q.pool.Query(ctx, `
		update job set locked_at = now(), locked_by = $1, attempts = attempts + 1
		where id in (
			select id from job
			where completed_at is null and failed_at is null and run_at <= now()
			  and (locked_at is null or locked_at < now() - interval '10 minutes')
			order by run_at, id
			for update skip locked
			limit $2)
		returning id, coalesce(firm_id, '00000000-0000-0000-0000-000000000000'), kind, payload, attempts`, worker, n)
	if err != nil {
		return nil, fmt.Errorf("claim: %w", err)
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.FirmID, &j.Kind, &j.Payload, &j.Attempts); err != nil {
			return nil, fmt.Errorf("claim: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// Complete marks a job done.
func (q *PostgresQueue) Complete(ctx context.Context, id int64) error {
	_, err := q.pool.Exec(ctx, `update job set completed_at = now(), locked_at = null where id = $1`, id)
	return err
}

// Fail records a failure: retried at retryAt, or failed for good when nil or
// out of attempts.
func (q *PostgresQueue) Fail(ctx context.Context, id int64, code string, retryAt *time.Time) error {
	_, err := q.pool.Exec(ctx, `
		update job set locked_at = null, last_error_code = $2,
		       run_at = coalesce($3, run_at),
		       failed_at = case when $3::timestamptz is null or attempts >= max_attempts then now() end
		where id = $1`, id, code, retryAt)
	return err
}

// Enqueue adds a job inside tx, so it exists only if the business change
// that needs it commits.
func Enqueue(ctx context.Context, tx pgx.Tx, firmID uuid.UUID, kind string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode job: %w", err)
	}
	if _, err := tx.Exec(ctx, `insert into job (firm_id, kind, payload) values ($1, $2, $3)`, firmID, kind, body); err != nil {
		return fmt.Errorf("enqueue job: %w", err)
	}
	return nil
}

// Mux routes jobs to handlers by kind.
type Mux map[string]Handler

// Handle dispatches a job; an unknown kind fails permanently.
func (m Mux) Handle(ctx context.Context, job Job) error {
	h, ok := m[job.Kind]
	if !ok {
		return &Permanent{Code: "unknown_job_kind"}
	}
	return h(ctx, job)
}

// NoopHandler completes every job; kept for an empty deployment.
func NoopHandler(logger *slog.Logger) Handler {
	return func(_ context.Context, job Job) error {
		logger.Info("no handler for job", slog.String("job", job.Kind))
		return nil
	}
}
