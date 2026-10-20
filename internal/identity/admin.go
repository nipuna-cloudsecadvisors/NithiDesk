package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
)

// UserView is a user as administrators and pickers see it. No credential
// material is ever included.
type UserView struct {
	ID              uuid.UUID  `json:"id"`
	Email           string     `json:"email,omitempty"`
	DisplayName     I18n       `json:"display_name"`
	Status          string     `json:"status"`
	PreferredLocale string     `json:"preferred_locale"`
	Roles           []string   `json:"roles"`
	MFAEnrolled     bool       `json:"mfa_enrolled"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
}

// I18n is a bilingual name.
type I18n struct {
	EN *string `json:"en,omitempty"`
	SI *string `json:"si,omitempty"`
}

// CreateUserInput creates an internal user with an initial password; the user
// enrols TOTP at first login.
type CreateUserInput struct {
	Email           string   `json:"email"`
	DisplayNameEN   string   `json:"display_name_en"`
	DisplayNameSI   string   `json:"display_name_si"`
	PreferredLocale string   `json:"preferred_locale"`
	Password        string   `json:"initial_password"`
	Roles           []string `json:"roles"`
}

func (s *Service) adminActor(p *authz.Principal) audit.Actor {
	return audit.Actor{Kind: audit.ActorUser, UserID: p.UserID, SessionID: p.SessionID, RequestID: p.RequestID, IP: p.IP}
}

// CreateUser creates a user (user.manage + step-up).
func (s *Service) CreateUser(ctx context.Context, p *authz.Principal, in CreateUserInput) (*UserView, error) {
	if err := p.Require(authz.UserManage); err != nil {
		return nil, err
	}
	if err := p.RequireStepUp(); err != nil {
		return nil, err
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	var fields []apperr.FieldError
	if !strings.Contains(in.Email, "@") || len(in.Email) > 254 {
		fields = append(fields, apperr.Field("email", "email.invalid"))
	}
	if strings.TrimSpace(in.DisplayNameEN) == "" && strings.TrimSpace(in.DisplayNameSI) == "" {
		fields = append(fields, apperr.Field("display_name_en", "required"))
	}
	if code := CheckPasswordPolicy(in.Password, in.Email); code != "" {
		fields = append(fields, apperr.Field("initial_password", code))
	}
	if in.PreferredLocale == "" {
		in.PreferredLocale = "en"
	}
	if in.PreferredLocale != "en" && in.PreferredLocale != "si" {
		fields = append(fields, apperr.Field("preferred_locale", "locale.unsupported"))
	}
	if len(in.Roles) == 0 {
		fields = append(fields, apperr.Field("roles", "required"))
	}
	if len(fields) > 0 {
		return nil, apperr.Invalid(fields...)
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	err = db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			insert into app_user (firm_id, email, display_name_en, display_name_si, preferred_locale, status,
			                      password_hash, password_changed_at, created_by)
			values ($1, $2, $3, $4, $5, 'invited', $6, now(), $7) returning id`,
			p.FirmID, in.Email, nullable(in.DisplayNameEN), nullable(in.DisplayNameSI), in.PreferredLocale, hash,
			p.UserID).Scan(&id)
		if db.IsUniqueViolation(err, "app_user_email") {
			return apperr.New(apperr.KindConflict, "user.email_taken")
		}
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		if err := setRoles(ctx, tx, p, id, in.Roles); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: s.adminActor(p), Action: "user.created",
			TargetKind: "user", TargetID: id, Changes: map[string]any{"roles": in.Roles}})
	})
	if err != nil {
		return nil, err
	}
	return s.GetUser(ctx, p, id)
}

