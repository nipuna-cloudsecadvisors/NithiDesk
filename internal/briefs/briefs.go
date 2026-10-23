// Package briefs builds the M1 daily court brief (doc 06 §6.3) and its
// printable bilingual PDF (ADR 0007, spike #26).
//
// The brief is the continuity fallback for a court day: if the system or the
// connection fails, the lawyer still has paper. Its content is exactly the
// Today view (hearings.Service.Today) for one to seven days, so the printed
// page can never show something the screen would hide: diary-access cards
// carry no instructions or order text, and masked matters carry no names.
//
// PDFs are rendered asynchronously by the worker ("brief.render" jobs),
// never in the API request path, and stored privately (ADR 0005) under a
// random key. Only the requester can fetch the file, only until it expires,
// and every download is audited as an export (doc 10 §8).
package briefs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/hearings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/clock"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/objectstore"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/worker"
)

// JobKind is the worker job that renders a brief PDF.
const JobKind = "brief.render"

// Render statuses (brief_render.status).
const (
	StatusQueued    = "queued"
	StatusRendering = "rendering"
	StatusReady     = "ready"
	StatusFailed    = "failed"
)

// Limits.
const (
	maxDays = 7
	// renderAttempts is how many times a render is tried before the row is
	// marked failed. It is below the job table's max_attempts (5), so the
	// row always reaches a final state while the job still exists.
	renderAttempts = 3
)

// Errors returned to clients.
var (
	ErrNotReady = apperr.New(apperr.KindConflict, "brief.not_ready")
	ErrExpired  = apperr.New(apperr.KindConflict, "brief.expired")
)

// Service builds briefs and manages their PDF renders.
type Service struct {
	pool     *pgxpool.Pool
	hearings *hearings.Service
	store    objectstore.Blob
	printer  *Printer
	logger   *slog.Logger
}

// New builds the service. Fonts are loaded and the browser located once;
// a problem with either fails renders, not the process.
func New(pool *pgxpool.Pool, h *hearings.Service, store objectstore.Blob, cfg config.PDF, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{pool: pool, hearings: h, store: store, printer: NewPrinter(cfg, logger), logger: logger}
}

// Brief is the daily brief: the Today view for each day from From to To.
type Brief struct {
	From  string             `json:"from"`
	To    string             `json:"to"`
	Scope string             `json:"scope"`
	Days  []hearings.DayView `json:"days"`
}

// normaliseScope applies the default and the firm-wide capability rule.
func normaliseScope(p *authz.Principal, scope string) (string, error) {
	switch scope {
	case "":
		return hearings.ScopeMine, nil
	case hearings.ScopeMine, hearings.ScopeTeam:
		return scope, nil
	case hearings.ScopeFirm:
		// The firm-wide brief lists every visible hearing in the firm on one
		// piece of paper, so it is a capability of its own (doc 09 §6).
		if err := p.Require(authz.BriefPrintFirm); err != nil {
			return "", err
		}
		return scope, nil
	default:
		return "", apperr.Invalid(apperr.Field("scope", "invalid"))
	}
}

// Daily returns the brief for days 1–7 (default 1) starting at date.
func (s *Service) Daily(ctx context.Context, p *authz.Principal, date time.Time, days int, scope string) (*Brief, error) {
	if days == 0 {
		days = 1
	}
	if days < 1 || days > maxDays {
		return nil, apperr.Invalid(apperr.Field("days", "invalid"))
	}
	scope, err := normaliseScope(p, scope)
	if err != nil {
		return nil, err
	}
	b := &Brief{From: clock.FormatDate(date), To: clock.FormatDate(date.AddDate(0, 0, days-1)), Scope: scope,
		Days: make([]hearings.DayView, 0, days)}
	for i := range days {
		day, err := s.hearings.Today(ctx, p, date.AddDate(0, 0, i), scope, nil)
		if err != nil {
			return nil, err
		}
		b.Days = append(b.Days, *day)
	}
	return b, nil
}

// RenderInput requests a PDF.
type RenderInput struct {
	Date  string `json:"date"`  // YYYY-MM-DD; default today in Sri Lanka
	Days  int    `json:"days"`  // 1–7; default 2 (today and tomorrow)
	Scope string `json:"scope"` // mine (default), team, firm
	Lang  string `json:"lang"`  // en, si, both (default)
}

