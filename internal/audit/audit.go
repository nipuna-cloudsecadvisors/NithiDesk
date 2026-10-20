// Package audit appends tamper-evident audit events (doc 10 §9).
//
// Rules this package enforces:
//   - An event is written in the same transaction as the change it records
//     (I-AU-2). If the audit write fails, the change fails.
//   - Events are append-only; the database rejects UPDATE and DELETE.
//   - Each firm's events form a hash chain: hash = SHA-256(prev_hash ||
//     canonical event), so a removed or altered row is detectable.
//   - Changes record identifiers and business fields, never secrets, file
//     content or full identity numbers.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Actor kinds.
const (
	ActorUser      = "user"
	ActorSystem    = "system"
	ActorImport    = "import"
	ActorConnector = "connector"
)

// Results.
const (
	ResultSuccess  = "success"
	ResultDenied   = "denied"
	ResultNotFound = "not_found"
	ResultError    = "error"
)

// Actor identifies who caused an event and from where.
type Actor struct {
	Kind      string
	UserID    uuid.UUID
	SessionID uuid.UUID
	RequestID string
	IP        string
}

// Event is one audit record.
type Event struct {
	FirmID     uuid.UUID
	Actor      Actor
	Action     string // e.g. hearing.verified, auth.login_failed
	TargetKind string
	TargetID   uuid.UUID
	MatterID   uuid.UUID
	Result     string
	Reason     string
	Changes    map[string]any
}

// genesis is the previous hash of a firm's first event.
var genesis = make([]byte, 32)

// Append writes ev inside tx, extending the firm's hash chain.
func Append(ctx context.Context, tx pgx.Tx, ev Event) error {
	if ev.Result == "" {
		ev.Result = ResultSuccess
	}
	if ev.Actor.Kind == "" {
		ev.Actor.Kind = ActorUser
	}

	// Lock the chain head; create it on a firm's first event.
	if _, err := tx.Exec(ctx,
		`insert into audit_chain_head (firm_id, last_hash) values ($1, $2) on conflict (firm_id) do nothing`,
		ev.FirmID, genesis); err != nil {
		return fmt.Errorf("audit chain head: %w", err)
	}
	var prev []byte
	if err := tx.QueryRow(ctx,
		`select last_hash from audit_chain_head where firm_id = $1 for update`, ev.FirmID).Scan(&prev); err != nil {
		return fmt.Errorf("lock audit chain: %w", err)
	}

	occurred := time.Now().UTC().Truncate(time.Microsecond)
	var changes []byte
	if len(ev.Changes) > 0 {
		b, err := json.Marshal(ev.Changes)
		if err != nil {
			return fmt.Errorf("encode audit changes: %w", err)
		}
		changes = b
	}

	hash := chainHash(prev, ev, occurred, changes)

	var ip *netip.Addr
	if parsed, err := netip.ParseAddr(ev.Actor.IP); err == nil {
		ip = &parsed
	}

	var id int64
	err := tx.QueryRow(ctx, `
		insert into audit_event (firm_id, occurred_at, actor_kind, actor_user_id, session_id, request_id, ip,
		                         action, target_kind, target_id, matter_id, result, reason, changes, prev_hash, hash)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		returning id`,
		ev.FirmID, occurred, ev.Actor.Kind, nullUUID(ev.Actor.UserID), nullUUID(ev.Actor.SessionID),
		nullString(ev.Actor.RequestID), ip, ev.Action, nullString(ev.TargetKind), nullUUID(ev.TargetID),
		nullUUID(ev.MatterID), ev.Result, nullString(ev.Reason), changes, prev, hash,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`update audit_chain_head set last_id = $2, last_hash = $3 where firm_id = $1`, ev.FirmID, id, hash); err != nil {
		return fmt.Errorf("advance audit chain: %w", err)
	}
	return nil
}

// chainHash is the canonical hash of one event. Field order is fixed; it
// must never change for existing rows, so a new field is appended at the end.
func chainHash(prev []byte, ev Event, occurred time.Time, changes []byte) []byte {
	h := sha256.New()
	h.Write(prev)
	for _, part := range []string{
		ev.FirmID.String(), occurred.Format(time.RFC3339Nano), ev.Actor.Kind, ev.Actor.UserID.String(),
		ev.Actor.SessionID.String(), ev.Action, ev.TargetKind, ev.TargetID.String(), ev.MatterID.String(),
		ev.Result, ev.Reason, string(changes),
	} {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(part), part)
	}
	return h.Sum(nil)
}

// VerifyChain recomputes a firm's chain and returns the id of the first event
// that does not match, or 0 when the chain is intact.
func VerifyChain(ctx context.Context, tx pgx.Tx, firmID uuid.UUID) (int64, error) {
	rows, err := tx.Query(ctx, `
		select id, occurred_at, actor_kind, coalesce(actor_user_id, '00000000-0000-0000-0000-000000000000'),
		       coalesce(session_id, '00000000-0000-0000-0000-000000000000'), action, coalesce(target_kind, ''),
		       coalesce(target_id, '00000000-0000-0000-0000-000000000000'),
		       coalesce(matter_id, '00000000-0000-0000-0000-000000000000'), result, coalesce(reason, ''),
		       changes, prev_hash, hash
		from audit_event where firm_id = $1 order by id`, firmID)
	if err != nil {
		return 0, fmt.Errorf("read audit chain: %w", err)
	}
	defer rows.Close()
	prev := genesis
	for rows.Next() {
		var (
			id                 int64
			occurred           time.Time
			ev                 Event
			changes, pPrev, pH []byte
		)
		ev.FirmID = firmID
		if err := rows.Scan(&id, &occurred, &ev.Actor.Kind, &ev.Actor.UserID, &ev.Actor.SessionID, &ev.Action,
			&ev.TargetKind, &ev.TargetID, &ev.MatterID, &ev.Result, &ev.Reason, &changes, &pPrev, &pH); err != nil {
			return 0, fmt.Errorf("read audit chain: %w", err)
		}
		// jsonb normalises key order and spacing, so re-encode the stored
		// value the way Append encoded it before hashing.
		if len(changes) > 0 {
			var m map[string]any
			if err := json.Unmarshal(changes, &m); err == nil {
				changes, _ = json.Marshal(m)
			}
		}
		want := chainHash(prev, ev, occurred.UTC(), changes)
		if string(pPrev) != string(prev) || string(want) != string(pH) {
			return id, nil
		}
		prev = pH
	}
	return 0, rows.Err()
}

func nullUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
