// Package imports loads the firm's current diary from CSV (doc 16, logical
// model §11): upload → dry-run validation with a per-row report → commit in
// one transaction → rollback while nothing has been touched.
//
// Rules from the migration plan:
//   - Nothing is written to business tables until commit.
//   - Every imported date is a "migrated" assertion with hearsay authority,
//     so it is unverified and lands in the clerk's verification queue
//     (doc 16 §3: every future hearing is verified by hand).
//   - Exact duplicates are reused, never merged by guesswork.
package imports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/courts"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/hearings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/matters"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/clock"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/proceedings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/registry"
)

// MaxFileBytes bounds an upload (also a database check).
const MaxFileBytes = 5 << 20

// MaxRows bounds one batch.
const MaxRows = 5000

// Columns is the diary CSV template, in order. Headers are matched by name,
// case-insensitively, so a spreadsheet can reorder them.
var Columns = []string{
	"matter_title_en", "matter_title_si", "client_name_en", "client_name_si", "client_kind", "practice_area",
	"confidentiality", "responsible_partner_email", "lead_email", "court_code", "proceeding_type", "case_number",
	"our_side", "our_party_role", "opposing_party_en", "opposing_party_si", "opposing_party_role", "hearing_date",
	"hearing_session", "hearing_purpose", "appearing_email", "hearing_source_note",
}

var required = []string{"court_code", "proceeding_type", "hearing_date", "responsible_partner_email"}

// Template returns the CSV template with a BOM so Excel opens it as UTF-8,
// and one synthetic example row.
func Template() []byte {
	var b bytes.Buffer
	b.WriteString("\uFEFF")
	w := csv.NewWriter(&b)
	_ = w.Write(Columns)
	_ = w.Write([]string{"Perera v. Silva — land at Kotte", "පෙරේරා එදිරිව සිල්වා", "Nimal Perera", "නිමල් පෙරේරා",
		"person", "land", "standard", "partner@example.lk", "lawyer@example.lk", "DC-COLOMBO", "dc_land", "L/1234/24",
		"plaintiff_side", "plaintiff", "Kamal Silva", "කමල් සිල්වා", "defendant", "2026-11-24", "morning", "trial",
		"lawyer@example.lk", "Paper diary page 112"})
	w.Flush()
	return b.Bytes()
}

// Batch is an import batch.
type Batch struct {
	ID           uuid.UUID  `json:"id"`
	Kind         string     `json:"kind"`
	FileName     string     `json:"file_name"`
	Status       string     `json:"status"`
	RowCount     int        `json:"row_count"`
	OKCount      int        `json:"ok_count"`
	WarningCount int        `json:"warning_count"`
	ErrorCount   int        `json:"error_count"`
	CreatedAt    time.Time  `json:"created_at"`
	ValidatedAt  *time.Time `json:"validated_at,omitempty"`
	CommittedAt  *time.Time `json:"committed_at,omitempty"`
	RolledBackAt *time.Time `json:"rolled_back_at,omitempty"`
	Version      int        `json:"row_version"`
}

