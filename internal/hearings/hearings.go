// Package hearings is the court diary core (doc 06, logical model §8).
//
// A hearing's date is never edited in place. Each claim about the date is an
// append-only date assertion carrying its source and authority; a
// verification confirms one assertion against an authoritative source. The
// verification state is derived by one rule (recompute) and cached on the
// hearing. Listing status — what is happening to the listing — is a separate
// dimension with its own state machine (doc 06 §4.2).
package hearings

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/courts"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/clock"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/proceedings"
)

// Listing statuses (dimension A).
const (
	StatusScheduled       = "scheduled"
	StatusHeld            = "held"
	StatusNotReached      = "not_reached"
	StatusCourtNotSitting = "court_not_sitting"
	StatusVacated         = "vacated"
	StatusCancelled       = "cancelled"
	StatusOutcomeRecorded = "outcome_recorded"
)

// Verification states (dimension B).
const (
	Unverified = "unverified"
	Verified   = "verified"
	Conflict   = "conflict"
)

// Assertion kinds.
const (
	KindInitial       = "initial"
	KindCorrection    = "correction"
	KindFromOutcome   = "from_outcome"
	KindFromCourtList = "from_court_list"
	KindFromImport    = "from_import"
	KindReplacement   = "replacement"
)

var sessions = map[string]bool{"morning": true, "afternoon": true, "unspecified": true}

// Errors.
var (
	ErrDateChangeNeedsAssertion = apperr.New(apperr.KindInvalid, "hearing.date_change_requires_assertion")
	ErrIllegalTransition        = apperr.New(apperr.KindConflict, "hearing.illegal_transition")
	ErrAssertionNotCurrent      = apperr.New(apperr.KindConflict, "verification.assertion_not_current")
	ErrSourceNotAuthoritative   = apperr.New(apperr.KindInvalid, "verification.source_not_authoritative")
	ErrOutcomeNeedsNextOrReason = apperr.New(apperr.KindInvalid, "hearing.outcome_requires_next_or_reason")
	ErrOutcomeInFuture          = apperr.New(apperr.KindConflict, "hearing.outcome_before_date")
	ErrOutcomeExists            = apperr.New(apperr.KindConflict, "hearing.outcome_already_recorded")
	ErrAppearanceNotMember      = apperr.New(apperr.KindConflict, "hearing.appearing_user_not_on_team")
)

// Warning is a non-blocking notice.
type Warning struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// Service implements the diary.
type Service struct {
	pool *pgxpool.Pool
}

// New builds the service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func actorOf(p *authz.Principal) audit.Actor {
	return audit.Actor{Kind: audit.ActorUser, UserID: p.UserID, SessionID: p.SessionID, RequestID: p.RequestID, IP: p.IP}
}

// DateClaim is a date with its source, as entered.
type DateClaim struct {
	Date         string    `json:"date"`
	Session      string    `json:"session"`
	Time         string    `json:"time"`
	SourceTypeID uuid.UUID `json:"source_type_id"`
	SourceDetail string    `json:"source_detail"`
}

type parsedClaim struct {
	date      time.Time
	session   string
	timeOfDay *string
	sourceID  uuid.UUID
	authority string
	detail    *string
}

func parseClaim(ctx context.Context, tx pgx.Tx, c DateClaim, prefix string) (parsedClaim, error) {
	var fields []apperr.FieldError
	date, err := clock.ParseDate(c.Date)
	if err != nil {
		fields = append(fields, apperr.Field(prefix+"date", "date.invalid"))
	}
	if c.Session == "" {
		c.Session = "unspecified"
	}
	if !sessions[c.Session] {
		fields = append(fields, apperr.Field(prefix+"session", "invalid"))
	}
	var tod *string
	if c.Time != "" {
		if _, err := time.Parse("15:04", c.Time); err != nil {
			fields = append(fields, apperr.Field(prefix+"time", "time.invalid"))
		} else {
			tod = &c.Time
		}
	}
	if len(fields) > 0 {
		return parsedClaim{}, apperr.Invalid(fields...)
	}
	_, attrs, err := courts.ItemByID(ctx, tx, courts.KindDateSourceType, c.SourceTypeID)
	if err != nil {
		return parsedClaim{}, err
	}
	authority, _ := attrs["default_authority"].(string)
	if authority == "" {
		authority = courts.AuthorityHearsay
	}
	var detail *string
	if d := strings.TrimSpace(c.SourceDetail); d != "" {
		detail = &d
	}
	return parsedClaim{date: date, session: c.Session, timeOfDay: tod, sourceID: c.SourceTypeID, authority: authority,
		detail: detail}, nil
}

// CreateInput lists a new hearing.
type CreateInput struct {
	DateClaim
	PurposeID    *uuid.UUID  `json:"purpose_id"`
	PurposeNote  string      `json:"purpose_note"`
	CourtID      *uuid.UUID  `json:"court_id"`
	CourtroomID  *uuid.UUID  `json:"courtroom_id"`
	Appearing    []uuid.UUID `json:"appearing_user_ids"`
	ClerkID      *uuid.UUID  `json:"assigned_clerk_id"`
	Instructions string      `json:"instructions"`
	// Importer only.
	CreatedVia    string    `json:"-"`
	ImportBatchID uuid.UUID `json:"-"`
	AssertionKind string    `json:"-"`
}

