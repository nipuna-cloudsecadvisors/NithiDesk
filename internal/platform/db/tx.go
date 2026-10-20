package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Scope is the tenant and caller context of one transaction. Row-level
// security filters every table on FirmID; nd_visible_matters() reads UserID
// and MatterScope to apply the matter-visibility rule inside each query.
type Scope struct {
	FirmID      uuid.UUID
	UserID      uuid.UUID // uuid.Nil for system work (worker, import commit is still a user)
	MatterScope string    // member, diary, all_non_walled or none
}

// Beginner is satisfied by *pgxpool.Pool and by test doubles.
type Beginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

var _ Beginner = (*pgxpool.Pool)(nil)

// InTx runs fn in one transaction with the tenant context set. The settings
// are SET LOCAL, so they end with the transaction and can never leak to the
// next user of a pooled connection.
func InTx(ctx context.Context, pool Beginner, scope Scope, fn func(tx pgx.Tx) error) error {
	return inTx(ctx, pool, scope, pgx.TxOptions{}, fn)
}

// ReadTx is InTx for read-only work.
func ReadTx(ctx context.Context, pool Beginner, scope Scope, fn func(tx pgx.Tx) error) error {
	return inTx(ctx, pool, scope, pgx.TxOptions{AccessMode: pgx.ReadOnly}, fn)
}

func inTx(ctx context.Context, pool Beginner, scope Scope, opts pgx.TxOptions, fn func(tx pgx.Tx) error) error {
	if scope.FirmID == uuid.Nil {
		return errors.New("db: transaction without a firm")
	}
	tx, err := pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	userID := ""
	if scope.UserID != uuid.Nil {
		userID = scope.UserID.String()
	}
	matterScope := scope.MatterScope
	if matterScope == "" {
		matterScope = "none"
	}
	if _, err := tx.Exec(ctx,
		`select set_config('app.firm_id', $1, true), set_config('app.user_id', $2, true),
		        set_config('app.matter_scope', $3, true)`,
		scope.FirmID.String(), userID, matterScope); err != nil {
		return fmt.Errorf("set tenant context: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// FirmByCode resolves a firm code before any tenant context exists (login).
func FirmByCode(ctx context.Context, pool *pgxpool.Pool, code string) (uuid.UUID, error) {
	var id *uuid.UUID
	if err := pool.QueryRow(ctx, `select nd_firm_id_by_code($1)`, code).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("resolve firm: %w", err)
	}
	if id == nil {
		return uuid.Nil, ErrNoRows
	}
	return *id, nil
}

// SessionFirm resolves the firm a session token hash belongs to.
func SessionFirm(ctx context.Context, pool *pgxpool.Pool, tokenHash []byte) (uuid.UUID, error) {
	var id *uuid.UUID
	if err := pool.QueryRow(ctx, `select nd_session_firm($1)`, tokenHash).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("resolve session firm: %w", err)
	}
	if id == nil {
		return uuid.Nil, ErrNoRows
	}
	return *id, nil
}

// ErrNoRows aliases pgx.ErrNoRows so callers need not import pgx for it.
var ErrNoRows = pgx.ErrNoRows

// Constraint returns the name of the violated constraint, if err is one.
func Constraint(err error) (code, name string, ok bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, pgErr.ConstraintName, true
	}
	return "", "", false
}

// IsUniqueViolation reports whether err is a unique violation on constraint
// (or on any constraint when name is empty).
func IsUniqueViolation(err error, name string) bool {
	code, constraint, ok := Constraint(err)
	return ok && code == "23505" && (name == "" || constraint == name)
}