func setRoles(ctx context.Context, tx pgx.Tx, p *authz.Principal, userID uuid.UUID, roles []string) error {
	if _, err := tx.Exec(ctx, `update user_role set valid_to = now() where user_id = $1 and valid_to is null`,
		userID); err != nil {
		return fmt.Errorf("end roles: %w", err)
	}
	for _, code := range roles {
		tag, err := tx.Exec(ctx, `insert into user_role (firm_id, user_id, role_template_id, created_by)
			select $1, $2, id, $4 from role_template where code = $3`, p.FirmID, userID, code, p.UserID)
		if err != nil {
			return fmt.Errorf("assign role: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Invalid(apperr.Field("roles", "role.unknown")).WithParams(map[string]any{"role": code})
		}
	}
	return nil
}

// SetRoles replaces a user's roles (user.manage + step-up).
func (s *Service) SetRoles(ctx context.Context, p *authz.Principal, userID uuid.UUID, roles []string) error {
	if err := p.Require(authz.UserManage); err != nil {
		return err
	}
	if err := p.RequireStepUp(); err != nil {
		return err
	}
	if len(roles) == 0 {
		return apperr.Invalid(apperr.Field("roles", "required"))
	}
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `select exists (select 1 from app_user where id = $1)`, userID).Scan(&exists); err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		if !exists {
			return apperr.NotFound
		}
		if err := setRoles(ctx, tx, p, userID, roles); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: s.adminActor(p), Action: "user.roles_changed",
			TargetKind: "user", TargetID: userID, Changes: map[string]any{"roles": roles}})
	})
}

// SetStatus disables or re-enables a user; disabling ends every session at once.
func (s *Service) SetStatus(ctx context.Context, p *authz.Principal, userID uuid.UUID, disabled bool) error {
	if err := p.Require(authz.UserManage); err != nil {
		return err
	}
	if err := p.RequireStepUp(); err != nil {
		return err
	}
	if userID == p.UserID && disabled {
		return apperr.New(apperr.KindConflict, "user.cannot_disable_self")
	}
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		status := "active"
		action := "user.enabled"
		if disabled {
			status, action = "disabled", "user.disabled"
		}
		tag, err := tx.Exec(ctx, `update app_user set status = $2, updated_by = $3 where id = $1`, userID, status, p.UserID)
		if err != nil {
			return fmt.Errorf("set user status: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound
		}
		if disabled {
			if _, err := tx.Exec(ctx, `update user_session set revoked_at = now(), revoked_reason = 'user_disabled'
				where user_id = $1 and revoked_at is null`, userID); err != nil {
				return fmt.Errorf("end sessions: %w", err)
			}
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: s.adminActor(p), Action: action,
			TargetKind: "user", TargetID: userID})
	})
}

// ResetMFA revokes a user's factors and recovery codes (admin-assisted reset,
// doc 10 §3) and ends their sessions; they enrol again at next login.
func (s *Service) ResetMFA(ctx context.Context, p *authz.Principal, userID uuid.UUID) error {
	if err := p.Require(authz.UserManage); err != nil {
		return err
	}
	if err := p.RequireStepUp(); err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `select exists (select 1 from app_user where id = $1)`, userID).Scan(&exists); err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		if !exists {
			return apperr.NotFound
		}
		for _, q := range []string{
			`update mfa_factor set revoked_at = now() where user_id = $1 and revoked_at is null`,
			`update mfa_recovery_code set used_at = now() where user_id = $1 and used_at is null`,
			`update user_session set revoked_at = now(), revoked_reason = 'mfa_reset' where user_id = $1 and revoked_at is null`,
		} {
			if _, err := tx.Exec(ctx, q, userID); err != nil {
				return fmt.Errorf("reset mfa: %w", err)
			}
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: s.adminActor(p), Action: "user.mfa_reset",
			TargetKind: "user", TargetID: userID})
	})
}