// Create lists a hearing with its first date assertion (I-HE-1).
func (s *Service) Create(ctx context.Context, p *authz.Principal, proceedingID uuid.UUID, in CreateInput) (*Detail, []Warning, error) {
	var id uuid.UUID
	var warnings []Warning
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, err := proceedings.RequireProceeding(ctx, tx, proceedingID, authz.AccessFull)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingWrite); err != nil {
			return err
		}
		id, warnings, err = CreateInTx(ctx, tx, p, proceedingID, matterID, in)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	d, err := s.Get(ctx, p, id)
	return d, warnings, err
}

// CreateInTx lists a hearing inside tx; the caller checked access.
func CreateInTx(ctx context.Context, tx pgx.Tx, p *authz.Principal, proceedingID, matterID uuid.UUID, in CreateInput) (uuid.UUID, []Warning, error) {
	claim, err := parseClaim(ctx, tx, in.DateClaim, "")
	if err != nil {
		return uuid.Nil, nil, err
	}
	if in.PurposeID != nil {
		if _, _, err := courts.ItemByID(ctx, tx, courts.KindHearingPurpose, *in.PurposeID); err != nil {
			return uuid.Nil, nil, err
		}
	}
	courtID := uuid.Nil
	if in.CourtID != nil {
		courtID = *in.CourtID
	} else if err := tx.QueryRow(ctx, `select current_court_id from proceeding where id = $1`, proceedingID).Scan(&courtID); err != nil {
		return uuid.Nil, nil, fmt.Errorf("load proceeding court: %w", err)
	}
	via := in.CreatedVia
	if via == "" {
		via = "user"
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		insert into hearing (firm_id, proceeding_id, matter_id, court_id, courtroom_id, hearing_date, session, time_of_day,
		                     purpose_id, purpose_note, instructions, assigned_clerk_id, created_by, created_via, import_batch_id)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) returning id`,
		p.FirmID, proceedingID, matterID, courtID, in.CourtroomID, claim.date, claim.session, claim.timeOfDay, in.PurposeID,
		nullable(in.PurposeNote), nullable(in.Instructions), in.ClerkID, p.UserID, via, nullUUID(in.ImportBatchID)).Scan(&id)
	if code, _, ok := db.Constraint(err); ok && code == "23503" {
		return uuid.Nil, nil, apperr.Invalid(apperr.Field("court_id", "reference.unknown"))
	}
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("create hearing: %w", err)
	}
	kind := in.AssertionKind
	if kind == "" {
		kind = KindInitial
	}
	if _, err := insertAssertion(ctx, tx, p, id, claim, kind, nil); err != nil {
		return uuid.Nil, nil, err
	}
	for _, u := range in.Appearing {
		if err := addAppearance(ctx, tx, p, id, matterID, u); err != nil {
			return uuid.Nil, nil, err
		}
	}
	if err := recompute(ctx, tx, id); err != nil {
		return uuid.Nil, nil, err
	}
	warnings, err := dateWarnings(ctx, tx, id)
	if err != nil {
		return uuid.Nil, nil, err
	}
	return id, warnings, audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.created",
		TargetKind: "hearing", TargetID: id, MatterID: matterID,
		Changes: map[string]any{"date": in.Date, "session": claim.session, "authority": claim.authority, "via": via}})
}

func insertAssertion(ctx context.Context, tx pgx.Tx, p *authz.Principal, hearingID uuid.UUID, c parsedClaim, kind string, supersedes *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		insert into date_assertion (firm_id, hearing_id, asserted_date, asserted_session, asserted_time, source_type_id,
		                            authority, source_detail, kind, supersedes_id, asserted_by)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) returning id`,
		p.FirmID, hearingID, c.date, c.session, c.timeOfDay, c.sourceID, c.authority, c.detail, kind, supersedes,
		nullUUID(p.UserID)).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert date assertion: %w", err)
	}
	return id, nil
}

// currentAssertion returns the latest assertion of a hearing that no later
// assertion supersedes.
func currentAssertion(ctx context.Context, tx pgx.Tx, hearingID uuid.UUID) (uuid.UUID, time.Time, string, error) {
	var id uuid.UUID
	var date time.Time
	var session string
	err := tx.QueryRow(ctx, `
		select a.id, a.asserted_date, a.asserted_session from date_assertion a
		where a.hearing_id = $1 and not exists (select 1 from date_assertion b where b.supersedes_id = a.id)
		order by a.asserted_at desc, a.id desc limit 1`, hearingID).Scan(&id, &date, &session)
	if err != nil {
		return uuid.Nil, time.Time{}, "", fmt.Errorf("current assertion: %w", err)
	}
	return id, date, session, nil
}

