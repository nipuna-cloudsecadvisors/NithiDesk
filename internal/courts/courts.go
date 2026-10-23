// Package courts owns court master data and the configurable reference
// catalogues (doc 05, doc 17 §7): court levels, courts, courtrooms, the court
// calendar, proceeding types, party roles, hearing purposes, outcome types,
// proceeding relationship types and date source types.
package courts

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
)

// Reference kinds.
const (
	KindCourtLevel       = "court_level"
	KindPracticeArea     = "practice_area"
	KindProceedingType   = "proceeding_type"
	KindPartyRoleType    = "party_role_type"
	KindHearingPurpose   = "hearing_purpose"
	KindOutcomeType      = "outcome_type"
	KindRelationshipType = "relationship_type"
	KindDateSourceType   = "date_source_type"
)

// Kinds lists every reference kind.
var Kinds = []string{KindCourtLevel, KindPracticeArea, KindProceedingType, KindPartyRoleType, KindHearingPurpose,
	KindOutcomeType, KindRelationshipType, KindDateSourceType}

// Date authorities, strongest first (doc 06 §3).
const (
	AuthorityAuthoritative = "authoritative"
	AuthorityPrimary       = "primary"
	AuthoritySecondary     = "secondary"
	AuthorityHearsay       = "hearsay"
)

// CaseNumberPattern is one accepted shape of a court's case numbers.
type CaseNumberPattern struct {
	Pattern string `json:"pattern"`
	Example string `json:"example"`
}

// Name is a bilingual (later trilingual) label.
type Name struct {
	EN string  `json:"en"`
	SI *string `json:"si,omitempty"`
	TA *string `json:"ta,omitempty"`
}

// Item is one reference catalogue entry.
type Item struct {
	ID         uuid.UUID      `json:"id"`
	Kind       string         `json:"kind"`
	Code       string         `json:"code"`
	Name       Name           `json:"name"`
	Sort       int            `json:"sort_order"`
	Attributes map[string]any `json:"attributes"`
	ValidTo    *time.Time     `json:"valid_to,omitempty"`
	Version    int            `json:"row_version"`
}

// Court is a court with its level.
type Court struct {
	ID         uuid.UUID           `json:"id"`
	Code       string              `json:"code"`
	LevelID    uuid.UUID           `json:"court_level_id"`
	LevelCode  string              `json:"court_level_code"`
	Name       Name                `json:"name"`
	LocationEN *string             `json:"location_en,omitempty"`
	LocationSI *string             `json:"location_si,omitempty"`
	Patterns   []CaseNumberPattern `json:"case_number_patterns"`
	Sort       int                 `json:"sort_order"`
	ValidTo    *time.Time          `json:"valid_to,omitempty"`
	Courtrooms []Courtroom         `json:"courtrooms"`
}

// Courtroom is a room or bench within a court.
type Courtroom struct {
	ID    uuid.UUID `json:"id"`
	Label string    `json:"label"`
}

// CalendarDay is a holiday, Poya day, vacation or non-sitting day.
type CalendarDay struct {
	ID      uuid.UUID  `json:"id"`
	CourtID *uuid.UUID `json:"court_id"`
	Day     string     `json:"day"`
	Kind    string     `json:"kind"`
	LabelEN *string    `json:"label_en,omitempty"`
	LabelSI *string    `json:"label_si,omitempty"`
}

// Service reads and maintains reference data.
type Service struct {
	pool *pgxpool.Pool
}

