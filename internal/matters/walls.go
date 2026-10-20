package matters

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/clock"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
)

// Wall is an ethical wall as its managers see it.
type Wall struct {
	ID                 uuid.UUID   `json:"id"`
	Reason             string      `json:"reason"`
	StartsOn           string      `json:"starts_on"`
	ReviewOn           *string     `json:"review_on,omitempty"`
	MatterIDs          []uuid.UUID `json:"matter_ids"`
	UserIDs            []uuid.UUID `json:"user_ids"`
	CreatedBy          uuid.UUID   `json:"created_by"`
	CreatedAt          time.Time   `json:"created_at"`
	RemovalRequestedBy *uuid.UUID  `json:"removal_requested_by,omitempty"`
	RemovedAt          *time.Time  `json:"removed_at,omitempty"`
}

// WallInput creates a wall.
type WallInput struct {
	Reason    string      `json:"reason"`
	ReviewOn  string      `json:"review_on"`
	MatterIDs []uuid.UUID `json:"matter_ids"`
	UserIDs   []uuid.UUID `json:"user_ids"`
}

// CreateWall excludes users from matters (wall.manage + step-up). Excluded
// users lose any current team membership and future hearing appearance on
// those matters in the same transaction (I-AC-2). The manager must be able
// to see every matter they wall.
func (s *Service) CreateWall(ctx context.Context, p *authz.Principal, in WallInput) (*Wall, error) {
	if err := p.Require(authz.WallManage); err != nil {
		return nil, err
	}
	if err := p.RequireStepUp(); err != nil {
		return nil, err
	}
	var fields []apperr.FieldError
	if strings.TrimSpace(in.Reason) == "" {
		fields = append(fields, apperr.Field("reason", "required"))
	}
	if len(in.MatterIDs) == 0 {
		fields = append(fields, apperr.Field("matter_ids", "required"))
	}
	if len(in.UserIDs) == 0 {
		fields = append(fields, apperr.Field("user_ids", "required"))
	}
	var review *time.Time
	if in.ReviewOn != "" {
		t, err := clock.ParseDate(in.ReviewOn)
		if err != nil {
			fields = append(fields, apperr.Field("review_on", "date.invalid"))
		}
		review = &t
	}
	for _, u := range in.UserIDs {
		if u == p.UserID {
			fields = append(fields, apperr.Field("user_ids", "wall.cannot_wall_self"))
		}
	}
	if len(fields) > 0 {
		return nil, apperr.Invalid(fields...)
	}
	var id uuid.UUID
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		for _, m := range in.MatterIDs {
			if _, _, err := authz.RequireMatter(ctx, tx, m, authz.AccessFull); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `insert into ethical_wall (firm_id, reason, review_on, created_by)
			values ($1, $2, $3, $4) returning id`, p.FirmID, in.Reason, review, p.UserID).Scan(&id); err != nil {
			return fmt.Errorf("create wall: %w", err)
		}
		for _, m := range in.MatterIDs {
			if _, err := tx.Exec(ctx, `insert into ethical_wall_matter (firm_id, wall_id, matter_id) values ($1, $2, $3)
				on conflict do nothing`, p.FirmID, id, m); err != nil {
				return fmt.Errorf("wall matter: %w", err)
			}
		}
		for _, u := range in.UserIDs {
			if _, err := tx.Exec(ctx, `insert into ethical_wall_user (firm_id, wall_id, user_id) values ($1, $2, $3)
				on conflict do nothing`, p.FirmID, id, u); err != nil {
				if code, _, ok := db.Constraint(err); ok && code == "23503" {
					return apperr.Invalid(apperr.Field("user_ids", "user.unknown"))
				}
				return fmt.Errorf("wall user: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `update matter_member set valid_to = $3, ended_by = $4
			where matter_id = any($1) and user_id = any($2) and valid_to is null`,
			in.MatterIDs, in.UserIDs, clock.Today(), p.UserID); err != nil {
			return fmt.Errorf("end walled memberships: %w", err)
		}
		if _, err := tx.Exec(ctx, `delete from hearing_appearance ha using hearing h
			where h.id = ha.hearing_id and h.matter_id = any($1) and ha.user_id = any($2)
			  and h.listing_status = 'scheduled'`, in.MatterIDs, in.UserIDs); err != nil {
			return fmt.Errorf("remove walled appearances: %w", err)
		}
		for _, m := range in.MatterIDs {
			if err := audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "wall.created",
				TargetKind: "ethical_wall", TargetID: id, MatterID: m,
				Changes: map[string]any{"user_ids": in.UserIDs}}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetWall(ctx, p, id)
}

// GetWall returns a wall (wall.manage).
func (s *Service) GetWall(ctx context.Context, p *authz.Principal, id uuid.UUID) (*Wall, error) {
	if !p.Can(authz.WallManage) {
		return nil, apperr.NotFound
	}
	var w Wall
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `select w.id, w.reason, to_char(w.starts_on, 'YYYY-MM-DD'), to_char(w.review_on, 'YYYY-MM-DD'),
			w.created_by, w.created_at, w.removal_requested_by, w.removed_at,
			coalesce((select array_agg(matter_id) from ethical_wall_matter where wall_id = w.id), '{}'),
			coalesce((select array_agg(user_id) from ethical_wall_user where wall_id = w.id), '{}')
			from ethical_wall w where w.id = $1`, id).Scan(&w.ID, &w.Reason, &w.StartsOn, &w.ReviewOn, &w.CreatedBy,
			&w.CreatedAt, &w.RemovalRequestedBy, &w.RemovedAt, &w.MatterIDs, &w.UserIDs)
		if err == pgx.ErrNoRows {
			return apperr.NotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// RequestWallRemoval is the first of the two approvals a removal needs.
func (s *Service) RequestWallRemoval(ctx context.Context, p *authz.Principal, id uuid.UUID, reason string) error {
	return s.wallStep(ctx, p, id, func(tx pgx.Tx, requestedBy *uuid.UUID) error {
		if strings.TrimSpace(reason) == "" {
			return apperr.Invalid(apperr.Field("reason", "required"))
		}
		if requestedBy != nil {
			return apperr.New(apperr.KindConflict, "wall.removal_already_requested")
		}
		if _, err := tx.Exec(ctx, `update ethical_wall set removal_requested_by = $2, removal_requested_at = now(),
			removal_reason = $3 where id = $1`, id, p.UserID, reason); err != nil {
			return fmt.Errorf("request removal: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "wall.removal_requested",
			TargetKind: "ethical_wall", TargetID: id, Reason: reason})
	})
}

// ApproveWallRemoval removes the wall; the approver must differ from the
// requester (database check too).
func (s *Service) ApproveWallRemoval(ctx context.Context, p *authz.Principal, id uuid.UUID) error {
	return s.wallStep(ctx, p, id, func(tx pgx.Tx, requestedBy *uuid.UUID) error {
		if requestedBy == nil {
			return apperr.New(apperr.KindConflict, "wall.removal_not_requested")
		}
		if *requestedBy == p.UserID {
			return apperr.New(apperr.KindConflict, "wall.second_approver_required")
		}
		if _, err := tx.Exec(ctx, `update ethical_wall set removed_at = now(), removed_by = $2 where id = $1`,
			id, p.UserID); err != nil {
			return fmt.Errorf("remove wall: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "wall.removed",
			TargetKind: "ethical_wall", TargetID: id})
	})
}

func (s *Service) wallStep(ctx context.Context, p *authz.Principal, id uuid.UUID, step func(pgx.Tx, *uuid.UUID) error) error {
	if !p.Can(authz.WallManage) {
		return apperr.NotFound
	}
	if err := p.RequireStepUp(); err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		var requestedBy *uuid.UUID
		var removed *time.Time
		err := tx.QueryRow(ctx, `select removal_requested_by, removed_at from ethical_wall where id = $1 for update`, id).
			Scan(&requestedBy, &removed)
		if err == pgx.ErrNoRows {
			return apperr.NotFound
		}
		if err != nil {
			return fmt.Errorf("load wall: %w", err)
		}
		if removed != nil {
			return apperr.New(apperr.KindConflict, "wall.already_removed")
		}
		return step(tx, requestedBy)
	})
}