// recompute applies the derivation rule (logical model §8.2) and caches the
// result on the hearing:
//  1. the current assertion is the latest one not superseded;
//  2. an unrevoked verification of it  → verified;
//  3. else a different date that was verified earlier → conflict (I-HE-4);
//  4. else → unverified.
//
// hearing_date and session always follow the current assertion.
func recompute(ctx context.Context, tx pgx.Tx, hearingID uuid.UUID) error {
	curID, date, session, err := currentAssertion(ctx, tx, hearingID)
	if err != nil {
		return err
	}
	var verifiedCurrent, conflicting bool
	if err := tx.QueryRow(ctx, `select
		exists (select 1 from verification v where v.date_assertion_id = $2 and v.revoked_at is null),
		exists (select 1 from verification v join date_assertion a on a.id = v.date_assertion_id
		        where v.hearing_id = $1 and v.revoked_at is null and a.id <> $2
		          and (a.asserted_date <> $3 or a.asserted_session <> $4))`,
		hearingID, curID, date, session).Scan(&verifiedCurrent, &conflicting); err != nil {
		return fmt.Errorf("derive verification: %w", err)
	}
	state := Unverified
	switch {
	case verifiedCurrent:
		state = Verified
	case conflicting:
		state = Conflict
	}
	if _, err := tx.Exec(ctx, `update hearing set hearing_date = $2, session = $3, verification_state = $4 where id = $1`,
		hearingID, date, session, state); err != nil {
		return fmt.Errorf("cache verification state: %w", err)
	}
	return nil
}

// dateWarnings flags a past date, a weekend, a court calendar day, and the
// appearing lawyers' clashes at other courts in the same session.
func dateWarnings(ctx context.Context, tx pgx.Tx, hearingID uuid.UUID) ([]Warning, error) {
	var date time.Time
	var session string
	var courtID uuid.UUID
	if err := tx.QueryRow(ctx, `select hearing_date, session, court_id from hearing where id = $1`, hearingID).
		Scan(&date, &session, &courtID); err != nil {
		return nil, fmt.Errorf("load hearing: %w", err)
	}
	var out []Warning
	if date.Before(clock.Today()) {
		out = append(out, Warning{Code: "hearing.date_in_past"})
	}
	if wd := date.Weekday(); wd == time.Saturday || wd == time.Sunday {
		out = append(out, Warning{Code: "hearing.date_on_weekend"})
	}
	rows, err := tx.Query(ctx, `select kind, coalesce(label_en, '') from court_calendar_day
		where day = $1 and (court_id is null or court_id = $2)`, date, courtID)
	if err != nil {
		return nil, fmt.Errorf("calendar check: %w", err)
	}
	for rows.Next() {
		var kind, label string
		if err := rows.Scan(&kind, &label); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, Warning{Code: "hearing.date_on_" + kind, Params: map[string]any{"label": label}})
	}
	rows.Close()
	clashRows, err := tx.Query(ctx, `
		select distinct ha.user_id, coalesce(u.display_name_en, u.display_name_si, '') from hearing_appearance ha
		join app_user u on u.id = ha.user_id
		join hearing h2 on h2.id <> $1 and h2.listing_status = 'scheduled' and h2.hearing_date = $2
		     and h2.court_id <> $3 and (h2.session = $4 or $4 = 'unspecified' or h2.session = 'unspecified')
		join hearing_appearance hb on hb.hearing_id = h2.id and hb.user_id = ha.user_id
		where ha.hearing_id = $1`, hearingID, date, courtID, session)
	if err != nil {
		return nil, fmt.Errorf("clash check: %w", err)
	}
	defer clashRows.Close()
	for clashRows.Next() {
		var userID uuid.UUID
		var name string
		if err := clashRows.Scan(&userID, &name); err != nil {
			return nil, err
		}
		out = append(out, Warning{Code: "hearing.appearance_clash", Params: map[string]any{"user_id": userID, "name": name}})
	}
	return out, clashRows.Err()
}

func addAppearance(ctx context.Context, tx pgx.Tx, p *authz.Principal, hearingID, matterID, userID uuid.UUID) error {
	walled, err := authz.UserWalledFrom(ctx, tx, userID, matterID)
	if err != nil {
		return err
	}
	if walled {
		return apperr.New(apperr.KindConflict, "wall.member_excluded")
	}
	var member bool
	if err := tx.QueryRow(ctx, `select exists (select 1 from matter_member where matter_id = $1 and user_id = $2
		and valid_to is null)`, matterID, userID).Scan(&member); err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	// Appearance-scoped access for non-members is Phase 2 (doc 09 §2.2); in
	// M1 the lawyer joins the team first.
	if !member {
		return ErrAppearanceNotMember
	}
	_, err = tx.Exec(ctx, `insert into hearing_appearance (firm_id, hearing_id, user_id, created_by)
		values ($1, $2, $3, $4) on conflict do nothing`, p.FirmID, hearingID, userID, p.UserID)
	if err != nil {
		return fmt.Errorf("add appearance: %w", err)
	}
	return nil
}