// New builds the service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// SeedFirm inserts the default catalogue and courts for a new firm.
func SeedFirm(ctx context.Context, tx pgx.Tx, firmID uuid.UUID) error {
	levelIDs := map[string]uuid.UUID{}
	for _, kind := range Kinds {
		for _, it := range Seed[kind] {
			attrs, err := json.Marshal(nonNil(it.Attributes))
			if err != nil {
				return err
			}
			var id uuid.UUID
			if err := tx.QueryRow(ctx, `
				insert into reference_item (firm_id, kind, code, name_en, name_si, sort_order, attributes)
				values ($1, $2, $3, $4, $5, $6, $7)
				on conflict (firm_id, kind, code) do update set name_en = excluded.name_en
				returning id`,
				firmID, kind, it.Code, it.NameEN, it.NameSI, it.Sort, attrs).Scan(&id); err != nil {
				return fmt.Errorf("seed %s %s: %w", kind, it.Code, err)
			}
			if kind == KindCourtLevel {
				levelIDs[it.Code] = id
			}
		}
	}
	for i, c := range SeedCourts {
		patterns, err := json.Marshal(nonNilPatterns(c.Patterns))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			insert into court (firm_id, code, court_level_id, name_en, name_si, location_en, location_si,
			                   case_number_patterns, sort_order)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			on conflict (firm_id, code) do nothing`,
			firmID, c.Code, levelIDs[c.Level], c.NameEN, c.NameSI, c.LocationEN, c.LocationSI, patterns, (i+1)*10); err != nil {
			return fmt.Errorf("seed court %s: %w", c.Code, err)
		}
	}
	return nil
}

func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nonNilPatterns(p []CaseNumberPattern) []CaseNumberPattern {
	if p == nil {
		return []CaseNumberPattern{}
	}
	return p
}

// List returns the reference items of one kind (current ones unless all).
func (s *Service) List(ctx context.Context, p *authz.Principal, kind string, all bool) ([]Item, error) {
	if !validKind(kind) {
		return nil, apperr.NotFound
	}
	var out []Item
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			select id, kind, code, name_en, name_si, name_ta, sort_order, attributes, valid_to, row_version
			from reference_item
			where kind = $1 and ($2 or valid_to is null or valid_to >= current_date)
			order by sort_order, name_en`, kind, all)
		if err != nil {
			return fmt.Errorf("list reference: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var it Item
			var attrs []byte
			if err := rows.Scan(&it.ID, &it.Kind, &it.Code, &it.Name.EN, &it.Name.SI, &it.Name.TA, &it.Sort, &attrs,
				&it.ValidTo, &it.Version); err != nil {
				return fmt.Errorf("list reference: %w", err)
			}
			_ = json.Unmarshal(attrs, &it.Attributes)
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}

func validKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Courts returns every current court with its courtrooms.
func (s *Service) Courts(ctx context.Context, p *authz.Principal) ([]Court, error) {
	var out []Court
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			select c.id, c.code, c.court_level_id, l.code, c.name_en, c.name_si, c.name_ta, c.location_en, c.location_si,
			       c.case_number_patterns, c.sort_order, c.valid_to,
			       coalesce((select json_agg(json_build_object('id', r.id, 'label', r.label) order by r.label)
			                 from courtroom r where r.court_id = c.id and r.valid_to is null), '[]')
			from court c join reference_item l on l.id = c.court_level_id
			where c.valid_to is null or c.valid_to >= current_date
			order by l.sort_order, c.sort_order, c.name_en`)
		if err != nil {
			return fmt.Errorf("list courts: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var c Court
			var patterns, rooms []byte
			if err := rows.Scan(&c.ID, &c.Code, &c.LevelID, &c.LevelCode, &c.Name.EN, &c.Name.SI, &c.Name.TA,
				&c.LocationEN, &c.LocationSI, &patterns, &c.Sort, &c.ValidTo, &rooms); err != nil {
				return fmt.Errorf("list courts: %w", err)
			}
			_ = json.Unmarshal(patterns, &c.Patterns)
			_ = json.Unmarshal(rooms, &c.Courtrooms)
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// Calendar returns the non-sitting days between from and to (inclusive).
func (s *Service) Calendar(ctx context.Context, p *authz.Principal, from, to time.Time) ([]CalendarDay, error) {
	var out []CalendarDay
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		var err error
		out, err = CalendarDays(ctx, tx, from, to)
		return err
	})
	return out, err
}

// CalendarDays reads calendar days inside an existing transaction.
func CalendarDays(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]CalendarDay, error) {
	rows, err := tx.Query(ctx, `select id, court_id, to_char(day, 'YYYY-MM-DD'), kind, label_en, label_si
		from court_calendar_day where day between $1 and $2 order by day`, from, to)
	if err != nil {
		return nil, fmt.Errorf("court calendar: %w", err)
	}
	defer rows.Close()
	var out []CalendarDay
	for rows.Next() {
		var d CalendarDay
		if err := rows.Scan(&d.ID, &d.CourtID, &d.Day, &d.Kind, &d.LabelEN, &d.LabelSI); err != nil {
			return nil, fmt.Errorf("court calendar: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ItemInput creates or updates a reference item.
type ItemInput struct {
	Code       string         `json:"code"`
	NameEN     string         `json:"name_en"`
	NameSI     *string        `json:"name_si"`
	Sort       *int           `json:"sort_order"`
	Attributes map[string]any `json:"attributes"`
	ValidTo    *string        `json:"valid_to"`
}

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,60}$`)

// CreateItem adds a reference item (config.manage).
func (s *Service) CreateItem(ctx context.Context, p *authz.Principal, kind string, in ItemInput) (*Item, error) {
	if err := p.Require(authz.ConfigManage); err != nil {
		return nil, err
	}
	if !validKind(kind) {
		return nil, apperr.NotFound
	}
	var fields []apperr.FieldError
	if !codePattern.MatchString(in.Code) {
		fields = append(fields, apperr.Field("code", "code.invalid"))
	}
	if strings.TrimSpace(in.NameEN) == "" {
		fields = append(fields, apperr.Field("name_en", "required"))
	}
	if err := validateAttributes(kind, in.Attributes); err != "" {
		fields = append(fields, apperr.Field("attributes", err))
	}
	if len(fields) > 0 {
		return nil, apperr.Invalid(fields...)
	}
	sort := 100
	if in.Sort != nil {
		sort = *in.Sort
	}
	attrs, _ := json.Marshal(nonNil(in.Attributes))
	var id uuid.UUID
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `insert into reference_item (firm_id, kind, code, name_en, name_si, sort_order, attributes)
			values ($1, $2, $3, $4, $5, $6, $7) returning id`,
			p.FirmID, kind, in.Code, in.NameEN, in.NameSI, sort, attrs).Scan(&id)
		if db.IsUniqueViolation(err, "") {
			return apperr.New(apperr.KindConflict, "reference.code_taken")
		}
		if err != nil {
			return fmt.Errorf("create reference item: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "config.reference_created",
			TargetKind: kind, TargetID: id, Changes: map[string]any{"code": in.Code}})
	})
	if err != nil {
		return nil, err
	}
	return s.item(ctx, p, id)
}

