// Package identity owns users, credentials, MFA and sessions (ADR 0004).
//
// Sessions are server-side: the browser holds an opaque 256-bit token in an
// HttpOnly cookie and the database stores only its SHA-256. Revocation is a
// row update and takes effect on the next request. MFA (TOTP) is mandatory
// for every internal user; until it is satisfied a session can do nothing
// except complete MFA. The token is rotated whenever the session gains
// privilege (MFA satisfied), which defeats session fixation.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/secretbox"
)

// Account lockout after repeated wrong passwords (doc 10 §3).
const (
	maxFailedLogins = 10
	lockoutDuration = 15 * time.Minute
	maxPINFailures  = 5
	// lastSeenThrottle limits session writes to one per minute per session.
	lastSeenThrottle = time.Minute
)

// MFA states reported to the client after login.
const (
	MFASatisfied      = "satisfied"
	MFAVerifyRequired = "verify_required"
	MFAEnrollRequired = "enroll_required"
)

// Errors returned to clients.
var (
	ErrInvalidCredentials = apperr.New(apperr.KindUnauthorized, "auth.invalid_credentials")
	ErrAccountLocked      = apperr.New(apperr.KindUnauthorized, "auth.account_locked")
	ErrSessionExpired     = apperr.New(apperr.KindUnauthorized, "auth.session_expired")
	ErrSessionLocked      = apperr.New(apperr.KindUnauthorized, "auth.session_locked")
	ErrInvalidCode        = apperr.New(apperr.KindUnauthorized, "auth.invalid_code")
	ErrInvalidPIN         = apperr.New(apperr.KindUnauthorized, "auth.invalid_pin")
	ErrMFAAlreadyEnrolled = apperr.New(apperr.KindConflict, "auth.mfa_already_enrolled")
	ErrMFANotEnrolling    = apperr.New(apperr.KindConflict, "auth.mfa_not_enrolling")
)

// Service implements authentication and session management.
type Service struct {
	pool        *pgxpool.Pool
	box         *secretbox.Box
	cfg         config.Session
	defaultFirm string
	issuer      string
	now         func() time.Time
}