// hearingMatter loads a hearing's matter and status, checking access.
func hearingMatter(ctx context.Context, tx pgx.Tx, hearingID uuid.UUID, min authz.Access, lock bool) (uuid.UUID, string, authz.Access, error) {
	q := `select matter_id, listing_status from hearing where id = $1`
	if lock {
		q += ` for update`
	}
	var matterID uuid.UUID
	var status string
	if err := tx.QueryRow(ctx, q, hearingID).Scan(&matterID, &status); err != nil {
		if err == pgx.ErrNoRows {
			return uuid.Nil, "", authz.AccessNone, apperr.NotFound
		}
		return uuid.Nil, "", authz.AccessNone, fmt.Errorf("load hearing: %w", err)
	}
	access, _, err := authz.RequireMatter(ctx, tx, matterID, min)
	if err != nil {
		return uuid.Nil, "", authz.AccessNone, err
	}
	return matterID, status, access, nil
}

// AssertDate records "the date was recorded wrongly": a correction on the
// same hearing. Contradicting a verified date leaves the hearing in conflict
// until someone verifies the new date (I-HE-4).
func (s *Service) AssertDate(ctx context.Context, p *authz.Principal, hearingID uuid.UUID, c DateClaim) (*Detail, []Warning, error) {
	var warnings []Warning
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, status, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessFull, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingWrite); err != nil {
			return err
		}
		if status != StatusScheduled {
			return ErrIllegalTransition
		}
		claim, err := parseClaim(ctx, tx, c, "")
		if err != nil {
			return err
		}
		curID, _, _, err := currentAssertion(ctx, tx, hearingID)
		if err != nil {
			return err
		}
		aid, err := insertAssertion(ctx, tx, p, hearingID, claim, KindCorrection, &curID)
		if err != nil {
			return err
		}
		if err := recompute(ctx, tx, hearingID); err != nil {
			return err
		}
		if warnings, err = dateWarnings(ctx, tx, hearingID); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.date_asserted",
			TargetKind: "date_assertion", TargetID: aid, MatterID: matterID,
			Changes: map[string]any{"hearing_id": hearingID, "date": c.Date, "session": claim.session, "authority": claim.authority}})
	})
	if err != nil {
		return nil, nil, err
	}
	d, err := s.Get(ctx, p, hearingID)
	return d, warnings, err
}

// VacateInput records "the court changed the date before the day".
type VacateInput struct {
	Reason      string     `json:"reason"`
	Replacement *DateClaim `json:"replacement"`
}

// Vacate marks a hearing vacated and, when a replacement date is given,
// lists the replacement linked by replaces_hearing_id (I-HE-5).
func (s *Service) Vacate(ctx context.Context, p *authz.Principal, hearingID uuid.UUID, in VacateInput) (*Detail, []Warning, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return nil, nil, apperr.Invalid(apperr.Field("reason", "required"))
	}
	resultID := hearingID
	var warnings []Warning
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, status, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessFull, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingWrite); err != nil {
			return err
		}
		if status != StatusScheduled {
			return ErrIllegalTransition
		}
		if _, err := tx.Exec(ctx, `update hearing set listing_status = 'vacated', status_reason = $2, updated_by = $3
			where id = $1`, hearingID, in.Reason, p.UserID); err != nil {
			return fmt.Errorf("vacate: %w", err)
		}
		if in.Replacement != nil {
			newID, err := cloneHearing(ctx, tx, p, hearingID, *in.Replacement, KindReplacement, true, nil)
			if err != nil {
				return err
			}
			resultID = newID
			if warnings, err = dateWarnings(ctx, tx, newID); err != nil {
				return err
			}
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.vacated",
			TargetKind: "hearing", TargetID: hearingID, MatterID: matterID, Reason: in.Reason,
			Changes: map[string]any{"replacement_hearing_id": nullUUID(boolID(in.Replacement != nil, resultID))}})
	})
	if err != nil {
		return nil, nil, err
	}
	d, err := s.Get(ctx, p, resultID)
	return d, warnings, err
}

func boolID(ok bool, id uuid.UUID) uuid.UUID {
	if ok {
		return id
	}
	return uuid.Nil
}