// ListUsers returns the firm's users. Administrators see emails and status;
// other users see the directory needed to assign teams and appearances.
func (s *Service) ListUsers(ctx context.Context, p *authz.Principal) ([]UserView, error) {
	var out []UserView
	admin := p.Can(authz.UserManage)
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, userViewSQL+` where ($1 or u.status = 'active') order by coalesce(u.display_name_en, u.display_name_si)`, admin)
		if err != nil {
			return fmt.Errorf("list users: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanUserView(rows)
			if err != nil {
				return err
			}
			if !admin {
				v.Email, v.LastLoginAt = "", nil
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// GetUser returns one user (administrators, or the user themself).
func (s *Service) GetUser(ctx context.Context, p *authz.Principal, id uuid.UUID) (*UserView, error) {
	if !p.Can(authz.UserManage) && id != p.UserID {
		return nil, apperr.NotFound
	}
	var out UserView
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		row, err := tx.Query(ctx, userViewSQL+` where u.id = $1`, id)
		if err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		defer row.Close()
		if !row.Next() {
			return apperr.NotFound
		}
		out, err = scanUserView(row)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

const userViewSQL = `
	select u.id, u.email, u.display_name_en, u.display_name_si, u.status, u.preferred_locale, u.last_login_at,
	       coalesce((select array_agg(rt.code order by rt.code) from user_role ur
	                 join role_template rt on rt.firm_id = ur.firm_id and rt.id = ur.role_template_id
	                 where ur.user_id = u.id and ur.valid_to is null), '{}'),
	       exists (select 1 from mfa_factor f where f.user_id = u.id and f.confirmed_at is not null and f.revoked_at is null)
	from app_user u`

func scanUserView(rows pgx.Rows) (UserView, error) {
	var v UserView
	if err := rows.Scan(&v.ID, &v.Email, &v.DisplayName.EN, &v.DisplayName.SI, &v.Status, &v.PreferredLocale,
		&v.LastLoginAt, &v.Roles, &v.MFAEnrolled); err != nil {
		return v, fmt.Errorf("scan user: %w", err)
	}
	return v, nil
}

// MeUpdate changes the caller's own preferences.
type MeUpdate struct {
	PreferredLocale *string `json:"preferred_locale"`
	LabelMode       *string `json:"label_mode"`
}

// Me is the caller's profile, roles, capabilities and session facts.
type Me struct {
	UserView
	LabelMode    string   `json:"label_mode"`
	Capabilities []string `json:"capabilities"`
	MatterScope  string   `json:"matter_scope"`
	ClientKind   string   `json:"client_kind"`
	CSRFToken    string   `json:"csrf_token"`
	HasPIN       bool     `json:"has_pin"`
	FirmName     I18n     `json:"firm_name"`
	FirmTimeZone string   `json:"firm_time_zone"`
}

// GetMe returns the caller's profile.
func (s *Service) GetMe(ctx context.Context, sess *Session) (*Me, error) {
	p := sess.Principal
	me := &Me{MatterScope: p.MatterScope, ClientKind: sess.ClientKind, CSRFToken: sess.CSRF}
	for c := range p.Capabilities {
		me.Capabilities = append(me.Capabilities, string(c))
	}
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, userViewSQL+` where u.id = $1`, p.UserID)
		if err != nil {
			return fmt.Errorf("load me: %w", err)
		}
		if !rows.Next() {
			rows.Close()
			return apperr.SessionRequired
		}
		me.UserView, err = scanUserView(rows)
		rows.Close()
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `select label_mode from app_user where id = $1`, p.UserID).Scan(&me.LabelMode); err != nil {
			return fmt.Errorf("load me: %w", err)
		}
		if err := tx.QueryRow(ctx, `select pin_hash is not null from user_session where id = $1`, sess.ID).Scan(&me.HasPIN); err != nil {
			return fmt.Errorf("load me: %w", err)
		}
		return tx.QueryRow(ctx, `select name_en, name_si, time_zone from firm where id = $1`, p.FirmID).
			Scan(&me.FirmName.EN, &me.FirmName.SI, &me.FirmTimeZone)
	})
	if err != nil {
		return nil, err
	}
	return me, nil
}

// UpdateMe changes the caller's locale or label mode.
func (s *Service) UpdateMe(ctx context.Context, sess *Session, in MeUpdate) error {
	var fields []apperr.FieldError
	if in.PreferredLocale != nil && *in.PreferredLocale != "en" && *in.PreferredLocale != "si" {
		fields = append(fields, apperr.Field("preferred_locale", "locale.unsupported"))
	}
	if in.LabelMode != nil && *in.LabelMode != "single" && *in.LabelMode != "dual" {
		fields = append(fields, apperr.Field("label_mode", "invalid"))
	}
	if len(fields) > 0 {
		return apperr.Invalid(fields...)
	}
	return db.InTx(ctx, s.pool, sess.Principal.Scope(), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update app_user set preferred_locale = coalesce($2, preferred_locale),
			label_mode = coalesce($3, label_mode) where id = $1`, sess.UserID, in.PreferredLocale, in.LabelMode)
		if err != nil {
			return fmt.Errorf("update me: %w", err)
		}
		return nil
	})
}

// ErrBootstrapExists is returned when a firm code is already taken.
var ErrBootstrapExists = errors.New("firm already exists")