// New builds the service.
func New(pool *pgxpool.Pool, box *secretbox.Box, cfg config.Session, defaultFirm string) *Service {
	return &Service{pool: pool, box: box, cfg: cfg, defaultFirm: defaultFirm, issuer: "NithiDesk", now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// RequestMeta is the network context of a request, for sessions and audit.
type RequestMeta struct {
	IP        string
	UserAgent string
	RequestID string
}

// Session is the caller's session as resolved from a cookie.
type Session struct {
	ID         uuid.UUID
	FirmID     uuid.UUID
	UserID     uuid.UUID
	CSRF       string
	ClientKind string
	MFAState   string
	// NewToken is set when the token was issued or rotated by this call; the
	// transport must write it to the cookie.
	NewToken  string
	Principal *authz.Principal
}

// LoginInput is a password login.
type LoginInput struct {
	Firm        string
	Email       string
	Password    string
	ClientKind  string
	DeviceLabel string
	Meta        RequestMeta
}

func newToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func newCSRF() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("csrf token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (s *Service) resolveFirm(ctx context.Context, code string) (uuid.UUID, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		code = s.defaultFirm
	}
	if code == "" {
		return uuid.Nil, ErrInvalidCredentials
	}
	id, err := db.FirmByCode(ctx, s.pool, code)
	if errors.Is(err, db.ErrNoRows) {
		return uuid.Nil, ErrInvalidCredentials
	}
	return id, err
}

func (s *Service) idleFor(kind string) time.Duration {
	if kind == "mobile" {
		return s.cfg.IdleMobile
	}
	return s.cfg.IdleDesktop
}

// Login verifies a password and opens a session that still needs MFA.
// Unknown accounts, wrong passwords and disabled accounts all return the same
// error after the same amount of work.
func (s *Service) Login(ctx context.Context, in LoginInput) (*Session, error) {
	firmID, err := s.resolveFirm(ctx, in.Firm)
	if err != nil {
		_, _ = VerifyPassword(dummyHash, in.Password)
		return nil, err
	}
	if in.ClientKind != "mobile" {
		in.ClientKind = "desktop"
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	now := s.now()

	var out *Session
	var loginErr error
	err = db.InTx(ctx, s.pool, db.Scope{FirmID: firmID}, func(tx pgx.Tx) error {
		var (
			userID      uuid.UUID
			hash        *string
			status      string
			failed      int
			lockedUntil *time.Time
		)
		err := tx.QueryRow(ctx, `
			select id, password_hash, status, failed_login_count, locked_until
			from app_user where email = $1 for update`, email).
			Scan(&userID, &hash, &status, &failed, &lockedUntil)
		actor := audit.Actor{Kind: audit.ActorUser, IP: in.Meta.IP, RequestID: in.Meta.RequestID}
		if errors.Is(err, pgx.ErrNoRows) {
			_, _ = VerifyPassword(dummyHash, in.Password)
			loginErr = ErrInvalidCredentials
			return audit.Append(ctx, tx, audit.Event{FirmID: firmID, Actor: actor, Action: "auth.login_failed",
				Result: audit.ResultDenied, Reason: "unknown_account"})
		}
		if err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		actor.UserID = userID

		if lockedUntil != nil && lockedUntil.After(now) {
			_, _ = VerifyPassword(dummyHash, in.Password)
			loginErr = ErrAccountLocked
			return audit.Append(ctx, tx, audit.Event{FirmID: firmID, Actor: actor, Action: "auth.login_failed",
				TargetKind: "user", TargetID: userID, Result: audit.ResultDenied, Reason: "locked"})
		}
		ok := false
		if hash != nil {
			ok, _ = VerifyPassword(*hash, in.Password)
		} else {
			_, _ = VerifyPassword(dummyHash, in.Password)
		}
		if !ok || status == "disabled" {
			failed++
			var lock *time.Time
			if failed >= maxFailedLogins {
				t := now.Add(lockoutDuration)
				lock = &t
				failed = 0
			}
			if _, err := tx.Exec(ctx, `update app_user set failed_login_count = $2, locked_until = $3 where id = $1`,
				userID, failed, lock); err != nil {
				return fmt.Errorf("record failed login: %w", err)
			}
			loginErr = ErrInvalidCredentials
			reason := "wrong_password"
			if status == "disabled" {
				reason = "disabled"
			}
			return audit.Append(ctx, tx, audit.Event{FirmID: firmID, Actor: actor, Action: "auth.login_failed",
				TargetKind: "user", TargetID: userID, Result: audit.ResultDenied, Reason: reason})
		}

		var enrolled bool
		if err := tx.QueryRow(ctx, `select exists (select 1 from mfa_factor
			where user_id = $1 and kind = 'totp' and confirmed_at is not null and revoked_at is null)`,
			userID).Scan(&enrolled); err != nil {
			return fmt.Errorf("check mfa: %w", err)
		}
		token, tokenHash, err := newToken()
		if err != nil {
			return err
		}
		csrf, err := newCSRF()
		if err != nil {
			return err
		}
		var sessionID uuid.UUID
		if err := tx.QueryRow(ctx, `
			insert into user_session (firm_id, user_id, token_hash, csrf_token, client_kind, device_label, ip,
			                          user_agent, idle_expires_at, absolute_expires_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) returning id`,
			firmID, userID, tokenHash, csrf, in.ClientKind, nullable(in.DeviceLabel), nullableIP(in.Meta.IP),
			nullable(truncate(in.Meta.UserAgent, 300)), now.Add(s.idleFor(in.ClientKind)), now.Add(s.cfg.Absolute),
		).Scan(&sessionID); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		if _, err := tx.Exec(ctx, `update app_user set failed_login_count = 0, locked_until = null where id = $1`,
			userID); err != nil {
			return fmt.Errorf("reset failed logins: %w", err)
		}
		state := MFAVerifyRequired
		if !enrolled {
			state = MFAEnrollRequired
		}
		out = &Session{ID: sessionID, FirmID: firmID, UserID: userID, CSRF: csrf, ClientKind: in.ClientKind,
			MFAState: state, NewToken: token}
		actor.SessionID = sessionID
		return audit.Append(ctx, tx, audit.Event{FirmID: firmID, Actor: actor, Action: "auth.password_accepted",
			TargetKind: "user", TargetID: userID})
	})
	if err != nil {
		return nil, err
	}
	if loginErr != nil {
		return nil, loginErr
	}
	return out, nil
}

// Resolve turns a session cookie into the caller. It enforces revocation,
// idle and absolute expiry, the PIN lock and user status on every request.
func (s *Service) Resolve(ctx context.Context, token string, meta RequestMeta) (*Session, error) {
	if token == "" {
		return nil, apperr.SessionRequired
	}
	tokenHash := hashToken(token)
	firmID, err := db.SessionFirm(ctx, s.pool, tokenHash)
	if errors.Is(err, db.ErrNoRows) {
		return nil, apperr.SessionRequired
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	var out *Session
	var resolveErr error
	err = db.InTx(ctx, s.pool, db.Scope{FirmID: firmID}, func(tx pgx.Tx) error {
		var (
			sess                    Session
			lastSeen, idle, absExp  time.Time
			mfaAt, stepUpAt, locked *time.Time
			pinHash                 *string
			userStatus, locale      string
			nameEN, nameSI          *string
		)
		err := tx.QueryRow(ctx, `
			select s.id, s.user_id, s.csrf_token, s.client_kind, s.last_seen_at, s.idle_expires_at,
			       s.absolute_expires_at, s.mfa_satisfied_at, s.step_up_at, s.locked_at, s.pin_hash,
			       u.status, u.preferred_locale, u.display_name_en, u.display_name_si
			from user_session s join app_user u on u.firm_id = s.firm_id and u.id = s.user_id
			where s.token_hash = $1 and s.revoked_at is null`, tokenHash).
			Scan(&sess.ID, &sess.UserID, &sess.CSRF, &sess.ClientKind, &lastSeen, &idle, &absExp, &mfaAt, &stepUpAt,
				&locked, &pinHash, &userStatus, &locale, &nameEN, &nameSI)
		if errors.Is(err, pgx.ErrNoRows) {
			resolveErr = apperr.SessionRequired
			return nil
		}
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}
		sess.FirmID = firmID

		if userStatus == "disabled" {
			resolveErr = apperr.SessionRequired
			_, err := tx.Exec(ctx, `update user_session set revoked_at = now(), revoked_reason = 'user_disabled' where id = $1`, sess.ID)
			return err
		}
		if now.After(absExp) {
			resolveErr = ErrSessionExpired
			_, err := tx.Exec(ctx, `update user_session set revoked_at = now(), revoked_reason = 'logout' where id = $1`, sess.ID)
			return err
		}
		if locked != nil {
			resolveErr = ErrSessionLocked
			return nil
		}
		if now.After(idle) {
			// A mobile session with a PIN locks instead of ending, so the
			// lawyer in court re-opens it with the PIN (doc 10 §4).
			if sess.ClientKind == "mobile" && pinHash != nil && mfaAt != nil {
				resolveErr = ErrSessionLocked
				_, err := tx.Exec(ctx, `update user_session set locked_at = now() where id = $1`, sess.ID)
				return err
			}
			resolveErr = ErrSessionExpired
			_, err := tx.Exec(ctx, `update user_session set revoked_at = now(), revoked_reason = 'logout' where id = $1`, sess.ID)
			return err
		}
		if now.Sub(lastSeen) > lastSeenThrottle {
			if _, err := tx.Exec(ctx, `update user_session set last_seen_at = $2, idle_expires_at = $3 where id = $1`,
				sess.ID, now, now.Add(s.idleFor(sess.ClientKind))); err != nil {
				return fmt.Errorf("touch session: %w", err)
			}
		}

		sess.MFAState = MFASatisfied
		if mfaAt == nil {
			var enrolled bool
			if err := tx.QueryRow(ctx, `select exists (select 1 from mfa_factor
				where user_id = $1 and kind = 'totp' and confirmed_at is not null and revoked_at is null)`,
				sess.UserID).Scan(&enrolled); err != nil {
				return fmt.Errorf("check mfa: %w", err)
			}
			sess.MFAState = MFAVerifyRequired
			if !enrolled {
				sess.MFAState = MFAEnrollRequired
			}
		}

		p := &authz.Principal{
			FirmID: firmID, UserID: sess.UserID, SessionID: sess.ID, RequestID: meta.RequestID, IP: meta.IP,
			Locale: locale, MatterScope: authz.ScopeNone, Capabilities: map[authz.Capability]bool{},
			StepUpFresh: stepUpAt != nil && now.Sub(*stepUpAt) <= s.cfg.StepUpValidity,
		}
		if nameEN != nil {
			p.DisplayName = *nameEN
		} else if nameSI != nil {
			p.DisplayName = *nameSI
		}
		// Capabilities only exist once MFA is satisfied.
		if sess.MFAState == MFASatisfied {
			if err := loadRoles(ctx, tx, p); err != nil {
				return err
			}
		}
		sess.Principal = p
		out = &sess
		return nil
	})
	if err != nil {
		return nil, err
	}
	if resolveErr != nil {
		return nil, resolveErr
	}
	return out, nil
}

func loadRoles(ctx context.Context, tx pgx.Tx, p *authz.Principal) error {
	rows, err := tx.Query(ctx, `
		select rt.code, rt.capabilities, rt.matter_scope
		from user_role ur join role_template rt on rt.firm_id = ur.firm_id and rt.id = ur.role_template_id
		where ur.user_id = $1 and ur.valid_to is null`, p.UserID)
	if err != nil {
		return fmt.Errorf("load roles: %w", err)
	}
	defer rows.Close()
	var scopes []string
	for rows.Next() {
		var code, scope string
		var caps []string
		if err := rows.Scan(&code, &caps, &scope); err != nil {
			return fmt.Errorf("load roles: %w", err)
		}
		p.Roles = append(p.Roles, code)
		scopes = append(scopes, scope)
		for _, c := range caps {
			p.Capabilities[authz.Capability(c)] = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load roles: %w", err)
	}
	p.MatterScope = authz.WidestScope(scopes)
	return nil
}

func (s *Service) actor(sess *Session) audit.Actor {
	a := audit.Actor{Kind: audit.ActorUser, UserID: sess.UserID, SessionID: sess.ID}
	if sess.Principal != nil {
		a.RequestID, a.IP = sess.Principal.RequestID, sess.Principal.IP
	}
	return a
}

// rotate replaces the session token (privilege change) and returns the new one.
func rotate(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID) (string, error) {
	token, tokenHash, err := newToken()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `update user_session set token_hash = $2 where id = $1`, sessionID, tokenHash); err != nil {
		return "", fmt.Errorf("rotate session: %w", err)
	}
	return token, nil
}

// TOTPEnrollment is shown to the user once, to add NithiDesk to an app.
type TOTPEnrollment struct {
	URI    string `json:"otpauth_uri"`
	Secret string `json:"secret"`
}

// EnrollTOTP starts (or restarts) enrolment for a user without a confirmed
// factor.
func (s *Service) EnrollTOTP(ctx context.Context, sess *Session) (*TOTPEnrollment, error) {
	if sess.MFAState != MFAEnrollRequired {
		return nil, ErrMFAAlreadyEnrolled
	}
	secret, err := NewTOTPSecret()
	if err != nil {
		return nil, err
	}
	var out *TOTPEnrollment
	err = db.InTx(ctx, s.pool, db.Scope{FirmID: sess.FirmID}, func(tx pgx.Tx) error {
		var email string
		if err := tx.QueryRow(ctx, `select email from app_user where id = $1`, sess.UserID).Scan(&email); err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		if _, err := tx.Exec(ctx, `update mfa_factor set revoked_at = now()
			where user_id = $1 and kind = 'totp' and confirmed_at is null and revoked_at is null`, sess.UserID); err != nil {
			return fmt.Errorf("discard pending factor: %w", err)
		}
		sealed, err := s.box.Seal(secret, []byte("totp:"+sess.UserID.String()))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into mfa_factor (firm_id, user_id, kind, secret_encrypted)
			values ($1, $2, 'totp', $3)`, sess.FirmID, sess.UserID, sealed); err != nil {
			return fmt.Errorf("store factor: %w", err)
		}
		out = &TOTPEnrollment{URI: OTPAuthURI(secret, s.issuer, email), Secret: EncodeSecret(secret)}
		return nil
	})
	return out, err
}

// ConfirmTOTP completes enrolment with a first code, satisfies MFA for the
// session, activates the user and returns one-time recovery codes.
func (s *Service) ConfirmTOTP(ctx context.Context, sess *Session, code string) ([]string, error) {
	if sess.MFAState != MFAEnrollRequired {
		return nil, ErrMFAAlreadyEnrolled
	}
	var codes []string
	var verifyErr error
	err := db.InTx(ctx, s.pool, db.Scope{FirmID: sess.FirmID}, func(tx pgx.Tx) error {
		var factorID uuid.UUID
		var sealed []byte
		err := tx.QueryRow(ctx, `select id, secret_encrypted from mfa_factor
			where user_id = $1 and kind = 'totp' and confirmed_at is null and revoked_at is null for update`,
			sess.UserID).Scan(&factorID, &sealed)
		if errors.Is(err, pgx.ErrNoRows) {
			verifyErr = ErrMFANotEnrolling
			return nil
		}
		if err != nil {
			return fmt.Errorf("load factor: %w", err)
		}
		secret, err := s.box.Open(sealed, []byte("totp:"+sess.UserID.String()))
		if err != nil {
			return fmt.Errorf("open factor: %w", err)
		}
		step, ok := VerifyTOTP(secret, code, s.now(), 0)
		if !ok {
			verifyErr = ErrInvalidCode
			return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess),
				Action: "auth.mfa_failed", TargetKind: "user", TargetID: sess.UserID, Result: audit.ResultDenied})
		}
		if _, err := tx.Exec(ctx, `update mfa_factor set confirmed_at = now(), last_used_step = $2 where id = $1`,
			factorID, step); err != nil {
			return fmt.Errorf("confirm factor: %w", err)
		}
		var hashes [][]byte
		codes, hashes, err = NewRecoveryCodes()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update mfa_recovery_code set used_at = now() where user_id = $1 and used_at is null`,
			sess.UserID); err != nil {
			return fmt.Errorf("retire recovery codes: %w", err)
		}
		for _, h := range hashes {
			if _, err := tx.Exec(ctx, `insert into mfa_recovery_code (firm_id, user_id, code_hash) values ($1, $2, $3)`,
				sess.FirmID, sess.UserID, h); err != nil {
				return fmt.Errorf("store recovery code: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `update app_user set status = 'active' where id = $1 and status = 'invited'`,
			sess.UserID); err != nil {
			return fmt.Errorf("activate user: %w", err)
		}
		if err := s.satisfyMFA(ctx, tx, sess); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess),
			Action: "auth.mfa_enrolled", TargetKind: "user", TargetID: sess.UserID})
	})
	if err != nil {
		return nil, err
	}
	if verifyErr != nil {
		return nil, verifyErr
	}
	return codes, nil
}

