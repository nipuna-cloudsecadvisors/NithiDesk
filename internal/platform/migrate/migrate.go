// Package migrate applies the embedded SQL migrations in order.
//
// It is deliberately small: one table records what has been applied, an
// advisory lock stops two processes migrating at once, each migration runs
// in its own transaction, and an applied file whose content has changed is
// refused rather than silently ignored (an applied migration is never
// edited, ADR 0001).
package migrate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// advisoryLockKey is an arbitrary constant shared by every NithiDesk process.
const advisoryLockKey int64 = 7_202_610_001

var fileName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// Migration is one versioned SQL file.
type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum [32]byte
}

// Load reads and orders the migrations in fsys.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var out []Migration
	seen := map[int]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := fileName.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		version, _ := strconv.Atoi(m[1])
		if other, dup := seen[version]; dup {
			return nil, fmt.Errorf("migration version %04d is used by both %s and %s", version, other, entry.Name())
		}
		seen[version] = entry.Name()
		body, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		out = append(out, Migration{Version: version, Name: m[2], SQL: string(body), Checksum: sha256.Sum256(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// Result reports what Up did.
type Result struct {
	Applied []Migration
	Current int
}

// Up applies every pending migration. conn must belong to the schema owner.
func Up(ctx context.Context, conn *pgx.Conn, migrations []Migration) (Result, error) {
	if _, err := conn.Exec(ctx, `select pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		return Result{}, fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		// Unlock on a fresh context: the caller's may already be cancelled.
		_, _ = conn.Exec(context.Background(), `select pg_advisory_unlock($1)`, advisoryLockKey)
	}()

	if _, err := conn.Exec(ctx, `
		create table if not exists schema_migration (
			version    integer primary key,
			name       text not null,
			checksum   bytea not null,
			applied_at timestamptz not null default now()
		)`); err != nil {
		return Result{}, fmt.Errorf("create migration table: %w", err)
	}

	applied := map[int][]byte{}
	rows, err := conn.Query(ctx, `select version, checksum from schema_migration`)
	if err != nil {
		return Result{}, fmt.Errorf("read applied migrations: %w", err)
	}
	for rows.Next() {
		var version int
		var checksum []byte
		if err := rows.Scan(&version, &checksum); err != nil {
			rows.Close()
			return Result{}, fmt.Errorf("read applied migrations: %w", err)
		}
		applied[version] = checksum
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Result{}, fmt.Errorf("read applied migrations: %w", err)
	}

	var result Result
	for _, m := range migrations {
		if checksum, ok := applied[m.Version]; ok {
			if string(checksum) != string(m.Checksum[:]) {
				return result, fmt.Errorf("migration %04d_%s was edited after it was applied; add a new migration instead",
					m.Version, m.Name)
			}
			result.Current = m.Version
			continue
		}
		if err := apply(ctx, conn, m); err != nil {
			return result, err
		}
		result.Applied = append(result.Applied, m)
		result.Current = m.Version
	}
	return result, nil
}

func apply(ctx context.Context, conn *pgx.Conn, m Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %04d: %w", m.Version, err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("apply migration %04d_%s: %w", m.Version, m.Name, err)
	}
	if _, err := tx.Exec(ctx, `insert into schema_migration (version, name, checksum) values ($1, $2, $3)`,
		m.Version, m.Name, m.Checksum[:]); err != nil {
		return fmt.Errorf("record migration %04d: %w", m.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %04d: %w", m.Version, err)
	}
	return nil
}

// ErrNoMigrations is returned when the embedded set is empty, which would
// mean a broken build rather than an up-to-date database.
var ErrNoMigrations = errors.New("no migrations embedded")