// cloneHearing lists a new hearing for the same proceeding, carrying the
// court, purpose, clerk and appearances. replaces=true links a replacement;
// otherwise the new hearing is the next one in the chain.
func cloneHearing(ctx context.Context, tx pgx.Tx, p *authz.Principal, fromID uuid.UUID, c DateClaim, kind string, replaces bool, newID *uuid.UUID) (uuid.UUID, error) {
	claim, err := parseClaim(ctx, tx, c, "replacement.")
	if err != nil {
		return uuid.Nil, err
	}
	link := "previous_hearing_id"
	via := "outcome"
	if replaces {
		link, via = "replaces_hearing_id", "user"
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		insert into hearing (id, firm_id, proceeding_id, matter_id, court_id, courtroom_id, hearing_date, session, time_of_day,
		                     purpose_id, assigned_clerk_id, `+link+`, created_by, created_via)
		select coalesce($7, uuidv7()), firm_id, proceeding_id, matter_id, court_id, courtroom_id, $2, $3, $4, purpose_id,
		       assigned_clerk_id, id, $5, $6
		from hearing where id = $1 returning id`, fromID, claim.date, claim.session, claim.timeOfDay, p.UserID, via, newID).Scan(&id)
	if db.IsUniqueViolation(err, "hearing_pkey") {
		return uuid.Nil, apperr.New(apperr.KindConflict, "hearing.id_taken")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("list follow-on hearing: %w", err)
	}
	if _, err := insertAssertion(ctx, tx, p, id, claim, kind, nil); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `insert into hearing_appearance (firm_id, hearing_id, user_id, capacity, created_by)
		select firm_id, $2, user_id, capacity, $3 from hearing_appearance where hearing_id = $1`, fromID, id, p.UserID); err != nil {
		return uuid.Nil, fmt.Errorf("copy appearances: %w", err)
	}
	return id, recompute(ctx, tx, id)
}

// VerifyInput confirms the current date against an authoritative source.
type VerifyInput struct {
	DateAssertionID   uuid.UUID `json:"date_assertion_id"`
	SourceTypeID      uuid.UUID `json:"source_type_id"`
	EvidenceReference string    `json:"evidence_reference"`
}

// Verify confirms the current assertion (hearing.verify; clerks with diary
// access may verify). The source must be authoritative unless the firm
// setting verification_allow_primary is on (doc 06 §4.3,
// [NEEDS LAWYER CONFIRMATION]). AI never verifies.
func (s *Service) Verify(ctx context.Context, p *authz.Principal, hearingID uuid.UUID, in VerifyInput) (*Detail, error) {
	if strings.TrimSpace(in.EvidenceReference) == "" {
		return nil, apperr.Invalid(apperr.Field("evidence_reference", "required"))
	}
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, status, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessDiary, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingVerify); err != nil {
			return err
		}
		if status == StatusVacated || status == StatusCancelled {
			return ErrIllegalTransition
		}
		return verifyInTx(ctx, tx, p, hearingID, matterID, in)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, hearingID)
}

func verifyInTx(ctx context.Context, tx pgx.Tx, p *authz.Principal, hearingID, matterID uuid.UUID, in VerifyInput) error {
	curID, date, _, err := currentAssertion(ctx, tx, hearingID)
	if err != nil {
		return err
	}
	if in.DateAssertionID != curID {
		return ErrAssertionNotCurrent
	}
	_, attrs, err := courts.ItemByID(ctx, tx, courts.KindDateSourceType, in.SourceTypeID)
	if err != nil {
		return err
	}
	authority, _ := attrs["default_authority"].(string)
	if authority != courts.AuthorityAuthoritative {
		allowPrimary, err := firmFlag(ctx, tx, p.FirmID, "verification_allow_primary")
		if err != nil {
			return err
		}
		if !(allowPrimary && authority == courts.AuthorityPrimary) {
			return ErrSourceNotAuthoritative
		}
	}
	var vid uuid.UUID
	if err := tx.QueryRow(ctx, `insert into verification (firm_id, hearing_id, date_assertion_id, source_type_id,
		evidence_reference, verified_by) values ($1, $2, $3, $4, $5, $6) returning id`,
		p.FirmID, hearingID, curID, in.SourceTypeID, strings.TrimSpace(in.EvidenceReference), p.UserID).Scan(&vid); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if err := recompute(ctx, tx, hearingID); err != nil {
		return err
	}
	return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.verified",
		TargetKind: "verification", TargetID: vid, MatterID: matterID,
		Changes: map[string]any{"hearing_id": hearingID, "date": clock.FormatDate(date)}})
}

func firmFlag(ctx context.Context, tx pgx.Tx, firmID uuid.UUID, key string) (bool, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `select settings from firm where id = $1`, firmID).Scan(&raw); err != nil {
		return false, fmt.Errorf("firm settings: %w", err)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	v, _ := m[key].(bool)
	return v, nil
}

// RevokeVerification withdraws a verification with a reason.
func (s *Service) RevokeVerification(ctx context.Context, p *authz.Principal, hearingID, verificationID uuid.UUID, reason string) (*Detail, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, apperr.Invalid(apperr.Field("reason", "required"))
	}
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, _, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessDiary, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingVerify); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `update verification set revoked_at = now(), revoked_by = $3, revoke_reason = $4
			where id = $1 and hearing_id = $2 and revoked_at is null`, verificationID, hearingID, p.UserID, reason)
		if err != nil {
			return fmt.Errorf("revoke verification: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound
		}
		if err := recompute(ctx, tx, hearingID); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.verification_revoked",
			TargetKind: "verification", TargetID: verificationID, MatterID: matterID, Reason: reason})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, hearingID)
}

// transitions is the listing-status state machine set by hand. Recording an
// outcome and vacating have their own operations.
var transitions = map[string]map[string]bool{
	StatusScheduled: {StatusHeld: true, StatusNotReached: true, StatusCourtNotSitting: true, StatusCancelled: true},
}

// SetListingStatus moves a scheduled hearing to held, not reached, court not
// sitting or cancelled (cancel needs a reason).
func (s *Service) SetListingStatus(ctx context.Context, p *authz.Principal, hearingID uuid.UUID, to, reason string) (*Detail, error) {
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, from, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessFull, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingWrite); err != nil {
			return err
		}
		if !transitions[from][to] {
			return ErrIllegalTransition.WithParams(map[string]any{"from": from, "to": to})
		}
		if to == StatusCancelled && strings.TrimSpace(reason) == "" {
			return apperr.Invalid(apperr.Field("reason", "required"))
		}
		if _, err := tx.Exec(ctx, `update hearing set listing_status = $2, status_reason = $3, updated_by = $4 where id = $1`,
			hearingID, to, nullable(reason), p.UserID); err != nil {
			return fmt.Errorf("set listing status: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.status_changed",
			TargetKind: "hearing", TargetID: hearingID, MatterID: matterID, Reason: reason,
			Changes: map[string]any{"from": from, "to": to}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, hearingID)
}

// NextHearingInput is the next date given in court.
type NextHearingInput struct {
	ID          *uuid.UUID `json:"id"` // client-generated (offline queue)
	Date        string     `json:"date"`
	Session     string     `json:"session"`
	Time        string     `json:"time"`
	PurposeID   *uuid.UUID `json:"purpose_id"`
	PurposeNote string     `json:"purpose_note"`
	SourceNote  string     `json:"source_detail"`
}

// OutcomeInput records what happened at a hearing (doc 06 §6.4).
type OutcomeInput struct {
	OutcomeTypeID    uuid.UUID         `json:"outcome_type_id"`
	Summary          string            `json:"summary"`
	OrderText        string            `json:"order_text"`
	Next             *NextHearingInput `json:"next_hearing"`
	NoNextReason     string            `json:"no_next_reason"`
	NoNextNote       string            `json:"no_next_note"`
	InformClient     *bool             `json:"inform_client"`
	CertifiedCopy    bool              `json:"certified_copy"`
	AppearedUserID   *uuid.UUID        `json:"appeared_user_id"`
	ClientRecordedAt *time.Time        `json:"client_recorded_at"`
}

// RecordOutcome records an outcome atomically (I-OU-2): the outcome, the next
// hearing with a from_outcome assertion (announced in court: primary
// authority, so unverified until the clerk checks the record), the chain link
// and the status change — all or nothing. A concluding outcome with no next
// date concludes the proceeding.
func (s *Service) RecordOutcome(ctx context.Context, p *authz.Principal, hearingID uuid.UUID, in OutcomeInput) (*Detail, []Warning, error) {
	if (in.Next == nil) == (strings.TrimSpace(in.NoNextReason) == "") {
		return nil, nil, ErrOutcomeNeedsNextOrReason
	}
	var warnings []Warning
	switch in.NoNextReason {
	case "", "concluded", "laid_by", "date_from_registry", "other":
	default:
		return nil, nil, apperr.Invalid(apperr.Field("no_next_reason", "invalid"))
	}
	inform := true
	if in.InformClient != nil {
		inform = *in.InformClient
	}
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, status, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessFull, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingOutcome); err != nil {
			return err
		}
		switch status {
		case StatusScheduled, StatusHeld, StatusNotReached, StatusCourtNotSitting:
		case StatusOutcomeRecorded:
			return ErrOutcomeExists
		default:
			return ErrIllegalTransition
		}
		var date time.Time
		var proceedingID uuid.UUID
		if err := tx.QueryRow(ctx, `select hearing_date, proceeding_id from hearing where id = $1`, hearingID).
			Scan(&date, &proceedingID); err != nil {
			return fmt.Errorf("load hearing: %w", err)
		}
		if date.After(clock.Today()) {
			return ErrOutcomeInFuture
		}
		_, attrs, err := courts.ItemByID(ctx, tx, courts.KindOutcomeType, in.OutcomeTypeID)
		if err != nil {
			return err
		}

		var nextID *uuid.UUID
		if in.Next != nil {
			if in.Next.PurposeID != nil {
				if _, _, err := courts.ItemByID(ctx, tx, courts.KindHearingPurpose, *in.Next.PurposeID); err != nil {
					return err
				}
			}
			srcID, _, err := courts.ItemByCode(ctx, tx, courts.KindDateSourceType, "court_order")
			if err != nil {
				return err
			}
			detail := in.Next.SourceNote
			if detail == "" {
				detail = "Court order at hearing on " + clock.FormatDate(date)
			}
			id, err := cloneHearing(ctx, tx, p, hearingID, DateClaim{Date: in.Next.Date, Session: in.Next.Session,
				Time: in.Next.Time, SourceTypeID: srcID, SourceDetail: detail}, KindFromOutcome, false, in.Next.ID)
			if err != nil {
				return err
			}
			next, _ := clock.ParseDate(in.Next.Date)
			if !next.After(date) {
				return apperr.Invalid(apperr.Field("next_hearing.date", "hearing.next_date_not_after"))
			}
			if _, err := tx.Exec(ctx, `update hearing set purpose_id = coalesce($2, purpose_id), purpose_note = $3
				where id = $1`, id, in.Next.PurposeID, nullable(in.Next.PurposeNote)); err != nil {
				return fmt.Errorf("next purpose: %w", err)
			}
			if warnings, err = dateWarnings(ctx, tx, id); err != nil {
				return err
			}
			nextID = &id
		}

		var outcomeID uuid.UUID
		if err := tx.QueryRow(ctx, `insert into outcome (firm_id, hearing_id, outcome_type_id, summary, order_text,
			next_hearing_id, no_next_reason, no_next_note, inform_client, certified_copy, appeared_user_id, recorded_by,
			client_recorded_at) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) returning id`,
			p.FirmID, hearingID, in.OutcomeTypeID, nullable(in.Summary), nullable(in.OrderText), nextID,
			nullable(in.NoNextReason), nullable(in.NoNextNote), inform, in.CertifiedCopy, in.AppearedUserID, p.UserID,
			in.ClientRecordedAt).Scan(&outcomeID); err != nil {
			if db.IsUniqueViolation(err, "outcome_one_original") {
				return ErrOutcomeExists
			}
			return fmt.Errorf("record outcome: %w", err)
		}
		if _, err := tx.Exec(ctx, `update hearing set listing_status = 'outcome_recorded', updated_by = $2 where id = $1`,
			hearingID, p.UserID); err != nil {
			return fmt.Errorf("mark outcome recorded: %w", err)
		}
		if concludes, _ := attrs["concludes"].(bool); concludes && nextID == nil {
			if _, err := tx.Exec(ctx, `update proceeding set status = 'concluded', outcome_type_id = $2, updated_by = $3
				where id = $1 and status = 'active'`, proceedingID, in.OutcomeTypeID, p.UserID); err != nil {
				return fmt.Errorf("conclude proceeding: %w", err)
			}
		}
		if in.NoNextReason == "laid_by" {
			if _, err := tx.Exec(ctx, `update proceeding set status = 'dormant', updated_by = $2 where id = $1 and status = 'active'`,
				proceedingID, p.UserID); err != nil {
				return fmt.Errorf("lay by proceeding: %w", err)
			}
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.outcome_recorded",
			TargetKind: "outcome", TargetID: outcomeID, MatterID: matterID,
			Changes: map[string]any{"hearing_id": hearingID, "next_hearing_id": nextID, "no_next_reason": in.NoNextReason}})
	})
	if err != nil {
		return nil, nil, err
	}
	d, err := s.Get(ctx, p, hearingID)
	return d, warnings, err
}

// CorrectOutcome appends a correction to a recorded outcome's text and type.
// The next-hearing link is not changed here: a wrong next date is corrected
// on that hearing with a date assertion.
func (s *Service) CorrectOutcome(ctx context.Context, p *authz.Principal, hearingID uuid.UUID, in OutcomeInput) (*Detail, error) {
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, status, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessFull, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingOutcome); err != nil {
			return err
		}
		if status != StatusOutcomeRecorded {
			return ErrIllegalTransition
		}
		if _, _, err := courts.ItemByID(ctx, tx, courts.KindOutcomeType, in.OutcomeTypeID); err != nil {
			return err
		}
		var prevID uuid.UUID
		var next *uuid.UUID
		var noNext, noNextNote *string
		if err := tx.QueryRow(ctx, `select id, next_hearing_id, no_next_reason, no_next_note from outcome o
			where hearing_id = $1 and not exists (select 1 from outcome c where c.corrects_id = o.id)
			order by recorded_at desc limit 1`, hearingID).Scan(&prevID, &next, &noNext, &noNextNote); err != nil {
			return fmt.Errorf("load outcome: %w", err)
		}
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `insert into outcome (firm_id, hearing_id, outcome_type_id, summary, order_text,
			next_hearing_id, no_next_reason, no_next_note, corrects_id, recorded_by) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			returning id`, p.FirmID, hearingID, in.OutcomeTypeID, nullable(in.Summary), nullable(in.OrderText), next, noNext,
			noNextNote, prevID, p.UserID).Scan(&id); err != nil {
			return fmt.Errorf("correct outcome: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.outcome_corrected",
			TargetKind: "outcome", TargetID: id, MatterID: matterID, Changes: map[string]any{"corrects_id": prevID}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, hearingID)
}

// UpdateInput changes non-date fields of a hearing.
type UpdateInput struct {
	PurposeID         *uuid.UUID `json:"purpose_id"`
	PurposeNote       *string    `json:"purpose_note"`
	CourtroomID       *uuid.UUID `json:"courtroom_id"`
	Instructions      *string    `json:"instructions"`
	Notes             *string    `json:"notes"`
	AssignedClerkID   *uuid.UUID `json:"assigned_clerk_id"`
	PreparationStatus *string    `json:"preparation_status"`
	PreparationNote   *string    `json:"preparation_note"`
	// Present only to reject date edits with a clear code.
	Date    *string `json:"date"`
	Session *string `json:"session"`
}

// Update changes non-date fields (If-Match). Date changes are refused: they
// are assertions or a vacate.
func (s *Service) Update(ctx context.Context, p *authz.Principal, hearingID uuid.UUID, version int, in UpdateInput) (*Detail, error) {
	if in.Date != nil || in.Session != nil {
		return nil, ErrDateChangeNeedsAssertion
	}
	if in.PreparationStatus != nil {
		switch *in.PreparationStatus {
		case "not_started", "in_progress", "ready", "blocked":
		default:
			return nil, apperr.Invalid(apperr.Field("preparation_status", "invalid"))
		}
	}
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, _, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessFull, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingWrite); err != nil {
			return err
		}
		if in.PurposeID != nil {
			if _, _, err := courts.ItemByID(ctx, tx, courts.KindHearingPurpose, *in.PurposeID); err != nil {
				return err
			}
		}
		var current int
		if err := tx.QueryRow(ctx, `select row_version from hearing where id = $1`, hearingID).Scan(&current); err != nil {
			return fmt.Errorf("load hearing: %w", err)
		}
		if current != version {
			return apperr.VersionMismatch
		}
		if _, err := tx.Exec(ctx, `update hearing set purpose_id = coalesce($2, purpose_id),
			purpose_note = coalesce($3, purpose_note), courtroom_id = coalesce($4, courtroom_id),
			instructions = coalesce($5, instructions), notes = coalesce($6, notes),
			assigned_clerk_id = coalesce($7, assigned_clerk_id), preparation_status = coalesce($8, preparation_status),
			preparation_note = coalesce($9, preparation_note), updated_by = $10 where id = $1`,
			hearingID, in.PurposeID, in.PurposeNote, in.CourtroomID, in.Instructions, in.Notes, in.AssignedClerkID,
			in.PreparationStatus, in.PreparationNote, p.UserID); err != nil {
			if code, _, ok := db.Constraint(err); ok && code == "23503" {
				return apperr.Invalid(apperr.Field("courtroom_id", "reference.unknown"))
			}
			return fmt.Errorf("update hearing: %w", err)
		}
		changes := map[string]any{}
		if in.PreparationStatus != nil {
			changes["preparation_status"] = *in.PreparationStatus
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.updated",
			TargetKind: "hearing", TargetID: hearingID, MatterID: matterID, Changes: changes})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, hearingID)
}

// AddAppearance assigns an appearing lawyer (must be on the team, not walled).
func (s *Service) AddAppearance(ctx context.Context, p *authz.Principal, hearingID, userID uuid.UUID) ([]Warning, error) {
	var warnings []Warning
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, status, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessFull, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingWrite); err != nil {
			return err
		}
		if status != StatusScheduled {
			return ErrIllegalTransition
		}
		if err := addAppearance(ctx, tx, p, hearingID, matterID, userID); err != nil {
			return err
		}
		if warnings, err = dateWarnings(ctx, tx, hearingID); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.appearance_added",
			TargetKind: "user", TargetID: userID, MatterID: matterID, Changes: map[string]any{"hearing_id": hearingID}})
	})
	return warnings, err
}

// RemoveAppearance unassigns an appearing lawyer.
func (s *Service) RemoveAppearance(ctx context.Context, p *authz.Principal, hearingID, userID uuid.UUID) error {
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, _, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessFull, true)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingWrite); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `delete from hearing_appearance where hearing_id = $1 and user_id = $2`, hearingID, userID)
		if err != nil {
			return fmt.Errorf("remove appearance: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.appearance_removed",
			TargetKind: "user", TargetID: userID, MatterID: matterID, Changes: map[string]any{"hearing_id": hearingID}})
	})
}

// AddNote appends a court-day note.
func (s *Service) AddNote(ctx context.Context, p *authz.Principal, hearingID uuid.UUID, body string, clientAt *time.Time) (uuid.UUID, error) {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 20000 {
		return uuid.Nil, apperr.Invalid(apperr.Field("body", "required"))
	}
	var id uuid.UUID
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, _, _, err := hearingMatter(ctx, tx, hearingID, authz.AccessFull, false)
		if err != nil {
			return err
		}
		if err := p.Require(authz.HearingWrite); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `insert into hearing_note (firm_id, hearing_id, body, created_by, client_recorded_at)
			values ($1, $2, $3, $4, $5) returning id`, p.FirmID, hearingID, body, p.UserID, clientAt).Scan(&id); err != nil {
			return fmt.Errorf("add note: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "hearing.note_added",
			TargetKind: "hearing_note", TargetID: id, MatterID: matterID})
	})
	return id, err
}

func nullable(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func nullUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