// Message is one validation finding for a row.
type Message struct {
	Code   string         `json:"code"`
	Field  string         `json:"field,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

// Row is one validated CSV row.
type Row struct {
	Number   int               `json:"row_number"`
	Status   string            `json:"status"`
	Messages []Message         `json:"messages"`
	Raw      map[string]string `json:"raw"`
}

// Service implements imports.
type Service struct {
	pool *pgxpool.Pool
}

// New builds the service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func actorOf(p *authz.Principal) audit.Actor {
	return audit.Actor{Kind: audit.ActorUser, UserID: p.UserID, SessionID: p.SessionID, RequestID: p.RequestID, IP: p.IP}
}

// Upload stores a CSV file as a new batch (import.run). The file is parsed
// as data only and never served back.
func (s *Service) Upload(ctx context.Context, p *authz.Principal, kind, name string, content []byte) (*Batch, error) {
	if err := p.Require(authz.ImportRun); err != nil {
		return nil, err
	}
	if kind != "diary_csv" {
		return nil, apperr.Invalid(apperr.Field("kind", "invalid"))
	}
	if len(content) == 0 || len(content) > MaxFileBytes {
		return nil, apperr.Invalid(apperr.Field("file", "import.file_size"))
	}
	content = bytes.TrimPrefix(content, []byte("\xEF\xBB\xBF"))
	if !utf8.Valid(content) {
		return nil, apperr.Invalid(apperr.Field("file", "import.not_utf8"))
	}
	name = safeName(name)
	sum := sha256.Sum256(content)
	var id uuid.UUID
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `insert into import_batch (firm_id, kind, file_name, file_sha256, file_content, created_by)
			values ($1, $2, $3, $4, $5, $6) returning id`, p.FirmID, kind, name, sum[:], content, p.UserID).Scan(&id); err != nil {
			return fmt.Errorf("store import: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "import.uploaded",
			TargetKind: "import_batch", TargetID: id, Changes: map[string]any{"bytes": len(content)}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func safeName(n string) string {
	n = strings.TrimSpace(n)
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	if n == "" {
		n = "import.csv"
	}
	if len(n) > 120 {
		n = n[:120]
	}
	return n
}

// Get returns a batch (import.run; only the firm's own, under RLS).
func (s *Service) Get(ctx context.Context, p *authz.Principal, id uuid.UUID) (*Batch, error) {
	if err := p.Require(authz.ImportRun); err != nil {
		return nil, err
	}
	var b Batch
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select id, kind, file_name, status, row_count, ok_count, warning_count, error_count,
			created_at, validated_at, committed_at, rolled_back_at, row_version from import_batch where id = $1`, id).
			Scan(&b.ID, &b.Kind, &b.FileName, &b.Status, &b.RowCount, &b.OKCount, &b.WarningCount, &b.ErrorCount,
				&b.CreatedAt, &b.ValidatedAt, &b.CommittedAt, &b.RolledBackAt, &b.Version)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// Rows returns validated rows, optionally by status.
