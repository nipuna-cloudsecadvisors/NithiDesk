// Package proceedings owns court proceedings within a matter (doc 05 §4–7):
// case-number history, the relationship graph (appeal, revision, transfer)
// and parties with their roles per proceeding.
package proceedings

import (
	"context"
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
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/textnorm"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/registry"
)

// Warning is a non-blocking notice returned with a successful write.
type Warning struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// CaseNumber is one entry in a proceeding's number history.
type CaseNumber struct {
	ID         uuid.UUID `json:"id"`
	CourtID    uuid.UUID `json:"court_id"`
	Raw        string    `json:"raw"`
	Normalised string    `json:"normalised"`
	ValidFrom  string    `json:"valid_from"`
	ValidTo    *string   `json:"valid_to,omitempty"`
	Reason     string    `json:"reason"`
	Current    bool      `json:"current"`
}

// Party is an entity's role in a proceeding.
type Party struct {
	ID         uuid.UUID     `json:"id"`
	EntityID   uuid.UUID     `json:"entity_id"`
	Name       registry.Name `json:"name"`
	RoleTypeID uuid.UUID     `json:"party_role_type_id"`
	RoleCode   string        `json:"party_role_code"`
	Ordinal    *int          `json:"ordinal,omitempty"`
	Side       string        `json:"side"`
	Status     string        `json:"status"`
}

// Relationship is a typed link to another proceeding.
type Relationship struct {
	ID              uuid.UUID `json:"id"`
	Direction       string    `json:"direction"` // outgoing: this → other
	OtherID         uuid.UUID `json:"other_proceeding_id"`
	TypeCode        string    `json:"relationship_code"`
	OtherCaseNumber *string   `json:"other_case_number,omitempty"`
	Note            *string   `json:"note,omitempty"`
}

// Proceeding is a proceeding page.
type Proceeding struct {
	ID            uuid.UUID      `json:"id"`
	MatterID      uuid.UUID      `json:"matter_id"`
	TypeID        uuid.UUID      `json:"proceeding_type_id"`
	TypeCode      string         `json:"proceeding_type_code"`
	CourtID       uuid.UUID      `json:"court_id"`
	Title         registry.Name  `json:"title"`
	OurSide       *string        `json:"our_side,omitempty"`
	OurRole       string         `json:"our_role"`
	InstitutedOn  *string        `json:"instituted_on,omitempty"`
	Status        string         `json:"status"`
	CurrentCaseNo *string        `json:"current_case_number,omitempty"`
	CaseNumbers   []CaseNumber   `json:"case_numbers"`
	Parties       []Party        `json:"parties"`
	Relationships []Relationship `json:"relationships"`
	Access        string         `json:"access"`
	Version       int            `json:"row_version"`
}

// Service implements proceedings.
type Service struct {
	pool *pgxpool.Pool
}

// New builds the service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func actorOf(p *authz.Principal) audit.Actor {
	return audit.Actor{Kind: audit.ActorUser, UserID: p.UserID, SessionID: p.SessionID, RequestID: p.RequestID, IP: p.IP}
}

// PartyInput adds a party.
type PartyInput struct {
	EntityID   *uuid.UUID      `json:"entity_id"`
	New        *registry.Input `json:"new"`
	RoleTypeID uuid.UUID       `json:"party_role_type_id"`
	Ordinal    *int            `json:"ordinal"`
	Side       string          `json:"side"`
}

// CreateInput creates a proceeding.
type CreateInput struct {
	TypeID       uuid.UUID    `json:"proceeding_type_id"`
	CourtID      uuid.UUID    `json:"court_id"`
	CaseNumber   string       `json:"case_number"`
	TitleEN      string       `json:"title_en"`
	TitleSI      string       `json:"title_si"`
	OurSide      string       `json:"our_side"`
	OurRole      string       `json:"our_role"`
	InstitutedOn string       `json:"instituted_on"`
	Parties      []PartyInput `json:"parties"`
	// Importer only.
	CreatedVia    string    `json:"-"`
	ImportBatchID uuid.UUID `json:"-"`
	CaseReason    string    `json:"-"`
}

var ourSides = map[string]bool{"plaintiff_side": true, "defendant_side": true, "petitioner": true, "respondent": true,
	"appellant": true, "accused": true, "complainant": true, "intervenient": true, "other": true}