// UpdateItem renames, re-orders or end-dates an item (config.manage, If-Match).
// Items are never deleted, so history keeps resolving.
func (s *Service) UpdateItem(ctx context.Context, p *authz.Principal, id uuid.UUID, version int, in ItemInput) (*Item, error) {
	if err := p.Require(authz.ConfigManage); err != nil {
		return nil, err
	}
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		var kind string
		var current int
		if err := tx.QueryRow(ctx, `select kind, row_version from reference_item where id = $1 for update`, id).
			Scan(&kind, &current); err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound
			}
			return fmt.Errorf("load reference item: %w", err)
		}
		if current != version {
			return apperr.VersionMismatch
		}
		if in.Attributes != nil {
			if e := validateAttributes(kind, in.Attributes); e != "" {
				return apperr.Invalid(apperr.Field("attributes", e))
			}
		}
		var attrs []byte
		if in.Attributes != nil {
			attrs, _ = json.Marshal(in.Attributes)
		}
		var name *string
		if strings.TrimSpace(in.NameEN) != "" {
			name = &in.NameEN
		}
		if _, err := tx.Exec(ctx, `update reference_item set name_en = coalesce($2, name_en), name_si = coalesce($3, name_si),
			sort_order = coalesce($4, sort_order), attributes = coalesce($5, attributes), valid_to = coalesce($6::date, valid_to)
			where id = $1`, id, name, in.NameSI, in.Sort, attrs, in.ValidTo); err != nil {
			return fmt.Errorf("update reference item: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "config.reference_updated",
			TargetKind: kind, TargetID: id})
	})
	if err != nil {
		return nil, err
	}
	return s.item(ctx, p, id)
}