func (s *Service) satisfyMFA(ctx context.Context, tx pgx.Tx, sess *Session) error {
	// MFA just happened, so the session also counts as freshly stepped up.
	if _, err := tx.Exec(ctx, `update user_session set mfa_satisfied_at = now(), step_up_at = now() where id = $1`,
		sess.ID); err != nil {
		return fmt.Errorf("satisfy mfa: %w", err)
	}
	if _, err := tx.Exec(ctx, `update app_user set last_login_at = now() where id = $1`, sess.UserID); err != nil {
		return fmt.Errorf("record login: %w", err)
	}
	token, err := rotate(ctx, tx, sess.ID)
	if err != nil {
		return err
	}
	sess.NewToken = token
	sess.MFAState = MFASatisfied
	return nil
}

// verifySecondFactor checks a TOTP code or a recovery code.
func (s *Service) verifySecondFactor(ctx context.Context, tx pgx.Tx, sess *Session, code, recovery string) (bool, error) {
	if recovery != "" {
		tag, err := tx.Exec(ctx, `update mfa_recovery_code set used_at = now()
			where id = (select id from mfa_recovery_code where user_id = $1 and code_hash = $2 and used_at is null limit 1)`,
			sess.UserID, HashRecoveryCode(recovery))
		if err != nil {
			return false, fmt.Errorf("use recovery code: %w", err)
		}
		return tag.RowsAffected() == 1, nil
	}
	var factorID uuid.UUID
	var sealed []byte
	var lastStep int64
	err := tx.QueryRow(ctx, `select id, secret_encrypted, last_used_step from mfa_factor
		where user_id = $1 and kind = 'totp' and confirmed_at is not null and revoked_at is null for update`,
		sess.UserID).Scan(&factorID, &sealed, &lastStep)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load factor: %w", err)
	}
	secret, err := s.box.Open(sealed, []byte("totp:"+sess.UserID.String()))
	if err != nil {
		return false, fmt.Errorf("open factor: %w", err)
	}
	step, ok := VerifyTOTP(secret, code, s.now(), lastStep)
	if !ok {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `update mfa_factor set last_used_step = $2 where id = $1`, factorID, step); err != nil {
		return false, fmt.Errorf("record code use: %w", err)
	}
	return true, nil
}