func (s *Service) Rows(ctx context.Context, p *authz.Principal, id uuid.UUID, status string) ([]Row, error) {
	if _, err := s.Get(ctx, p, id); err != nil {
		return nil, err
	}
	out := []Row{}
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select row_number, status, messages, raw from import_row
			where batch_id = $1 and ($2 = '' or status = $2) order by row_number limit 5000`, id, status)
		if err != nil {
			return fmt.Errorf("import rows: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var r Row
			var msgs, raw []byte
			if err := rows.Scan(&r.Number, &r.Status, &msgs, &raw); err != nil {
				return fmt.Errorf("import rows: %w", err)
			}
			_ = json.Unmarshal(msgs, &r.Messages)
			_ = json.Unmarshal(raw, &r.Raw)
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// parsed is a validated row ready to commit.
type parsed struct {
	row          Row
	matterKey    string
	procKey      string
	matter       matters.CreateInput
	proceeding   proceedings.CreateInput
	hearing      hearings.CreateInput
	existingProc uuid.UUID // proceeding already in NithiDesk with this case number
	existingMat  uuid.UUID
	skip         bool
}

// Validate is the dry run: it parses and checks every row and stores the
// report. Nothing is written to business tables.
func (s *Service) Validate(ctx context.Context, p *authz.Principal, id uuid.UUID) (*Batch, error) {
	if err := p.Require(authz.ImportRun); err != nil {
		return nil, err
	}
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		status, content, err := lockBatch(ctx, tx, id)
		if err != nil {
			return err
		}
		if status != "uploaded" && status != "validated" {
			return apperr.New(apperr.KindConflict, "import.not_validatable")
		}
		rows, err := s.parse(ctx, tx, p, content)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `delete from import_row where batch_id = $1`, id); err != nil {
			return fmt.Errorf("clear rows: %w", err)
		}
		counts := map[string]int{}
		for _, r := range rows {
			counts[r.row.Status]++
			msgs, _ := json.Marshal(r.row.Messages)
			raw, _ := json.Marshal(r.row.Raw)
			if _, err := tx.Exec(ctx, `insert into import_row (firm_id, batch_id, row_number, raw, status, messages)
				values ($1, $2, $3, $4, $5, $6)`, p.FirmID, id, r.row.Number, raw, r.row.Status, msgs); err != nil {
				return fmt.Errorf("store row: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `update import_batch set status = 'validated', validated_at = now(), row_count = $2,
			ok_count = $3, warning_count = $4, error_count = $5 where id = $1`,
			id, len(rows), counts["ok"], counts["warning"], counts["error"]); err != nil {
			return fmt.Errorf("update batch: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "import.validated",
			TargetKind: "import_batch", TargetID: id, Changes: map[string]any{"rows": len(rows), "errors": counts["error"],
				"warnings": counts["warning"]}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func lockBatch(ctx context.Context, tx pgx.Tx, id uuid.UUID) (string, []byte, error) {
	var status string
	var content []byte
	err := tx.QueryRow(ctx, `select status, file_content from import_batch where id = $1 for update`, id).Scan(&status, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, apperr.NotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("load batch: %w", err)
	}
	return status, content, nil
}

// lookups caches reference and user resolution for one parse.
type lookups struct {
	courts   map[string]uuid.UUID
	refs     map[string]map[string]uuid.UUID
	users    map[string]uuid.UUID
	eligible map[uuid.UUID]bool
}

func loadLookups(ctx context.Context, tx pgx.Tx) (*lookups, error) {
	l := &lookups{courts: map[string]uuid.UUID{}, refs: map[string]map[string]uuid.UUID{}, users: map[string]uuid.UUID{},
		eligible: map[uuid.UUID]bool{}}
	rows, err := tx.Query(ctx, `select code, id from court where valid_to is null or valid_to >= current_date`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var code string
		var id uuid.UUID
		if err := rows.Scan(&code, &id); err != nil {
			rows.Close()
			return nil, err
		}
		l.courts[strings.ToUpper(code)] = id
	}
	rows.Close()
	rows, err = tx.Query(ctx, `select kind, code, id from reference_item where valid_to is null or valid_to >= current_date`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind, code string
		var id uuid.UUID
		if err := rows.Scan(&kind, &code, &id); err != nil {
			rows.Close()
			return nil, err
		}
		if l.refs[kind] == nil {
			l.refs[kind] = map[string]uuid.UUID{}
		}
		l.refs[kind][code] = id
	}
	rows.Close()
	rows, err = tx.Query(ctx, `select u.email, u.id, exists (select 1 from user_role ur join role_template rt on rt.id = ur.role_template_id
		where ur.user_id = u.id and ur.valid_to is null and rt.matter_scope <> 'none')
		from app_user u where u.status <> 'disabled'`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var email string
		var id uuid.UUID
		var ok bool
		if err := rows.Scan(&email, &id, &ok); err != nil {
			rows.Close()
			return nil, err
		}
		l.users[email] = id
		l.eligible[id] = ok
	}
	rows.Close()
	return l, rows.Err()
}

func (s *Service) parse(ctx context.Context, tx pgx.Tx, p *authz.Principal, content []byte) ([]*parsed, error) {
	r := csv.NewReader(bytes.NewReader(content))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return nil, apperr.Invalid(apperr.Field("file", "import.no_header"))
	}
	index := map[string]int{}
	for i, h := range header {
		index[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF")))] = i
	}
	for _, c := range required {
		if _, ok := index[c]; !ok {
			return nil, apperr.Invalid(apperr.Field("file", "import.missing_column")).WithParams(map[string]any{"column": c})
		}
	}
	l, err := loadLookups(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("import lookups: %w", err)
	}
	var out []*parsed
	seenHearing := map[string]int{}
	for n := 2; ; n++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, apperr.Invalid(apperr.Field("file", "import.csv_malformed")).WithParams(map[string]any{"row": n})
		}
		if len(out) >= MaxRows {
			return nil, apperr.Invalid(apperr.Field("file", "import.too_many_rows")).WithParams(map[string]any{"max": MaxRows})
		}
		raw := map[string]string{}
		blank := true
		for _, c := range Columns {
			if i, ok := index[c]; ok && i < len(rec) {
				v := strings.TrimSpace(rec[i])
				raw[c] = v
				if v != "" {
					blank = false
				}
			}
		}
		if blank {
			continue
		}
		pr := s.validateRow(ctx, tx, p, l, n, raw)
		if pr.row.Status != "error" {
			k := pr.procKey + "|" + raw["hearing_date"]
			if first, dup := seenHearing[k]; dup {
				pr.row.Messages = append(pr.row.Messages, Message{Code: "import.duplicate_hearing_row",
					Params: map[string]any{"first_row": first}})
				pr.skip = true
			} else {
				seenHearing[k] = n
			}
		}
		pr.row.Status = statusOf(pr.row.Messages)
		out = append(out, pr)
	}
	return out, nil
}

func statusOf(msgs []Message) string {
	status := "ok"
	for _, m := range msgs {
		if strings.HasPrefix(m.Code, "import.warn.") || m.Code == "import.duplicate_hearing_row" ||
			m.Code == "import.existing_proceeding" || m.Code == "case_number.unusual_format" || m.Code == "hearing.date_in_past" {
			status = "warning"
			continue
		}
		return "error"
	}
	return status
}

func (s *Service) validateRow(ctx context.Context, tx pgx.Tx, p *authz.Principal, l *lookups, n int, raw map[string]string) *parsed {
	pr := &parsed{row: Row{Number: n, Raw: raw, Messages: []Message{}}}
	errf := func(field, code string) { pr.row.Messages = append(pr.row.Messages, Message{Code: code, Field: field}) }
	warnf := func(field, code string, params map[string]any) {
		pr.row.Messages = append(pr.row.Messages, Message{Code: code, Field: field, Params: params})
	}
	for _, c := range required {
		if raw[c] == "" {
			errf(c, "required")
		}
	}
	courtID, ok := l.courts[strings.ToUpper(raw["court_code"])]
	if raw["court_code"] != "" && !ok {
		errf("court_code", "reference.unknown")
	}
	typeID, ok := l.refs[courts.KindProceedingType][raw["proceeding_type"]]
	if raw["proceeding_type"] != "" && !ok {
		errf("proceeding_type", "reference.unknown")
	}
	date, err := clock.ParseDate(raw["hearing_date"])
	if raw["hearing_date"] != "" && err != nil {
		errf("hearing_date", "date.invalid")
	} else if err == nil && date.Before(clock.Today()) {
		warnf("hearing_date", "hearing.date_in_past", nil)
	}
	session := raw["hearing_session"]
	if session == "" {
		session = "unspecified"
	}
	if session != "morning" && session != "afternoon" && session != "unspecified" {
		errf("hearing_session", "invalid")
	}
	refOpt := func(field, kind string) *uuid.UUID {
		if raw[field] == "" {
			return nil
		}
		id, ok := l.refs[kind][raw[field]]
		if !ok {
			errf(field, "reference.unknown")
			return nil
		}
		return &id
	}
	practice := refOpt("practice_area", courts.KindPracticeArea)
	purpose := refOpt("hearing_purpose", courts.KindHearingPurpose)
	ourRole := refOpt("our_party_role", courts.KindPartyRoleType)
	oppRole := refOpt("opposing_party_role", courts.KindPartyRoleType)
	conf := raw["confidentiality"]
	if conf == "" {
		conf = "standard"
	}
	if conf != "standard" && conf != "restricted" && conf != "highly_restricted" {
		errf("confidentiality", "invalid")
	}
	kind := raw["client_kind"]
	if kind == "" {
		kind = registry.KindPerson
	}
	if kind != registry.KindPerson && kind != registry.KindOrganisation && kind != registry.KindGovernment {
		errf("client_kind", "invalid")
	}
	if raw["client_name_en"] == "" && raw["client_name_si"] == "" {
		errf("client_name_en", "required")
	}
	user := func(field string) uuid.UUID {
		email := strings.ToLower(raw[field])
		if email == "" {
			return uuid.Nil
		}
		id, ok := l.users[email]
		if !ok || !l.eligible[id] {
			errf(field, "member.not_eligible")
			return uuid.Nil
		}
		return id
	}
	partner := user("responsible_partner_email")
	lead := user("lead_email")
	appearing := user("appearing_email")

	title := raw["matter_title_en"]
	if title == "" && raw["matter_title_si"] == "" {
		client := firstNonEmpty(raw["client_name_en"], raw["client_name_si"])
		opp := firstNonEmpty(raw["opposing_party_en"], raw["opposing_party_si"])
		title = client
		if opp != "" {
			title = client + " v. " + opp
		}
	}
	srcID, ok := l.refs[courts.KindDateSourceType]["migrated"]
	if !ok {
		errf("hearing_source_note", "import.no_migrated_source")
	}

	caseNorm := courts.NormaliseCaseNumber(raw["case_number"])
	pr.procKey = strings.ToUpper(raw["court_code"]) + "|" + caseNorm
	if caseNorm == "" {
		// Without a number each row stands alone; the clerk adds it later.
		pr.procKey = fmt.Sprintf("row-%d", n)
		warnf("case_number", "import.warn.no_case_number", nil)
	}
	pr.matterKey = strings.ToLower(firstNonEmpty(title, raw["matter_title_si"]) + "|" +
		firstNonEmpty(raw["client_name_en"], raw["client_name_si"]))

	if courtID != uuid.Nil && caseNorm != "" {
		check, err := courts.CheckCaseNumber(ctx, tx, courtID, raw["case_number"])
		if err == nil && !check.MatchesPattern {
			warnf("case_number", "case_number.unusual_format", map[string]any{"example": check.Example})
		}
		var procID, matterID uuid.UUID
		var access *string
		err = tx.QueryRow(ctx, `select pr.id, pr.matter_id, (select access from nd_visible_matters() vm where vm.matter_id = pr.matter_id)
			from case_number_record c join proceeding pr on pr.id = c.proceeding_id and pr.deleted_at is null
			where c.court_id = $1 and c.normalised = $2 and c.valid_to is null limit 1`, courtID, caseNorm).
			Scan(&procID, &matterID, &access)
		switch {
		case err == nil && access != nil && *access == string(authz.AccessFull):
			pr.existingProc, pr.existingMat = procID, matterID
			warnf("case_number", "import.existing_proceeding", nil)
		case err == nil:
			// Exists in a matter the importer cannot see: never reveal it.
			errf("case_number", "case_number.exists_elsewhere")
		}
	}

	team := []matters.MemberInput{}
	if partner != uuid.Nil {
		team = append(team, matters.MemberInput{UserID: partner, TeamRole: matters.RoleResponsiblePartner})
	}
	if lead != uuid.Nil && lead != partner {
		team = append(team, matters.MemberInput{UserID: lead, TeamRole: matters.RoleLead})
	}
	if appearing != uuid.Nil && appearing != partner && appearing != lead {
		team = append(team, matters.MemberInput{UserID: appearing, TeamRole: matters.RoleSupporting})
	}
	pr.matter = matters.CreateInput{TitleEN: title, TitleSI: raw["matter_title_si"], PracticeAreaID: practice,
		Confidentiality: conf, CreatedVia: "import",
		Clients: []matters.ClientInput{{New: &registry.Input{Kind: kind, NameEN: raw["client_name_en"],
			NameSI: raw["client_name_si"]}, Role: "primary"}},
		Team: team}
	var parties []proceedings.PartyInput
	if ourRole != nil {
		parties = append(parties, proceedings.PartyInput{New: &registry.Input{Kind: kind, NameEN: raw["client_name_en"],
			NameSI: raw["client_name_si"]}, RoleTypeID: *ourRole, Side: "our_client"})
	}
	if oppRole != nil && (raw["opposing_party_en"] != "" || raw["opposing_party_si"] != "") {
		parties = append(parties, proceedings.PartyInput{New: &registry.Input{Kind: registry.KindPerson,
			NameEN: raw["opposing_party_en"], NameSI: raw["opposing_party_si"]}, RoleTypeID: *oppRole, Side: "opposing"})
	}
	ourSide := raw["our_side"]
	pr.proceeding = proceedings.CreateInput{TypeID: typeID, CourtID: courtID, CaseNumber: raw["case_number"],
		OurSide: ourSide, Parties: parties, CreatedVia: "import", CaseReason: "migrated"}
	var appearingList []uuid.UUID
	if appearing != uuid.Nil {
		appearingList = []uuid.UUID{appearing}
	}
	pr.hearing = hearings.CreateInput{DateClaim: hearings.DateClaim{Date: raw["hearing_date"], Session: session,
		SourceTypeID: srcID, SourceDetail: firstNonEmpty(raw["hearing_source_note"], "Imported from diary CSV")},
		PurposeID: purpose, Appearing: appearingList, CreatedVia: "import", AssertionKind: hearings.KindFromImport}
	if ourSide != "" {
		switch ourSide {
		case "plaintiff_side", "defendant_side", "petitioner", "respondent", "appellant", "accused", "complainant",
			"intervenient", "other":
		default:
			errf("our_side", "invalid")
		}
	}
	return pr
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return strings.TrimSpace(x)
		}
	}
	return ""
}

// Commit creates the batch's records in one transaction (import.run +
// step-up, If-Match). Rows sharing a court and case number become one
// proceeding; rows sharing a matter title and client become one matter.
func (s *Service) Commit(ctx context.Context, p *authz.Principal, id uuid.UUID, version int) (*Batch, error) {
	if err := p.Require(authz.ImportRun); err != nil {
		return nil, err
	}
	if err := p.RequireStepUp(); err != nil {
		return nil, err
	}
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		status, content, err := lockBatch(ctx, tx, id)
		if err != nil {
			return err
		}
		var current, errorCount int
		if err := tx.QueryRow(ctx, `select row_version, error_count from import_batch where id = $1`, id).
			Scan(&current, &errorCount); err != nil {
			return fmt.Errorf("load batch: %w", err)
		}
		if current != version {
			return apperr.VersionMismatch
		}
		if status != "validated" {
			return apperr.New(apperr.KindConflict, "import.not_validated")
		}
		// Re-validate inside the commit transaction: reference data or users
		// may have changed since the dry run.
		rows, err := s.parse(ctx, tx, p, content)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.row.Status == "error" {
				return apperr.New(apperr.KindConflict, "import.has_errors").WithParams(map[string]any{"row": r.row.Number})
			}
		}
		if errorCount > 0 {
			return apperr.New(apperr.KindConflict, "import.has_errors")
		}
		matterIDs := map[string]uuid.UUID{}
		procIDs := map[string]uuid.UUID{}
		procMatter := map[string]uuid.UUID{}
		created := map[string]int{}
		for _, r := range rows {
			if r.skip {
				continue
			}
			var matterID, procID uuid.UUID
			switch {
			case r.existingProc != uuid.Nil:
				procID, matterID = r.existingProc, r.existingMat
			case procIDs[r.procKey] != uuid.Nil:
				procID, matterID = procIDs[r.procKey], procMatter[r.procKey]
			default:
				matterID = matterIDs[r.matterKey]
				if matterID == uuid.Nil {
					r.matter.ImportBatchID = id
					matterID, err = matters.CreateInTx(ctx, tx, p, r.matter)
					if err != nil {
						return rowErr(r.row.Number, err)
					}
					matterIDs[r.matterKey] = matterID
					created["matters"]++
				}
				r.proceeding.ImportBatchID = id
				procID, _, err = proceedings.CreateInTx(ctx, tx, p, matterID, r.proceeding)
				if err != nil {
					return rowErr(r.row.Number, err)
				}
				procIDs[r.procKey], procMatter[r.procKey] = procID, matterID
				created["proceedings"]++
			}
			r.hearing.ImportBatchID = id
			hearingID, _, err := hearings.CreateInTx(ctx, tx, p, procID, matterID, r.hearing)
			if err != nil {
				return rowErr(r.row.Number, err)
			}
			created["hearings"]++
			if _, err := tx.Exec(ctx, `update import_row set created_matter_id = $3, created_proceeding_id = $4,
				created_hearing_id = $5 where batch_id = $1 and row_number = $2`, id, r.row.Number, matterID, procID,
				hearingID); err != nil {
				return fmt.Errorf("record created ids: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `update import_batch set status = 'committed', committed_at = now(), committed_by = $2
			where id = $1`, id, p.UserID); err != nil {
			return fmt.Errorf("commit batch: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "import.committed",
			TargetKind: "import_batch", TargetID: id, Changes: map[string]any{"matters": created["matters"],
				"proceedings": created["proceedings"], "hearings": created["hearings"]}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func rowErr(n int, err error) error {
	e := apperr.As(err)
	if e.Kind == apperr.KindInternal {
		return err
	}
	params := map[string]any{"row": n, "cause": e.Code}
	return apperr.New(apperr.KindConflict, "import.row_failed").WithParams(params)
}

// Rollback undoes a committed batch while nobody has touched its records:
// hearings still scheduled with only their imported assertion, no
// verification, no outcome. Matters and proceedings it created are
// soft-deleted (doc 16 §4.1).
func (s *Service) Rollback(ctx context.Context, p *authz.Principal, id uuid.UUID) (*Batch, error) {
	if err := p.Require(authz.ImportRun); err != nil {
		return nil, err
	}
	if err := p.RequireStepUp(); err != nil {
		return nil, err
	}
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		status, _, err := lockBatch(ctx, tx, id)
		if err != nil {
			return err
		}
		if status != "committed" {
			return apperr.New(apperr.KindConflict, "import.not_committed")
		}
		var touched int
		if err := tx.QueryRow(ctx, `select count(*) from hearing h where h.import_batch_id = $1 and (
			h.listing_status <> 'scheduled' or h.updated_at > h.created_at
			or exists (select 1 from verification v where v.hearing_id = h.id)
			or (select count(*) from date_assertion a where a.hearing_id = h.id) > 1
			or exists (select 1 from hearing n where n.previous_hearing_id = h.id or n.replaces_hearing_id = h.id))`, id).
			Scan(&touched); err != nil {
			return fmt.Errorf("rollback check: %w", err)
		}
		if touched > 0 {
			return apperr.New(apperr.KindConflict, "import.rollback_blocked").WithParams(map[string]any{"touched": touched})
		}
		for _, q := range []string{
			`update hearing set listing_status = 'cancelled', status_reason = 'import_rolled_back' where import_batch_id = $1`,
			`update proceeding set deleted_at = now(), deleted_by = $2, delete_reason = 'import_rolled_back'
			 where import_batch_id = $1 and deleted_at is null`,
			`update matter set deleted_at = now(), deleted_by = $2, delete_reason = 'import_rolled_back'
			 where import_batch_id = $1 and deleted_at is null`,
			`update import_batch set status = 'rolled_back', rolled_back_at = now(), rolled_back_by = $2 where id = $1`,
		} {
			args := []any{id}
			if strings.Contains(q, "$2") {
				args = append(args, p.UserID)
			}
			if _, err := tx.Exec(ctx, q, args...); err != nil {
				return fmt.Errorf("rollback: %w", err)
			}
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "import.rolled_back",
			TargetKind: "import_batch", TargetID: id})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}