func (s *Service) item(ctx context.Context, p *authz.Principal, id uuid.UUID) (*Item, error) {
	var it Item
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		var attrs []byte
		err := tx.QueryRow(ctx, `select id, kind, code, name_en, name_si, name_ta, sort_order, attributes, valid_to, row_version
			from reference_item where id = $1`, id).Scan(&it.ID, &it.Kind, &it.Code, &it.Name.EN, &it.Name.SI, &it.Name.TA,
			&it.Sort, &attrs, &it.ValidTo, &it.Version)
		if err != nil {
			return err
		}
		_ = json.Unmarshal(attrs, &it.Attributes)
		return nil
	})
	if err == pgx.ErrNoRows {
		return nil, apperr.NotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// validateAttributes checks the typed attributes business code relies on.
func validateAttributes(kind string, a map[string]any) string {
	switch kind {
	case KindDateSourceType:
		v, _ := a["default_authority"].(string)
		switch v {
		case AuthorityAuthoritative, AuthorityPrimary, AuthoritySecondary, AuthorityHearsay:
			return ""
		}
		return "attributes.default_authority_required"
	case KindOutcomeType:
		if _, ok := a["requires_next"].(bool); !ok {
			return "attributes.requires_next_required"
		}
	case KindRelationshipType:
		if _, ok := a["derivation"].(bool); !ok {
			return "attributes.derivation_required"
		}
	}
	return ""
}

// CourtInput creates a court.
type CourtInput struct {
	Code       string              `json:"code"`
	LevelID    uuid.UUID           `json:"court_level_id"`
	NameEN     string              `json:"name_en"`
	NameSI     *string             `json:"name_si"`
	LocationEN *string             `json:"location_en"`
	LocationSI *string             `json:"location_si"`
	Patterns   []CaseNumberPattern `json:"case_number_patterns"`
	Courtrooms []string            `json:"courtrooms"`
}

var courtCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{1,40}$`)

// CreateCourt adds a court (config.manage).
func (s *Service) CreateCourt(ctx context.Context, p *authz.Principal, in CourtInput) (uuid.UUID, error) {
	if err := p.Require(authz.ConfigManage); err != nil {
		return uuid.Nil, err
	}
	var fields []apperr.FieldError
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if !courtCodePattern.MatchString(in.Code) {
		fields = append(fields, apperr.Field("code", "code.invalid"))
	}
	if strings.TrimSpace(in.NameEN) == "" {
		fields = append(fields, apperr.Field("name_en", "required"))
	}
	for _, pat := range in.Patterns {
		if _, err := regexp.Compile(pat.Pattern); err != nil {
			fields = append(fields, apperr.Field("case_number_patterns", "pattern.invalid"))
		}
	}
	if len(fields) > 0 {
		return uuid.Nil, apperr.Invalid(fields...)
	}
	patterns, _ := json.Marshal(nonNilPatterns(in.Patterns))
	var id uuid.UUID
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `insert into court (firm_id, code, court_level_id, name_en, name_si, location_en,
			location_si, case_number_patterns) values ($1, $2, $3, $4, $5, $6, $7, $8) returning id`,
			p.FirmID, in.Code, in.LevelID, in.NameEN, in.NameSI, in.LocationEN, in.LocationSI, patterns).Scan(&id)
		if db.IsUniqueViolation(err, "") {
			return apperr.New(apperr.KindConflict, "court.code_taken")
		}
		if code, _, ok := db.Constraint(err); ok && code == "23503" {
			return apperr.Invalid(apperr.Field("court_level_id", "reference.unknown"))
		}
		if err != nil {
			return fmt.Errorf("create court: %w", err)
		}
		for _, label := range in.Courtrooms {
			if strings.TrimSpace(label) == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `insert into courtroom (firm_id, court_id, label) values ($1, $2, $3)
				on conflict do nothing`, p.FirmID, id, strings.TrimSpace(label)); err != nil {
				return fmt.Errorf("create courtroom: %w", err)
			}
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "config.court_created",
			TargetKind: "court", TargetID: id, Changes: map[string]any{"code": in.Code}})
	})
	return id, err
}

// CalendarInput adds a non-sitting day.
type CalendarInput struct {
	CourtID *uuid.UUID `json:"court_id"`
	Day     string     `json:"day"`
	Kind    string     `json:"kind"`
	LabelEN *string    `json:"label_en"`
	LabelSI *string    `json:"label_si"`
}

// AddCalendarDay records a holiday, Poya day, vacation or non-sitting day.
func (s *Service) AddCalendarDay(ctx context.Context, p *authz.Principal, in CalendarInput) error {
	if err := p.Require(authz.ConfigManage); err != nil {
		return err
	}
	day, err := time.Parse(time.DateOnly, in.Day)
	if err != nil {
		return apperr.Invalid(apperr.Field("day", "date.invalid"))
	}
	switch in.Kind {
	case "public_holiday", "poya", "vacation", "not_sitting":
	default:
		return apperr.Invalid(apperr.Field("kind", "invalid"))
	}
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `insert into court_calendar_day (firm_id, court_id, day, kind, label_en, label_si)
			values ($1, $2, $3, $4, $5, $6)`, p.FirmID, in.CourtID, day, in.Kind, in.LabelEN, in.LabelSI)
		if db.IsUniqueViolation(err, "") {
			return apperr.New(apperr.KindConflict, "calendar.day_exists")
		}
		if err != nil {
			return fmt.Errorf("add calendar day: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "config.calendar_day_added",
			TargetKind: "court_calendar_day", Changes: map[string]any{"day": in.Day, "kind": in.Kind}})
	})
}

func actorOf(p *authz.Principal) audit.Actor {
	return audit.Actor{Kind: audit.ActorUser, UserID: p.UserID, SessionID: p.SessionID, RequestID: p.RequestID, IP: p.IP}
}

// NormaliseCaseNumber returns the comparison form of a case number: NFKC,
// upper case, no spaces, "/" as the only separator, no leading zeros in
// numeric segments. "b 01234 / 26" and "B/1234/26" normalise identically.
func NormaliseCaseNumber(raw string) string {
	s := strings.ToUpper(norm.NFKC.String(strings.TrimSpace(raw)))
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return -1
		case r == '\\' || r == '|':
			return '/'
		}
		return r
	}, s)
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	s = strings.Trim(s, "/")
	parts := strings.Split(s, "/")
	for i, part := range parts {
		if isDigits(part) {
			trimmed := strings.TrimLeft(part, "0")
			if trimmed == "" {
				trimmed = "0"
			}
			parts[i] = trimmed
		}
	}
	return strings.Join(parts, "/")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// CaseNumberCheck is the result of checking a case number against a court.
type CaseNumberCheck struct {
	Normalised string `json:"normalised"`
	// MatchesPattern is false when the court defines patterns and none
	// matches. It is a warning: real numbering varies.
	MatchesPattern bool   `json:"matches_pattern"`
	Example        string `json:"example,omitempty"`
}

// CheckCaseNumber normalises raw and tests it against the court's patterns.
func CheckCaseNumber(ctx context.Context, tx pgx.Tx, courtID uuid.UUID, raw string) (CaseNumberCheck, error) {
	out := CaseNumberCheck{Normalised: NormaliseCaseNumber(raw), MatchesPattern: true}
	var patternsJSON []byte
	if err := tx.QueryRow(ctx, `select case_number_patterns from court where id = $1`, courtID).Scan(&patternsJSON); err != nil {
		if err == pgx.ErrNoRows {
			return out, apperr.Invalid(apperr.Field("court_id", "reference.unknown"))
		}
		return out, fmt.Errorf("load court: %w", err)
	}
	var patterns []CaseNumberPattern
	_ = json.Unmarshal(patternsJSON, &patterns)
	if len(patterns) == 0 {
		return out, nil
	}
	out.MatchesPattern = false
	out.Example = patterns[0].Example
	for _, pat := range patterns {
		re, err := regexp.Compile(pat.Pattern)
		if err == nil && re.MatchString(out.Normalised) {
			out.MatchesPattern = true
			break
		}
	}
	return out, nil
}

// CheckCaseNumberForCourt runs CheckCaseNumber in its own transaction.
func (s *Service) CheckCaseNumberForCourt(ctx context.Context, p *authz.Principal, courtID uuid.UUID, raw string) (CaseNumberCheck, error) {
	var out CaseNumberCheck
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		var err error
		out, err = CheckCaseNumber(ctx, tx, courtID, raw)
		return err
	})
	return out, err
}

// ItemByID loads one reference item of a kind inside tx, for validation.
func ItemByID(ctx context.Context, tx pgx.Tx, kind string, id uuid.UUID) (code string, attrs map[string]any, err error) {
	var raw []byte
	err = tx.QueryRow(ctx, `select code, attributes from reference_item where id = $1 and kind = $2`, id, kind).Scan(&code, &raw)
	if err == pgx.ErrNoRows {
		return "", nil, apperr.Invalid(apperr.Field(kind+"_id", "reference.unknown"))
	}
	if err != nil {
		return "", nil, fmt.Errorf("load %s: %w", kind, err)
	}
	_ = json.Unmarshal(raw, &attrs)
	return code, attrs, nil
}

// ItemByCode resolves a reference item by its stable code inside tx.
func ItemByCode(ctx context.Context, tx pgx.Tx, kind, code string) (uuid.UUID, map[string]any, error) {
	var id uuid.UUID
	var raw []byte
	err := tx.QueryRow(ctx, `select id, attributes from reference_item where kind = $1 and code = $2`, kind, code).Scan(&id, &raw)
	if err == pgx.ErrNoRows {
		return uuid.Nil, nil, apperr.Invalid(apperr.Field(kind, "reference.unknown")).WithParams(map[string]any{"code": code})
	}
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("load %s: %w", kind, err)
	}
	var attrs map[string]any
	_ = json.Unmarshal(raw, &attrs)
	return id, attrs, nil
}