// VerifyMFA satisfies MFA for a session with a TOTP or recovery code.
func (s *Service) VerifyMFA(ctx context.Context, sess *Session, code, recovery string) error {
	if sess.MFAState != MFAVerifyRequired {
		if sess.MFAState == MFAEnrollRequired {
			return ErrMFANotEnrolling
		}
		return nil
	}
	var verifyErr error
	err := db.InTx(ctx, s.pool, db.Scope{FirmID: sess.FirmID}, func(tx pgx.Tx) error {
		ok, err := s.verifySecondFactor(ctx, tx, sess, code, recovery)
		if err != nil {
			return err
		}
		if !ok {
			verifyErr = ErrInvalidCode
			return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess),
				Action: "auth.mfa_failed", TargetKind: "user", TargetID: sess.UserID, Result: audit.ResultDenied})
		}
		if err := s.satisfyMFA(ctx, tx, sess); err != nil {
			return err
		}
		reason := ""
		if recovery != "" {
			reason = "recovery_code"
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess),
			Action: "auth.login_succeeded", TargetKind: "user", TargetID: sess.UserID, Reason: reason})
	})
	if err != nil {
		return err
	}
	return verifyErr
}

// StepUp re-authenticates a satisfied session for a sensitive action.
func (s *Service) StepUp(ctx context.Context, sess *Session, password, code string) error {
	var stepErr error
	err := db.InTx(ctx, s.pool, db.Scope{FirmID: sess.FirmID}, func(tx pgx.Tx) error {
		var hash *string
		if err := tx.QueryRow(ctx, `select password_hash from app_user where id = $1`, sess.UserID).Scan(&hash); err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		ok := false
		if hash != nil {
			ok, _ = VerifyPassword(*hash, password)
		}
		if ok {
			ok, err := s.verifySecondFactor(ctx, tx, sess, code, "")
			if err != nil {
				return err
			}
			if ok {
				if _, err := tx.Exec(ctx, `update user_session set step_up_at = now() where id = $1`, sess.ID); err != nil {
					return fmt.Errorf("step up: %w", err)
				}
				return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess), Action: "auth.step_up",
					TargetKind: "user", TargetID: sess.UserID})
			}
		}
		stepErr = ErrInvalidCredentials
		return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess), Action: "auth.step_up",
			TargetKind: "user", TargetID: sess.UserID, Result: audit.ResultDenied})
	})
	if err != nil {
		return err
	}
	return stepErr
}