var ourRoles = map[string]bool{"attorney_on_record": true, "counsel_only": true, "watching_brief": true}
var partySides = map[string]bool{"our_client": true, "aligned": true, "opposing": true, "neutral": true}

// Create adds a proceeding to a matter (proceeding.write, full matter access).
func (s *Service) Create(ctx context.Context, p *authz.Principal, matterID uuid.UUID, in CreateInput) (*Proceeding, []Warning, error) {
	var id uuid.UUID
	var warnings []Warning
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		if _, _, err := authz.RequireMatter(ctx, tx, matterID, authz.AccessFull); err != nil {
			return err
		}
		if err := p.Require(authz.ProceedingWrite); err != nil {
			return err
		}
		var err error
		id, warnings, err = CreateInTx(ctx, tx, p, matterID, in)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	out, err := s.Get(ctx, p, id)
	return out, warnings, err
}

// CreateInTx creates a proceeding inside tx; the caller checked access.
func CreateInTx(ctx context.Context, tx pgx.Tx, p *authz.Principal, matterID uuid.UUID, in CreateInput) (uuid.UUID, []Warning, error) {
	var fields []apperr.FieldError
	if in.OurRole == "" {
		in.OurRole = "attorney_on_record"
	}
	if !ourRoles[in.OurRole] {
		fields = append(fields, apperr.Field("our_role", "invalid"))
	}
	if in.OurSide != "" && !ourSides[in.OurSide] {
		fields = append(fields, apperr.Field("our_side", "invalid"))
	}
	var instituted *time.Time
	if in.InstitutedOn != "" {
		t, err := clock.ParseDate(in.InstitutedOn)
		if err != nil {
			fields = append(fields, apperr.Field("instituted_on", "date.invalid"))
		}
		instituted = &t
	}
	if len(fields) > 0 {
		return uuid.Nil, nil, apperr.Invalid(fields...)
	}
	if _, _, err := courts.ItemByID(ctx, tx, courts.KindProceedingType, in.TypeID); err != nil {
		return uuid.Nil, nil, err
	}
	en := strings.TrimSpace(textnorm.Normalise(in.TitleEN, true))
	si := strings.TrimSpace(textnorm.Normalise(in.TitleSI, true))
	via := in.CreatedVia
	if via == "" {
		via = "user"
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		insert into proceeding (firm_id, matter_id, proceeding_type_id, current_court_id, title_en, title_si, title_latin,
		                        our_side, our_role, instituted_on, created_by, created_via, import_batch_id)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) returning id`,
		p.FirmID, matterID, in.TypeID, in.CourtID, nullable(en), nullable(si), latin(en, si), nullable(in.OurSide),
		in.OurRole, instituted, p.UserID, via, nullUUID(in.ImportBatchID)).Scan(&id)
	if code, _, ok := db.Constraint(err); ok && code == "23503" {
		return uuid.Nil, nil, apperr.Invalid(apperr.Field("court_id", "reference.unknown"))
	}
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("create proceeding: %w", err)
	}
	var warnings []Warning
	if strings.TrimSpace(in.CaseNumber) != "" {
		reason := in.CaseReason
		if reason == "" {
			reason = "assigned"
		}
		w, err := addCaseNumber(ctx, tx, p, id, matterID, CaseNumberInput{CourtID: in.CourtID, Raw: in.CaseNumber, Reason: reason})
		if err != nil {
			return uuid.Nil, nil, err
		}
		warnings = append(warnings, w...)
	}
	for i, party := range in.Parties {
		if _, err := addParty(ctx, tx, p, id, matterID, party, via, fmt.Sprintf("parties[%d].", i)); err != nil {
			return uuid.Nil, nil, err
		}
	}
	if en == "" && si == "" {
		if err := generateTitle(ctx, tx, id); err != nil {
			return uuid.Nil, nil, err
		}
	}
	return id, warnings, audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "proceeding.created",
		TargetKind: "proceeding", TargetID: id, MatterID: matterID, Changes: map[string]any{"via": via}})
}

// generateTitle builds "Perera v. Silva (+2)" from the first applicant-side
// and respondent-side parties when no title was given.
func generateTitle(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	rows, err := tx.Query(ctx, `select coalesce(e.name_en, e.name_si), ri.attributes->>'side', e.name_si
		from party_role pr join entity e on e.id = pr.entity_id join reference_item ri on ri.id = pr.party_role_type_id
		where pr.proceeding_id = $1 and pr.status = 'active' and ri.attributes->>'side' in ('applicant_side', 'respondent_side')
		order by ri.sort_order, coalesce(pr.ordinal, 0), pr.created_at`, id)
	if err != nil {
		return fmt.Errorf("title parties: %w", err)
	}
	var appEN, resEN, appSI, resSI string
	extra := 0
	for rows.Next() {
		var name, side string
		var si *string
		if err := rows.Scan(&name, &side, &si); err != nil {
			rows.Close()
			return fmt.Errorf("title parties: %w", err)
		}
		siName := name
		if si != nil {
			siName = *si
		}
		switch {
		case side == "applicant_side" && appEN == "":
			appEN, appSI = name, siName
		case side == "respondent_side" && resEN == "":
			resEN, resSI = name, siName
		default:
			extra++
		}
	}
	rows.Close()
	if appEN == "" || resEN == "" {
		return rows.Err()
	}
	en := appEN + " v. " + resEN
	si := appSI + " එදිරිව " + resSI
	if extra > 0 {
		en += fmt.Sprintf(" (+%d)", extra)
		si += fmt.Sprintf(" (+%d)", extra)
	}
	_, err = tx.Exec(ctx, `update proceeding set title_en = $2, title_si = $3, title_latin = $4 where id = $1`,
		id, en, si, latin(en, si))
	return err
}

// CaseNumberInput adds a number to a proceeding's history.
type CaseNumberInput struct {
	CourtID   uuid.UUID `json:"court_id"`
	Raw       string    `json:"raw"`
	Reason    string    `json:"reason"`
	ValidFrom string    `json:"valid_from"`
	Source    string    `json:"source"`
}

func addCaseNumber(ctx context.Context, tx pgx.Tx, p *authz.Principal, proceedingID, matterID uuid.UUID, in CaseNumberInput) ([]Warning, error) {
	in.Raw = strings.TrimSpace(in.Raw)
	switch in.Reason {
	case "assigned", "corrected", "renumbered", "transferred", "migrated":
	default:
		return nil, apperr.Invalid(apperr.Field("reason", "invalid"))
	}
	if in.Raw == "" || len(in.Raw) > 80 {
		return nil, apperr.Invalid(apperr.Field("raw", "required"))
	}
	from := clock.Today()
	if in.ValidFrom != "" {
		t, err := clock.ParseDate(in.ValidFrom)
		if err != nil {
			return nil, apperr.Invalid(apperr.Field("valid_from", "date.invalid"))
		}
		from = t
	}
	check, err := courts.CheckCaseNumber(ctx, tx, in.CourtID, in.Raw)
	if err != nil {
		return nil, err
	}
	var warnings []Warning
	if !check.MatchesPattern {
		warnings = append(warnings, Warning{Code: "case_number.unusual_format", Params: map[string]any{"example": check.Example}})
	}
	// I-PR-3: a duplicate is a warning that names the other matter only if
	// the caller can see it.
	var otherMatter *uuid.UUID
	var otherNumber *string
	var visible bool
	err = tx.QueryRow(ctx, `
		select pr.matter_id, m.matter_number, exists (select 1 from nd_visible_matters() vm where vm.matter_id = pr.matter_id)
		from case_number_record c join proceeding pr on pr.id = c.proceeding_id join matter m on m.id = pr.matter_id
		where c.court_id = $1 and c.normalised = $2 and c.valid_to is null and pr.id <> $3 and pr.deleted_at is null
		limit 1`, in.CourtID, check.Normalised, proceedingID).Scan(&otherMatter, &otherNumber, &visible)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("duplicate check: %w", err)
	}
	if err == nil {
		if visible {
			warnings = append(warnings, Warning{Code: "case_number.possible_duplicate",
				Params: map[string]any{"matter_id": otherMatter, "matter_number": otherNumber}})
		} else {
			warnings = append(warnings, Warning{Code: "case_number.possible_duplicate_hidden"})
		}
	}
	if _, err := tx.Exec(ctx, `update case_number_record set valid_to = $2, ended_by = $3
		where proceeding_id = $1 and valid_to is null`, proceedingID, from, p.UserID); err != nil {
		return nil, fmt.Errorf("end current case number: %w", err)
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `insert into case_number_record (firm_id, proceeding_id, court_id, raw, normalised,
		valid_from, reason, source, created_by) values ($1, $2, $3, $4, $5, $6, $7, $8, $9) returning id`,
		p.FirmID, proceedingID, in.CourtID, in.Raw, check.Normalised, from, in.Reason, nullable(in.Source), p.UserID).
		Scan(&id); err != nil {
		return nil, fmt.Errorf("add case number: %w", err)
	}
	// A transfer (or any number at another court) moves the proceeding's
	// current court; the old number and court stay in the history.
	if _, err := tx.Exec(ctx, `update proceeding set current_court_id = $2, updated_by = $3 where id = $1
		and current_court_id <> $2`, proceedingID, in.CourtID, p.UserID); err != nil {
		return nil, fmt.Errorf("update current court: %w", err)
	}
	return warnings, audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "proceeding.case_number_added",
		TargetKind: "case_number", TargetID: id, MatterID: matterID,
		Changes: map[string]any{"raw": in.Raw, "reason": in.Reason, "court_id": in.CourtID}})
}

// AddCaseNumber adds a number to the history; the current one is end-dated.
func (s *Service) AddCaseNumber(ctx context.Context, p *authz.Principal, proceedingID uuid.UUID, in CaseNumberInput) ([]Warning, error) {
	var warnings []Warning
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, err := s.requireProceeding(ctx, tx, proceedingID, authz.AccessFull)
		if err != nil {
			return err
		}
		if err := p.Require(authz.ProceedingWrite); err != nil {
			return err
		}
		warnings, err = addCaseNumber(ctx, tx, p, proceedingID, matterID, in)
		return err
	})
	return warnings, err
}

func addParty(ctx context.Context, tx pgx.Tx, p *authz.Principal, proceedingID, matterID uuid.UUID, in PartyInput, via, prefix string) (uuid.UUID, error) {
	if in.Side == "" {
		in.Side = "neutral"
	}
	if !partySides[in.Side] {
		return uuid.Nil, apperr.Invalid(apperr.Field(prefix+"side", "invalid"))
	}
	if _, _, err := courts.ItemByID(ctx, tx, courts.KindPartyRoleType, in.RoleTypeID); err != nil {
		return uuid.Nil, err
	}
	entityID := uuid.Nil
	switch {
	case in.EntityID != nil:
		entityID = *in.EntityID
	case in.New != nil:
		if fields := in.New.Validate(prefix + "new."); len(fields) > 0 {
			return uuid.Nil, apperr.Invalid(fields...)
		}
		createdVia := "user"
		if via == "import" {
			createdVia = "import"
			if existing, ok, err := registry.FindExact(ctx, tx, *in.New); err != nil {
				return uuid.Nil, err
			} else if ok {
				entityID = existing
			}
		}
		if entityID == uuid.Nil {
			id, err := registry.Create(ctx, tx, p.FirmID, p.UserID, *in.New, createdVia)
			if err != nil {
				return uuid.Nil, err
			}
			entityID = id
		}
	default:
		return uuid.Nil, apperr.Invalid(apperr.Field(prefix+"entity_id", "required"))
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `insert into party_role (firm_id, proceeding_id, entity_id, party_role_type_id, ordinal, side,
		created_by) values ($1, $2, $3, $4, $5, $6, $7) returning id`,
		p.FirmID, proceedingID, entityID, in.RoleTypeID, in.Ordinal, in.Side, p.UserID).Scan(&id)
	if db.IsUniqueViolation(err, "party_role_ordinal") {
		return uuid.Nil, apperr.New(apperr.KindConflict, "party.ordinal_taken")
	}
	if code, _, ok := db.Constraint(err); ok && code == "23503" {
		return uuid.Nil, apperr.Invalid(apperr.Field(prefix+"entity_id", "entity.unknown"))
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("add party: %w", err)
	}
	return id, audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "proceeding.party_added",
		TargetKind: "party_role", TargetID: id, MatterID: matterID})
}

// AddParty adds a party to a proceeding.
func (s *Service) AddParty(ctx context.Context, p *authz.Principal, proceedingID uuid.UUID, in PartyInput) error {
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, err := s.requireProceeding(ctx, tx, proceedingID, authz.AccessFull)
		if err != nil {
			return err
		}
		if err := p.Require(authz.ProceedingWrite); err != nil {
			return err
		}
		_, err = addParty(ctx, tx, p, proceedingID, matterID, in, "user", "")
		return err
	})
}

// RelationshipInput links this proceeding to another.
type RelationshipInput struct {
	ToProceedingID uuid.UUID `json:"to_proceeding_id"`
	TypeID         uuid.UUID `json:"relationship_type_id"`
	Note           string    `json:"note"`
}

// AddRelationship links two proceedings of the same matter. Derivation types
// (appeal, revision, transfer…) may not form a cycle (I-PR-5).
func (s *Service) AddRelationship(ctx context.Context, p *authz.Principal, fromID uuid.UUID, in RelationshipInput) error {
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, err := s.requireProceeding(ctx, tx, fromID, authz.AccessFull)
		if err != nil {
			return err
		}
		if err := p.Require(authz.ProceedingWrite); err != nil {
			return err
		}
		if in.ToProceedingID == fromID {
			return apperr.Invalid(apperr.Field("to_proceeding_id", "proceeding.self_link"))
		}
		var otherMatter uuid.UUID
		if err := tx.QueryRow(ctx, `select matter_id from proceeding where id = $1 and deleted_at is null`, in.ToProceedingID).
			Scan(&otherMatter); err != nil || otherMatter != matterID {
			// I-PR-6: cross-matter links need an explicit matter link (M2).
			return apperr.Invalid(apperr.Field("to_proceeding_id", "proceeding.not_in_matter"))
		}
		_, attrs, err := courts.ItemByID(ctx, tx, courts.KindRelationshipType, in.TypeID)
		if err != nil {
			return err
		}
		from, to := fromID, in.ToProceedingID
		if sym, _ := attrs["symmetric"].(bool); sym && to.String() < from.String() {
			from, to = to, from
		}
		if deriv, _ := attrs["derivation"].(bool); deriv {
			var cycle bool
			if err := tx.QueryRow(ctx, `
				with recursive up(id) as (
					select $2::uuid
					union
					select r.to_proceeding_id from proceeding_relationship r
					join reference_item t on t.id = r.relationship_type_id and (t.attributes->>'derivation')::boolean
					join up on r.from_proceeding_id = up.id)
				select exists (select 1 from up where id = $1)`, from, to).Scan(&cycle); err != nil {
				return fmt.Errorf("cycle check: %w", err)
			}
			if cycle {
				return apperr.New(apperr.KindConflict, "proceeding.relationship_cycle")
			}
		}
		var id uuid.UUID
		err = tx.QueryRow(ctx, `insert into proceeding_relationship (firm_id, from_proceeding_id, to_proceeding_id,
			relationship_type_id, note, created_by) values ($1, $2, $3, $4, $5, $6) returning id`,
			p.FirmID, from, to, in.TypeID, nullable(in.Note), p.UserID).Scan(&id)
		if db.IsUniqueViolation(err, "") {
			return apperr.New(apperr.KindConflict, "proceeding.relationship_exists")
		}
		if err != nil {
			return fmt.Errorf("add relationship: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "proceeding.linked",
			TargetKind: "proceeding_relationship", TargetID: id, MatterID: matterID})
	})
}

// requireProceeding returns the proceeding's matter after the access check;
// a deleted or invisible proceeding is not found.
func (s *Service) requireProceeding(ctx context.Context, tx pgx.Tx, id uuid.UUID, min authz.Access) (uuid.UUID, error) {
	return RequireProceeding(ctx, tx, id, min)
}

// RequireProceeding is the access check shared with other modules.
func RequireProceeding(ctx context.Context, tx pgx.Tx, id uuid.UUID, min authz.Access) (uuid.UUID, error) {
	var matterID uuid.UUID
	err := tx.QueryRow(ctx, `select matter_id from proceeding where id = $1 and deleted_at is null`, id).Scan(&matterID)
	if err == pgx.ErrNoRows {
		return uuid.Nil, apperr.NotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("load proceeding: %w", err)
	}
	if _, _, err := authz.RequireMatter(ctx, tx, matterID, min); err != nil {
		return uuid.Nil, err
	}
	return matterID, nil
}

// Get returns a proceeding page (diary access or better).
func (s *Service) Get(ctx context.Context, p *authz.Principal, id uuid.UUID) (*Proceeding, error) {
	var out Proceeding
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, err := RequireProceeding(ctx, tx, id, authz.AccessDiary)
		if err != nil {
			return err
		}
		access, masked, err := authz.MatterAccess(ctx, tx, matterID)
		if err != nil {
			return err
		}
		out.Access = string(access)
		if err := tx.QueryRow(ctx, `select pr.id, pr.matter_id, pr.proceeding_type_id, t.code, pr.current_court_id,
			pr.title_en, pr.title_si, pr.our_side, pr.our_role, to_char(pr.instituted_on, 'YYYY-MM-DD'), pr.status,
			pr.row_version
			from proceeding pr join reference_item t on t.id = pr.proceeding_type_id where pr.id = $1`, id).
			Scan(&out.ID, &out.MatterID, &out.TypeID, &out.TypeCode, &out.CourtID, &out.Title.EN, &out.Title.SI,
				&out.OurSide, &out.OurRole, &out.InstitutedOn, &out.Status, &out.Version); err != nil {
			return fmt.Errorf("load proceeding: %w", err)
		}
		if masked {
			out.Title = registry.Name{}
		}
		if out.CaseNumbers, err = loadCaseNumbers(ctx, tx, id); err != nil {
			return err
		}
		for _, c := range out.CaseNumbers {
			if c.Current {
				raw := c.Raw
				out.CurrentCaseNo = &raw
			}
		}
		if out.Parties, err = loadParties(ctx, tx, id, masked); err != nil {
			return err
		}
		out.Relationships, err = loadRelationships(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func loadCaseNumbers(ctx context.Context, tx pgx.Tx, id uuid.UUID) ([]CaseNumber, error) {
	rows, err := tx.Query(ctx, `select id, court_id, raw, normalised, to_char(valid_from, 'YYYY-MM-DD'),
		to_char(valid_to, 'YYYY-MM-DD'), reason from case_number_record where proceeding_id = $1
		order by valid_to is null desc, valid_from desc, created_at desc`, id)
	if err != nil {
		return nil, fmt.Errorf("load case numbers: %w", err)
	}
	defer rows.Close()
	out := []CaseNumber{}
	for rows.Next() {
		var c CaseNumber
		if err := rows.Scan(&c.ID, &c.CourtID, &c.Raw, &c.Normalised, &c.ValidFrom, &c.ValidTo, &c.Reason); err != nil {
			return nil, fmt.Errorf("load case numbers: %w", err)
		}
		c.Current = c.ValidTo == nil
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadParties(ctx context.Context, tx pgx.Tx, id uuid.UUID, masked bool) ([]Party, error) {
	rows, err := tx.Query(ctx, `select pr.id, pr.entity_id, e.name_en, e.name_si, e.name_ta, pr.party_role_type_id, t.code,
		pr.ordinal, pr.side, pr.status
		from party_role pr join entity e on e.id = pr.entity_id join reference_item t on t.id = pr.party_role_type_id
		where pr.proceeding_id = $1 order by t.sort_order, coalesce(pr.ordinal, 0), pr.created_at`, id)
	if err != nil {
		return nil, fmt.Errorf("load parties: %w", err)
	}
	defer rows.Close()
	out := []Party{}
	for rows.Next() {
		var pt Party
		if err := rows.Scan(&pt.ID, &pt.EntityID, &pt.Name.EN, &pt.Name.SI, &pt.Name.TA, &pt.RoleTypeID, &pt.RoleCode,
			&pt.Ordinal, &pt.Side, &pt.Status); err != nil {
			return nil, fmt.Errorf("load parties: %w", err)
		}
		// Diary scope on a non-standard matter shows the case, not who is in it.
		if masked {
			pt.Name = registry.Name{}
		}
		out = append(out, pt)
	}
	return out, rows.Err()
}

func loadRelationships(ctx context.Context, tx pgx.Tx, id uuid.UUID) ([]Relationship, error) {
	rows, err := tx.Query(ctx, `
		select r.id, case when r.from_proceeding_id = $1 then 'outgoing' else 'incoming' end,
		       case when r.from_proceeding_id = $1 then r.to_proceeding_id else r.from_proceeding_id end, t.code,
		       (select c.raw from case_number_record c where c.valid_to is null and c.proceeding_id =
		            case when r.from_proceeding_id = $1 then r.to_proceeding_id else r.from_proceeding_id end),
		       r.note
		from proceeding_relationship r join reference_item t on t.id = r.relationship_type_id
		where r.from_proceeding_id = $1 or r.to_proceeding_id = $1 order by r.created_at`, id)
	if err != nil {
		return nil, fmt.Errorf("load relationships: %w", err)
	}
	defer rows.Close()
	out := []Relationship{}
	for rows.Next() {
		var r Relationship
		if err := rows.Scan(&r.ID, &r.Direction, &r.OtherID, &r.TypeCode, &r.OtherCaseNumber, &r.Note); err != nil {
			return nil, fmt.Errorf("load relationships: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListForMatter returns a matter's proceedings.
func (s *Service) ListForMatter(ctx context.Context, p *authz.Principal, matterID uuid.UUID) ([]Proceeding, error) {
	var ids []uuid.UUID
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		if _, _, err := authz.RequireMatter(ctx, tx, matterID, authz.AccessDiary); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `select id from proceeding where matter_id = $1 and deleted_at is null order by created_at`, matterID)
		if err != nil {
			return fmt.Errorf("list proceedings: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	out := make([]Proceeding, 0, len(ids))
	for _, id := range ids {
		pr, err := s.Get(ctx, p, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *pr)
	}
	return out, nil
}

// SoftDelete removes a proceeding created in error (proceeding.delete).
func (s *Service) SoftDelete(ctx context.Context, p *authz.Principal, id uuid.UUID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return apperr.Invalid(apperr.Field("reason", "required"))
	}
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		matterID, err := RequireProceeding(ctx, tx, id, authz.AccessFull)
		if err != nil {
			return err
		}
		if err := p.Require(authz.ProceedingDelete); err != nil {
			return err
		}
		var held bool
		if err := tx.QueryRow(ctx, `select exists (select 1 from hearing where proceeding_id = $1
			and listing_status = 'outcome_recorded')`, id).Scan(&held); err != nil {
			return fmt.Errorf("check hearings: %w", err)
		}
		if held {
			return apperr.New(apperr.KindConflict, "proceeding.has_history")
		}
		if _, err := tx.Exec(ctx, `update proceeding set deleted_at = now(), deleted_by = $2, delete_reason = $3 where id = $1`,
			id, p.UserID, reason); err != nil {
			return fmt.Errorf("delete proceeding: %w", err)
		}
		if _, err := tx.Exec(ctx, `update hearing set listing_status = 'cancelled', status_reason = 'proceeding_deleted'
			where proceeding_id = $1 and listing_status = 'scheduled'`, id); err != nil {
			return fmt.Errorf("cancel hearings: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "proceeding.deleted",
			TargetKind: "proceeding", TargetID: id, MatterID: matterID, Reason: reason})
	})
}

// Lookup finds visible proceedings by any current or past case number.
func (s *Service) Lookup(ctx context.Context, p *authz.Principal, courtID *uuid.UUID, number string) ([]Proceeding, error) {
	norm := courts.NormaliseCaseNumber(number)
	if norm == "" {
		return []Proceeding{}, nil
	}
	var ids []uuid.UUID
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+`
			select distinct pr.id from case_number_record c
			join proceeding pr on pr.id = c.proceeding_id and pr.deleted_at is null
			join vm on vm.matter_id = pr.matter_id
			where c.normalised = $1 and ($2::uuid is null or c.court_id = $2) limit 20`, norm, courtID)
		if err != nil {
			return fmt.Errorf("lookup: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	out := []Proceeding{}
	for _, id := range ids {
		pr, err := s.Get(ctx, p, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *pr)
	}
	return out, nil
}

func latin(en, si string) string {
	parts := []string{}
	for _, t := range []string{en, si} {
		if l := textnorm.LatinText(t); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, " / ")
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