// Render is one requested PDF.
type Render struct {
	ID          uuid.UUID  `json:"id"`
	Date        string     `json:"date"`
	Days        int        `json:"days"`
	Scope       string     `json:"scope"`
	Lang        string     `json:"lang"`
	Status      string     `json:"status"`
	ByteSize    *int       `json:"byte_size,omitempty"`
	SHA256      string     `json:"sha256,omitempty"`
	ErrorCode   *string    `json:"error_code,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

const renderColumns = `id, to_char(brief_date, 'YYYY-MM-DD'), days, scope, lang, status, byte_size, sha256, error_code,
	created_at, completed_at, expires_at`

func scanRender(row pgx.Row) (*Render, error) {
	var r Render
	var sum []byte
	if err := row.Scan(&r.ID, &r.Date, &r.Days, &r.Scope, &r.Lang, &r.Status, &r.ByteSize, &sum, &r.ErrorCode,
		&r.CreatedAt, &r.CompletedAt, &r.ExpiresAt); err != nil {
		return nil, err
	}
	if len(sum) > 0 {
		r.SHA256 = hex.EncodeToString(sum)
	}
	return &r, nil
}

func actorOf(p *authz.Principal) audit.Actor {
	return audit.Actor{Kind: audit.ActorUser, UserID: p.UserID, SessionID: p.SessionID, RequestID: p.RequestID, IP: p.IP}
}

// RequestRender queues a PDF. The row and its job are written in one
// transaction, so a render is never queued without its job or vice versa.
func (s *Service) RequestRender(ctx context.Context, p *authz.Principal, in RenderInput) (*Render, error) {
	date := clock.Today()
	if in.Date != "" {
		d, err := clock.ParseDate(in.Date)
		if err != nil {
			return nil, apperr.Invalid(apperr.Field("date", "date.invalid"))
		}
		date = d
	}
	if in.Days == 0 {
		in.Days = 2
	}
	if in.Days < 1 || in.Days > maxDays {
		return nil, apperr.Invalid(apperr.Field("days", "invalid"))
	}
	switch in.Lang {
	case "":
		in.Lang = LangBoth
	case LangEN, LangSI, LangBoth:
	default:
		return nil, apperr.Invalid(apperr.Field("lang", "invalid"))
	}
	scope, err := normaliseScope(p, in.Scope)
	if err != nil {
		return nil, err
	}
	var out *Render
	err = db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		r, err := scanRender(tx.QueryRow(ctx, `insert into brief_render (firm_id, requested_by, brief_date, days, scope, lang)
			values ($1, $2, $3, $4, $5, $6) returning `+renderColumns, p.FirmID, p.UserID, date, in.Days, scope, in.Lang))
		if err != nil {
			return fmt.Errorf("insert brief render: %w", err)
		}
		if err := worker.Enqueue(ctx, tx, p.FirmID, JobKind, map[string]string{"render_id": r.ID.String()}); err != nil {
			return err
		}
		out = r
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "brief.render_requested",
			TargetKind: "brief_render", TargetID: r.ID, Changes: map[string]any{"date": r.Date, "days": r.Days,
				"scope": r.Scope, "lang": r.Lang}})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetRender returns a render's status. Another user's render is reported
// exactly like a missing one (I-AC-4).
func (s *Service) GetRender(ctx context.Context, p *authz.Principal, id uuid.UUID) (*Render, error) {
	var out *Render
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		r, err := scanRender(tx.QueryRow(ctx, `select `+renderColumns+` from brief_render
			where id = $1 and requested_by = $2`, id, p.UserID))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound
		}
		if err != nil {
			return fmt.Errorf("load brief render: %w", err)
		}
		out = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// OpenFile opens a ready PDF for its requester and audits the download. The
// object is opened before the audit commits, so a download is recorded only
// when there is a file to send; the caller closes the reader.
func (s *Service) OpenFile(ctx context.Context, p *authz.Principal, id uuid.UUID) (io.ReadCloser, int64, error) {
	var (
		body io.ReadCloser
		size int64
	)
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		var status string
		var key *string
		var expires time.Time
		err := tx.QueryRow(ctx, `select status, object_key, expires_at from brief_render
			where id = $1 and requested_by = $2 for update`, id, p.UserID).Scan(&status, &key, &expires)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound
		}
		if err != nil {
			return fmt.Errorf("load brief render: %w", err)
		}
		if status != StatusReady || key == nil {
			return ErrNotReady
		}
		if !time.Now().Before(expires) {
			return ErrExpired
		}
		r, n, err := s.store.Get(ctx, *key)
		if errors.Is(err, objectstore.ErrNotFound) {
			// Swept or lost: nothing to send, so treat it like an expired file.
			return ErrExpired
		}
		if err != nil {
			return fmt.Errorf("open brief file: %w", err)
		}
		body, size = r, n
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "brief.downloaded",
			TargetKind: "brief_render", TargetID: id, Changes: map[string]any{"byte_size": n}})
	})
	if err != nil {
		if body != nil {
			_ = body.Close()
		}
		return nil, 0, err
	}
	return body, size, nil
}

// renderJob is the job payload.
type renderJob struct {
	RenderID uuid.UUID `json:"render_id"`
}

// renderRow is what the worker needs from brief_render and app_user.
type renderRow struct {
	requestedBy uuid.UUID
	date        time.Time
	days        int
	scope       string
	lang        string
	status      string
	nameEN      *string
	nameSI      *string
	userStatus  string
}

// RenderJob handles "brief.render": it rebuilds the requester's view of the
// brief with the requester's current roles, renders it and stores the PDF.
func (s *Service) RenderJob(ctx context.Context, job worker.Job) error {
	started := time.Now()
	var payload renderJob
	if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.RenderID == uuid.Nil || job.FirmID == uuid.Nil {
		return &worker.Permanent{Code: "brief.payload_invalid"}
	}
	id := payload.RenderID
	logger := s.logger.With(slog.Int64("job_id", job.ID), slog.String("firm_id", job.FirmID.String()),
		slog.String("object_id", id.String()))

	// Phase 1: claim the row and rebuild the requester as a principal. The
	// row and roles are firm-scoped only, so the system scope reads them.
	sys := db.Scope{FirmID: job.FirmID, MatterScope: authz.ScopeNone}
	var row renderRow
	p := &authz.Principal{FirmID: job.FirmID, Capabilities: map[authz.Capability]bool{}, MatterScope: authz.ScopeNone,
		RequestID: fmt.Sprintf("job-%d", job.ID)}
	err := db.InTx(ctx, s.pool, sys, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `select b.requested_by, b.brief_date, b.days, b.scope, b.lang, b.status,
			u.display_name_en, u.display_name_si, u.status
			from brief_render b join app_user u on u.firm_id = b.firm_id and u.id = b.requested_by
			where b.id = $1 for update of b`, id).Scan(&row.requestedBy, &row.date, &row.days, &row.scope, &row.lang,
			&row.status, &row.nameEN, &row.nameSI, &row.userStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return &worker.Permanent{Code: "brief.render_missing"}
		}
		if err != nil {
			return fmt.Errorf("load brief render: %w", err)
		}
		if row.status == StatusReady || row.status == StatusFailed {
			return nil
		}
		p.UserID = row.requestedBy
		if err := loadRoles(ctx, tx, p); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `update brief_render set status = 'rendering' where id = $1`, id)
		return err
	})
	if err != nil {
		return err
	}
	if row.status == StatusReady || row.status == StatusFailed {
		// Already final (a retried job after a crash between store and ack).
		return nil
	}

	if row.userStatus != "active" {
		return s.fail(ctx, job.FirmID, id, "brief.user_inactive", logger)
	}
	lang := row.lang
	p.DisplayName = pickName(row.nameEN, row.nameSI, lang)

	// Phase 2: the brief, read under the requester's own matter scope so RLS
	// and nd_visible_matters() apply exactly as they would on screen.
	brief, err := s.Daily(ctx, p, row.date, row.days, row.scope)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Kind != apperr.KindInternal {
			// The requester lost the capability or the request is no longer
			// valid; retrying will not change that.
			return s.fail(ctx, job.FirmID, id, ae.Code, logger)
		}
		return s.retryOrFail(ctx, job, id, "brief.load_failed", err, logger)
	}

	// Phase 3: render and store. The object is written before the row is
	// marked ready (ADR 0005), so "ready" always has a file behind it.
	if !s.printer.Available() {
		return s.fail(ctx, job.FirmID, id, "brief.renderer_unavailable", logger)
	}
	pdf, err := s.printer.PDF(ctx, Document{Brief: brief, Lang: lang, PreparedFor: p.DisplayName, GeneratedAt: time.Now()})
	if err != nil {
		if errors.Is(err, ErrNoBrowser) || errors.Is(err, ErrFonts) {
			return s.fail(ctx, job.FirmID, id, "brief.renderer_unavailable", logger)
		}
		return s.retryOrFail(ctx, job, id, "brief.render_failed", err, logger)
	}
	key := fmt.Sprintf("briefs/%s/%s.pdf", job.FirmID, uuid.New())
	if err := s.store.Put(ctx, key, bytes.NewReader(pdf), int64(len(pdf)), "application/pdf"); err != nil {
		return s.retryOrFail(ctx, job, id, "brief.store_failed", err, logger)
	}
	sum := sha256.Sum256(pdf)
	err = db.InTx(ctx, s.pool, sys, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `update brief_render set status = 'ready', object_key = $2, byte_size = $3, sha256 = $4,
			error_code = null, completed_at = now() where id = $1`, id, key, len(pdf), sum[:]); err != nil {
			return fmt.Errorf("mark brief ready: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: job.FirmID, Actor: audit.Actor{Kind: audit.ActorSystem,
			RequestID: p.RequestID}, Action: "brief.rendered", TargetKind: "brief_render", TargetID: id,
			Changes: map[string]any{"byte_size": len(pdf), "sha256": hex.EncodeToString(sum[:]),
				"hearings": countHearings(brief)}})
	})
	if err != nil {
		// The object is orphaned; the orphan sweep of ADR 0005 removes it.
		return err
	}
	logger.Info("brief rendered", slog.String("object_key", key), slog.Int("count", countHearings(brief)),
		slog.String("duration", time.Since(started).String()))
	return nil
}

func countHearings(b *Brief) int {
	n := 0
	for _, d := range b.Days {
		n += d.Counts.Hearings
	}
	return n
}

// retryOrFail returns err for the worker to retry, until the last attempt,
// when the row is marked failed so the requester sees a final state.
func (s *Service) retryOrFail(ctx context.Context, job worker.Job, id uuid.UUID, code string, err error, logger *slog.Logger) error {
	if job.Attempts < renderAttempts {
		logger.Warn("brief render attempt failed", slog.String("code", code), slog.Int("attempt", job.Attempts),
			slog.String("error", err.Error()))
		return err
	}
	logger.Error("brief render failed", slog.String("code", code), slog.String("error", err.Error()))
	return s.fail(ctx, job.FirmID, id, code, logger)
}

// fail marks the render failed with a code and ends the job permanently.
func (s *Service) fail(ctx context.Context, firmID, id uuid.UUID, code string, logger *slog.Logger) error {
	err := db.InTx(ctx, s.pool, db.Scope{FirmID: firmID, MatterScope: authz.ScopeNone}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `update brief_render set status = 'failed', error_code = $2, completed_at = now()
			where id = $1`, id, code); err != nil {
			return fmt.Errorf("mark brief failed: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: firmID, Actor: audit.Actor{Kind: audit.ActorSystem},
			Action: "brief.render_failed", TargetKind: "brief_render", TargetID: id, Result: audit.ResultError,
			Reason: code})
	})
	if err != nil {
		logger.Error("record brief failure", slog.String("code", code), slog.String("error", err.Error()))
		return err
	}
	return &worker.Permanent{Code: code}
}

// loadRoles resolves the user's capabilities and widest matter scope from
// user_role and role_template, by the same rule as identity's session
// resolution, so a worker-built principal sees exactly what a session would.
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

// pickName chooses the requester's name for the watermark: the Sinhala
// name on a Sinhala brief, otherwise the English one, each falling back to
// the other. A person's own name is never doubled up as "Sinhala / English".
func pickName(en, si *string, lang string) string {
	if lang == LangSI {
		return pick(hearings.Name{EN: en, SI: si}, LangSI)
	}
	return pick(hearings.Name{EN: en, SI: si}, LangEN)
}