// Logout ends the session.
func (s *Service) Logout(ctx context.Context, sess *Session) error {
	return db.InTx(ctx, s.pool, db.Scope{FirmID: sess.FirmID}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `update user_session set revoked_at = now(), revoked_reason = 'logout'
			where id = $1 and revoked_at is null`, sess.ID); err != nil {
			return fmt.Errorf("logout: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess), Action: "auth.logout",
			TargetKind: "session", TargetID: sess.ID})
	})
}

// SetPIN sets the mobile quick-unlock PIN (4–8 digits) for this session.
func (s *Service) SetPIN(ctx context.Context, sess *Session, pin string) error {
	if !validPIN(pin) {
		return apperr.Invalid(apperr.Field("pin", "pin.invalid"))
	}
	if sess.ClientKind != "mobile" {
		return apperr.New(apperr.KindConflict, "auth.pin_mobile_only")
	}
	hash, err := HashPassword(pin)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, db.Scope{FirmID: sess.FirmID}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `update user_session set pin_hash = $2, pin_failures = 0 where id = $1`,
			sess.ID, hash); err != nil {
			return fmt.Errorf("set pin: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess), Action: "auth.pin_set",
			TargetKind: "session", TargetID: sess.ID})
	})
}

func validPIN(pin string) bool {
	if len(pin) < 4 || len(pin) > 8 {
		return false
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Unlock re-opens a PIN-locked mobile session. Five wrong PINs end it.
func (s *Service) Unlock(ctx context.Context, token, pin string, meta RequestMeta) error {
	tokenHash := hashToken(token)
	firmID, err := db.SessionFirm(ctx, s.pool, tokenHash)
	if errors.Is(err, db.ErrNoRows) {
		return apperr.SessionRequired
	}
	if err != nil {
		return err
	}
	now := s.now()
	var unlockErr error
	err = db.InTx(ctx, s.pool, db.Scope{FirmID: firmID}, func(tx pgx.Tx) error {
		var (
			id, userID uuid.UUID
			pinHash    *string
			failures   int
			absExp     time.Time
			kind       string
		)
		err := tx.QueryRow(ctx, `select id, user_id, pin_hash, pin_failures, absolute_expires_at, client_kind
			from user_session where token_hash = $1 and revoked_at is null for update`, tokenHash).
			Scan(&id, &userID, &pinHash, &failures, &absExp, &kind)
		if errors.Is(err, pgx.ErrNoRows) {
			unlockErr = apperr.SessionRequired
			return nil
		}
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}
		actor := audit.Actor{Kind: audit.ActorUser, UserID: userID, SessionID: id, RequestID: meta.RequestID, IP: meta.IP}
		if pinHash == nil || now.After(absExp) {
			unlockErr = ErrSessionExpired
			return nil
		}
		ok, _ := VerifyPassword(*pinHash, pin)
		if !ok {
			failures++
			if failures >= maxPINFailures {
				unlockErr = ErrSessionExpired
				if _, err := tx.Exec(ctx, `update user_session set revoked_at = now(), revoked_reason = 'pin_failures',
					pin_failures = $2 where id = $1`, id, failures); err != nil {
					return fmt.Errorf("revoke session: %w", err)
				}
			} else {
				unlockErr = ErrInvalidPIN.WithParams(map[string]any{"remaining": maxPINFailures - failures})
				if _, err := tx.Exec(ctx, `update user_session set pin_failures = $2 where id = $1`, id, failures); err != nil {
					return fmt.Errorf("record pin failure: %w", err)
				}
			}
			return audit.Append(ctx, tx, audit.Event{FirmID: firmID, Actor: actor, Action: "auth.pin_failed",
				TargetKind: "session", TargetID: id, Result: audit.ResultDenied})
		}
		if _, err := tx.Exec(ctx, `update user_session set locked_at = null, pin_failures = 0, last_seen_at = $2,
			idle_expires_at = $3 where id = $1`, id, now, now.Add(s.idleFor(kind))); err != nil {
			return fmt.Errorf("unlock: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: firmID, Actor: actor, Action: "auth.unlocked",
			TargetKind: "session", TargetID: id})
	})
	if err != nil {
		return err
	}
	return unlockErr
}

// SessionView is one entry in the user's device list.
type SessionView struct {
	ID          uuid.UUID `json:"id"`
	ClientKind  string    `json:"client_kind"`
	DeviceLabel *string   `json:"device_label"`
	CreatedAt   time.Time `json:"created_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	Current     bool      `json:"current"`
}

// ListSessions returns the caller's active sessions.
func (s *Service) ListSessions(ctx context.Context, sess *Session) ([]SessionView, error) {
	var out []SessionView
	err := db.ReadTx(ctx, s.pool, db.Scope{FirmID: sess.FirmID}, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id, client_kind, device_label, created_at, last_seen_at
			from user_session where user_id = $1 and revoked_at is null order by last_seen_at desc`, sess.UserID)
		if err != nil {
			return fmt.Errorf("list sessions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var v SessionView
			if err := rows.Scan(&v.ID, &v.ClientKind, &v.DeviceLabel, &v.CreatedAt, &v.LastSeenAt); err != nil {
				return fmt.Errorf("list sessions: %w", err)
			}
			v.Current = v.ID == sess.ID
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// RevokeSession ends one of the caller's own sessions.
func (s *Service) RevokeSession(ctx context.Context, sess *Session, id uuid.UUID) error {
	var notFound bool
	err := db.InTx(ctx, s.pool, db.Scope{FirmID: sess.FirmID}, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `update user_session set revoked_at = now(), revoked_reason = 'user_revoked'
			where id = $1 and user_id = $2 and revoked_at is null`, id, sess.UserID)
		if err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}
		if tag.RowsAffected() == 0 {
			notFound = true
			return nil
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess),
			Action: "auth.session_revoked", TargetKind: "session", TargetID: id})
	})
	if err != nil {
		return err
	}
	if notFound {
		return apperr.NotFound
	}
	return nil
}

// ChangePassword replaces the caller's password and ends their other sessions.
func (s *Service) ChangePassword(ctx context.Context, sess *Session, current, next string) error {
	var changeErr error
	err := db.InTx(ctx, s.pool, db.Scope{FirmID: sess.FirmID}, func(tx pgx.Tx) error {
		var hash *string
		var email string
		if err := tx.QueryRow(ctx, `select password_hash, email from app_user where id = $1 for update`,
			sess.UserID).Scan(&hash, &email); err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		ok := false
		if hash != nil {
			ok, _ = VerifyPassword(*hash, current)
		}
		if !ok {
			changeErr = ErrInvalidCredentials
			return nil
		}
		if code := CheckPasswordPolicy(next, email); code != "" {
			changeErr = apperr.Invalid(apperr.Field("new_password", code))
			return nil
		}
		newHash, err := HashPassword(next)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update app_user set password_hash = $2, password_changed_at = now() where id = $1`,
			sess.UserID, newHash); err != nil {
			return fmt.Errorf("change password: %w", err)
		}
		if _, err := tx.Exec(ctx, `update user_session set revoked_at = now(), revoked_reason = 'password_changed'
			where user_id = $1 and id <> $2 and revoked_at is null`, sess.UserID, sess.ID); err != nil {
			return fmt.Errorf("end other sessions: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: sess.FirmID, Actor: s.actor(sess),
			Action: "auth.password_changed", TargetKind: "user", TargetID: sess.UserID})
	})
	if err != nil {
		return err
	}
	return changeErr
}

func nullable(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func nullableIP(ip string) *string {
	if ip == "" {
		return nil
	}
	return &ip
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
