// Package matters owns matters, their clients and team, and ethical walls
// (doc 04 §4, doc 09 §5). Every read goes through nd_visible_matters(), so a
// matter the caller may not see is reported as not found.
package matters

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
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/textnorm"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/registry"
)

// Team roles.
const (
	RoleResponsiblePartner = "responsible_partner"
	RoleLead               = "lead"
	RoleSupporting         = "supporting"
	RoleClerk              = "clerk"
	RoleAssistant          = "assistant"
)

var teamRoles = map[string]bool{RoleResponsiblePartner: true, RoleLead: true, RoleSupporting: true, RoleClerk: true,
	RoleAssistant: true}

// Errors.
var (
	ErrWalledMember = apperr.New(apperr.KindConflict, "wall.member_excluded")
)

// Title is a bilingual title.
type Title struct {
	EN *string `json:"en,omitempty"`
	SI *string `json:"si,omitempty"`
}

// Member is one current team member.
type Member struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	TeamRole    string    `json:"team_role"`
	ValidFrom   string    `json:"valid_from"`
}

// Client is one client of the matter.
type Client struct {
	EntityID uuid.UUID     `json:"entity_id"`
	Name     registry.Name `json:"name"`
	Role     string        `json:"client_role"`
}

// Summary is a matter in a list.
type Summary struct {
	ID               uuid.UUID  `json:"id"`
	MatterNumber     string     `json:"matter_number"`
	Title            Title      `json:"title"`
	TitleMasked      bool       `json:"title_masked,omitempty"`
	Lifecycle        string     `json:"lifecycle"`
	Priority         string     `json:"priority"`
	Confidentiality  string     `json:"confidentiality"`
	OpenedOn         string     `json:"opened_on"`
	Access           string     `json:"access"`
	NextHearingDate  *string    `json:"next_hearing_date,omitempty"`
	NextHearingID    *uuid.UUID `json:"next_hearing_id,omitempty"`
	NextVerification *string    `json:"next_hearing_verification,omitempty"`
	CaseNumbers      []string   `json:"case_numbers"`
}

// Detail is a matter page.
type Detail struct {
	Summary
	Description    *string    `json:"description,omitempty"`
	PracticeAreaID *uuid.UUID `json:"practice_area_id,omitempty"`
	PhysicalFile   *string    `json:"physical_file,omitempty"`
	ClosedOn       *string    `json:"closed_on,omitempty"`
	Clients        []Client   `json:"clients"`
	Team           []Member   `json:"team"`
	Flags          []string   `json:"attention_flags"`
	Version        int        `json:"row_version"`
}

// Service implements matters.
type Service struct {
	pool *pgxpool.Pool
}

// New builds the service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func actorOf(p *authz.Principal) audit.Actor {
	return audit.Actor{Kind: audit.ActorUser, UserID: p.UserID, SessionID: p.SessionID, RequestID: p.RequestID, IP: p.IP}
}

// ClientInput names an existing entity or creates one.
type ClientInput struct {
	EntityID *uuid.UUID      `json:"entity_id"`
	New      *registry.Input `json:"new"`
	Role     string          `json:"client_role"`
}

// MemberInput adds a user to the team.
type MemberInput struct {
	UserID   uuid.UUID `json:"user_id"`
	TeamRole string    `json:"team_role"`
}

// CreateInput opens a matter.
type CreateInput struct {
	TitleEN         string        `json:"title_en"`
	TitleSI         string        `json:"title_si"`
	Description     string        `json:"description"`
	PracticeAreaID  *uuid.UUID    `json:"practice_area_id"`
	Priority        string        `json:"priority"`
	Confidentiality string        `json:"confidentiality"`
	OpenedOn        string        `json:"opened_on"`
	PhysicalFile    string        `json:"physical_file"`
	Clients         []ClientInput `json:"clients"`
	Team            []MemberInput `json:"team"`
	// Set by the importer; empty for interactive creation.
	CreatedVia    string    `json:"-"`
	ImportBatchID uuid.UUID `json:"-"`
}

func (in *CreateInput) validate() []apperr.FieldError {
	var fields []apperr.FieldError
	if strings.TrimSpace(in.TitleEN) == "" && strings.TrimSpace(in.TitleSI) == "" {
		fields = append(fields, apperr.Field("title_en", "required"))
	}
	if in.Priority == "" {
		in.Priority = "normal"
	}
	if in.Priority != "normal" && in.Priority != "high" && in.Priority != "urgent" {
		fields = append(fields, apperr.Field("priority", "invalid"))
	}
	if in.Confidentiality == "" {
		in.Confidentiality = "standard"
	}
	switch in.Confidentiality {
	case "standard", "restricted", "highly_restricted":
	default:
		fields = append(fields, apperr.Field("confidentiality", "invalid"))
	}
	if in.OpenedOn != "" {
		if _, err := time.Parse(time.DateOnly, in.OpenedOn); err != nil {
			fields = append(fields, apperr.Field("opened_on", "date.invalid"))
		}
	}
	if len(in.Clients) == 0 {
		fields = append(fields, apperr.Field("clients", "required"))
	}
	for i, c := range in.Clients {
		if c.Role == "" {
			in.Clients[i].Role = "primary"
		}
		if c.EntityID == nil && c.New == nil {
			fields = append(fields, apperr.Field(fmt.Sprintf("clients[%d]", i), "required"))
		}
		if c.New != nil {
			fields = append(fields, c.New.Validate(fmt.Sprintf("clients[%d].new.", i))...)
		}
	}
	partners, leads := 0, 0
	for i, m := range in.Team {
		if !teamRoles[m.TeamRole] {
			fields = append(fields, apperr.Field(fmt.Sprintf("team[%d].team_role", i), "invalid"))
		}
		if m.TeamRole == RoleResponsiblePartner {
			partners++
		}
		if m.TeamRole == RoleLead {
			leads++
		}
	}
	if partners != 1 {
		fields = append(fields, apperr.Field("team", "matter.one_responsible_partner"))
	}
	if leads > 1 {
		fields = append(fields, apperr.Field("team", "matter.one_lead"))
	}
	return fields
}

// Create opens a matter (matter.create). The creator joins the team as lead
// when no lead is named, so they can see what they created.
func (s *Service) Create(ctx context.Context, p *authz.Principal, in CreateInput) (*Detail, error) {
	if err := p.Require(authz.MatterCreate); err != nil {
		return nil, err
	}
	var id uuid.UUID
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		var err error
		id, err = CreateInTx(ctx, tx, p, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

// CreateInTx opens a matter inside an existing transaction (also used by the
// importer). The caller has checked capabilities.
func CreateInTx(ctx context.Context, tx pgx.Tx, p *authz.Principal, in CreateInput) (uuid.UUID, error) {
	hasLead, creatorOnTeam := false, false
	for _, m := range in.Team {
		if m.TeamRole == RoleLead {
			hasLead = true
		}
		if m.UserID == p.UserID {
			creatorOnTeam = true
		}
	}
	if !hasLead && !creatorOnTeam {
		in.Team = append(in.Team, MemberInput{UserID: p.UserID, TeamRole: RoleLead})
	}
	if fields := in.validate(); len(fields) > 0 {
		return uuid.Nil, apperr.Invalid(fields...)
	}
	if in.PracticeAreaID != nil {
		if _, _, err := courts.ItemByID(ctx, tx, courts.KindPracticeArea, *in.PracticeAreaID); err != nil {
			return uuid.Nil, err
		}
	}
	opened := clock.Today()
	if in.OpenedOn != "" {
		opened, _ = time.Parse(time.DateOnly, in.OpenedOn)
	}
	number, err := nextMatterNumber(ctx, tx, p.FirmID, opened.Year())
	if err != nil {
		return uuid.Nil, err
	}
	via := in.CreatedVia
	if via == "" {
		via = "user"
	}
	en := strings.TrimSpace(textnorm.Normalise(in.TitleEN, true))
	si := strings.TrimSpace(textnorm.Normalise(in.TitleSI, true))
	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		insert into matter (firm_id, matter_number, title_en, title_si, title_latin, description, practice_area_id,
		                    priority, confidentiality, opened_on, physical_file, created_by, created_via, import_batch_id)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) returning id`,
		p.FirmID, number, nullable(en), nullable(si), latinTitle(en, si), nullable(in.Description), in.PracticeAreaID,
		in.Priority, in.Confidentiality, opened, nullable(in.PhysicalFile), p.UserID, via, nullUUID(in.ImportBatchID),
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create matter: %w", err)
	}
	for _, c := range in.Clients {
		entityID := uuid.Nil
		if c.EntityID != nil {
			entityID = *c.EntityID
		} else {
			createdVia := "user"
			if via == "import" {
				createdVia = "import"
				if existing, ok, err := registry.FindExact(ctx, tx, *c.New); err != nil {
					return uuid.Nil, err
				} else if ok {
					entityID = existing
				}
			}
			if entityID == uuid.Nil {
				if entityID, err = registry.Create(ctx, tx, p.FirmID, p.UserID, *c.New, createdVia); err != nil {
					return uuid.Nil, err
				}
			}
		}
		if _, err := tx.Exec(ctx, `insert into matter_client (firm_id, matter_id, entity_id, client_role, created_by)
			values ($1, $2, $3, $4, $5) on conflict do nothing`, p.FirmID, id, entityID, c.Role, p.UserID); err != nil {
			if code, _, ok := db.Constraint(err); ok && code == "23503" {
				return uuid.Nil, apperr.Invalid(apperr.Field("clients", "entity.unknown"))
			}
			return uuid.Nil, fmt.Errorf("add client: %w", err)
		}
	}
	seen := map[uuid.UUID]bool{}
	for _, m := range in.Team {
		if seen[m.UserID] {
			continue
		}
		seen[m.UserID] = true
		if err := addMember(ctx, tx, p, id, m); err != nil {
			return uuid.Nil, err
		}
	}
	return id, audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "matter.opened",
		TargetKind: "matter", TargetID: id, MatterID: id,
		Changes: map[string]any{"matter_number": number, "confidentiality": in.Confidentiality, "via": via}})
}

func addMember(ctx context.Context, tx pgx.Tx, p *authz.Principal, matterID uuid.UUID, m MemberInput) error {
	walled, err := authz.UserWalledFrom(ctx, tx, m.UserID, matterID)
	if err != nil {
		return err
	}
	if walled {
		return ErrWalledMember
	}
	var scope string
	err = tx.QueryRow(ctx, `select coalesce(max(case rt.matter_scope when 'none' then 0 else 1 end), 0)::text
		from user_role ur join role_template rt on rt.id = ur.role_template_id
		join app_user u on u.id = ur.user_id
		where ur.user_id = $1 and ur.valid_to is null and u.status <> 'disabled'`, m.UserID).Scan(&scope)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if scope != "1" {
		// Unknown, disabled, or administrator-only users cannot join a team
		// (I-AC-3: the administrator never sees matter content).
		return apperr.Invalid(apperr.Field("team", "member.not_eligible"))
	}
	_, err = tx.Exec(ctx, `insert into matter_member (firm_id, matter_id, user_id, team_role, created_by)
		values ($1, $2, $3, $4, $5)`, p.FirmID, matterID, m.UserID, m.TeamRole, p.UserID)
	if db.IsUniqueViolation(err, "matter_member_one_partner") || db.IsUniqueViolation(err, "matter_member_one_lead") {
		return apperr.New(apperr.KindConflict, "matter.team_role_taken").WithParams(map[string]any{"team_role": m.TeamRole})
	}
	if db.IsUniqueViolation(err, "matter_member_current") {
		return apperr.New(apperr.KindConflict, "matter.already_member")
	}
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "matter.member_added",
		TargetKind: "user", TargetID: m.UserID, MatterID: matterID, Changes: map[string]any{"team_role": m.TeamRole}})
}

// nextMatterNumber issues the next number for the firm's format. The format
// is the firm setting matter_number_format, default {YYYY}/{SEQ:4}.
func nextMatterNumber(ctx context.Context, tx pgx.Tx, firmID uuid.UUID, year int) (string, error) {
	var settings []byte
	if err := tx.QueryRow(ctx, `select settings from firm where id = $1`, firmID).Scan(&settings); err != nil {
		return "", fmt.Errorf("load firm settings: %w", err)
	}
	var cfg struct {
		Format string `json:"matter_number_format"`
	}
	_ = json.Unmarshal(settings, &cfg)
	if cfg.Format == "" {
		cfg.Format = "{YYYY}/{SEQ:4}"
	}
	var seq int
	if err := tx.QueryRow(ctx, `
		insert into matter_number_counter (firm_id, year, last_seq) values ($1, $2, 1)
		on conflict (firm_id, year) do update set last_seq = matter_number_counter.last_seq + 1
		returning last_seq`, firmID, year).Scan(&seq); err != nil {
		return "", fmt.Errorf("matter number: %w", err)
	}
	out := strings.ReplaceAll(cfg.Format, "{YYYY}", fmt.Sprintf("%04d", year))
	out = strings.ReplaceAll(out, "{YY}", fmt.Sprintf("%02d", year%100))
	for width := 1; width <= 8; width++ {
		out = strings.ReplaceAll(out, fmt.Sprintf("{SEQ:%d}", width), fmt.Sprintf("%0*d", width, seq))
	}
	return strings.ReplaceAll(out, "{SEQ}", fmt.Sprint(seq)), nil
}

func latinTitle(en, si string) string {
	parts := []string{}
	for _, t := range []string{en, si} {
		if l := textnorm.LatinText(t); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, " / ")
}

const summarySQL = `
	select m.id, m.matter_number, m.title_en, m.title_si, vm.title_masked, m.lifecycle, m.priority, m.confidentiality,
	       to_char(m.opened_on, 'YYYY-MM-DD'), vm.access,
	       nh.hearing_date_text, nh.id, nh.verification_state,
	       coalesce((select array_agg(c.raw order by c.raw) from case_number_record c
	                 join proceeding pr on pr.firm_id = c.firm_id and pr.id = c.proceeding_id
	                 where pr.matter_id = m.id and pr.deleted_at is null and c.valid_to is null), '{}')
	from matter m
	join vm on vm.matter_id = m.id
	left join lateral (
		select h.id, to_char(h.hearing_date, 'YYYY-MM-DD') as hearing_date_text, h.verification_state
		from hearing h
		where h.matter_id = m.id and h.listing_status = 'scheduled' and h.hearing_date >= $1
		order by h.hearing_date, h.session limit 1) nh on true`

func scanSummary(row pgx.Row) (Summary, error) {
	var s Summary
	err := row.Scan(&s.ID, &s.MatterNumber, &s.Title.EN, &s.Title.SI, &s.TitleMasked, &s.Lifecycle, &s.Priority,
		&s.Confidentiality, &s.OpenedOn, &s.Access, &s.NextHearingDate, &s.NextHearingID, &s.NextVerification,
		&s.CaseNumbers)
	if s.TitleMasked {
		s.Title = Title{}
	}
	return s, err
}

// ListFilter filters the matter list.
type ListFilter struct {
	Query     string
	Lifecycle string
	Mine      bool
	Limit     int
	Cursor    string // matter_number of the last row
}

// List returns visible matters, newest number first.
func (s *Service) List(ctx context.Context, p *authz.Principal, f ListFilter) ([]Summary, string, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	var out []Summary
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		latin := textnorm.LatinText(f.Query)
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+summarySQL+`
			where ($2 = '' or m.lifecycle = $2)
			  and (not $3 or exists (select 1 from matter_member mm where mm.matter_id = m.id and mm.user_id = $4
			                          and mm.valid_to is null))
			  and ($5 = '' or (not vm.title_masked and ($5 <% m.title_latin or m.title_latin ilike '%' || $5 || '%'))
			       or m.matter_number ilike $6 || '%'
			       or exists (select 1 from case_number_record c join proceeding pr on pr.id = c.proceeding_id
			                  where pr.matter_id = m.id and c.normalised like $7 || '%'))
			  and ($8 = '' or m.matter_number < $8)
			order by m.matter_number desc
			limit $9`,
			clock.Today(), f.Lifecycle, f.Mine, p.UserID, latin, strings.TrimSpace(f.Query),
			courts.NormaliseCaseNumber(f.Query), f.Cursor, f.Limit+1)
		if err != nil {
			return fmt.Errorf("list matters: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			sm, err := scanSummary(rows)
			if err != nil {
				return fmt.Errorf("list matters: %w", err)
			}
			out = append(out, sm)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > f.Limit {
		out = out[:f.Limit]
		next = out[len(out)-1].MatterNumber
	}
	return out, next, nil
}

// Get returns a matter page. A caller with diary access gets the metadata
// only; one without access gets not found.
func (s *Service) Get(ctx context.Context, p *authz.Principal, id uuid.UUID) (*Detail, error) {
	var d Detail
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `with `+authz.VisibleMattersCTE+summarySQL+` where m.id = $2`, clock.Today(), id)
		sm, err := scanSummary(row)
		if err == pgx.ErrNoRows {
			return apperr.NotFound
		}
		if err != nil {
			return fmt.Errorf("load matter: %w", err)
		}
		d.Summary = sm
		if err := tx.QueryRow(ctx, `select description, practice_area_id, physical_file, to_char(closed_on, 'YYYY-MM-DD'),
			row_version from matter where id = $1`, id).Scan(&d.Description, &d.PracticeAreaID, &d.PhysicalFile,
			&d.ClosedOn, &d.Version); err != nil {
			return fmt.Errorf("load matter: %w", err)
		}
		if d.Access != string(authz.AccessFull) {
			d.Description, d.PhysicalFile = nil, nil
		}
		if d.Team, err = loadTeam(ctx, tx, id); err != nil {
			return err
		}
		if d.Clients, err = loadClients(ctx, tx, id, d.TitleMasked); err != nil {
			return err
		}
		d.Flags, err = flags(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func loadTeam(ctx context.Context, tx pgx.Tx, matterID uuid.UUID) ([]Member, error) {
	rows, err := tx.Query(ctx, `select mm.id, mm.user_id, coalesce(u.display_name_en, u.display_name_si), mm.team_role,
		to_char(mm.valid_from, 'YYYY-MM-DD')
		from matter_member mm join app_user u on u.id = mm.user_id
		where mm.matter_id = $1 and mm.valid_to is null
		order by array_position(array['responsible_partner','lead','supporting','clerk','assistant'], mm.team_role)`, matterID)
	if err != nil {
		return nil, fmt.Errorf("load team: %w", err)
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.UserID, &m.DisplayName, &m.TeamRole, &m.ValidFrom); err != nil {
			return nil, fmt.Errorf("load team: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func loadClients(ctx context.Context, tx pgx.Tx, matterID uuid.UUID, masked bool) ([]Client, error) {
	out := []Client{}
	if masked {
		return out, nil
	}
	rows, err := tx.Query(ctx, `select e.id, e.name_en, e.name_si, e.name_ta, mc.client_role
		from matter_client mc join entity e on e.id = mc.entity_id where mc.matter_id = $1 order by mc.created_at`, matterID)
	if err != nil {
		return nil, fmt.Errorf("load clients: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.EntityID, &c.Name.EN, &c.Name.SI, &c.Name.TA, &c.Role); err != nil {
			return nil, fmt.Errorf("load clients: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// flags derives attention flags; they are never stored (doc 08 §5).
func flags(ctx context.Context, tx pgx.Tx, matterID uuid.UUID) ([]string, error) {
	var unverified, notRecorded, soon bool
	err := tx.QueryRow(ctx, `select
		exists (select 1 from hearing where matter_id = $1 and listing_status = 'scheduled'
		        and verification_state <> 'verified' and hearing_date between $2 and $2 + 14),
		exists (select 1 from hearing where matter_id = $1 and listing_status in ('scheduled', 'held', 'not_reached',
		        'court_not_sitting') and hearing_date < $2),
		exists (select 1 from hearing where matter_id = $1 and listing_status = 'scheduled'
		        and hearing_date between $2 and $2 + 7)`, matterID, clock.Today()).Scan(&unverified, &notRecorded, &soon)
	if err != nil {
		return nil, fmt.Errorf("matter flags: %w", err)
	}
	out := []string{}
	if soon {
		out = append(out, "hearing_within_7_days")
	}
	if unverified {
		out = append(out, "unverified_upcoming_date")
	}
	if notRecorded {
		out = append(out, "outcome_not_recorded")
	}
	return out, nil
}

// UpdateInput changes controlled matter fields.
type UpdateInput struct {
	TitleEN        *string    `json:"title_en"`
	TitleSI        *string    `json:"title_si"`
	Description    *string    `json:"description"`
	PracticeAreaID *uuid.UUID `json:"practice_area_id"`
	Priority       *string    `json:"priority"`
	Lifecycle      *string    `json:"lifecycle"`
	PhysicalFile   *string    `json:"physical_file"`
}

// Update changes a matter (matter.update, full access, If-Match). Closure
// with partner approval is M2; M1 allows open ↔ on hold only.
func (s *Service) Update(ctx context.Context, p *authz.Principal, id uuid.UUID, version int, in UpdateInput) (*Detail, error) {
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		if _, _, err := authz.RequireMatter(ctx, tx, id, authz.AccessFull); err != nil {
			return err
		}
		if err := p.Require(authz.MatterUpdate); err != nil {
			return err
		}
		if in.Priority != nil && *in.Priority != "normal" && *in.Priority != "high" && *in.Priority != "urgent" {
			return apperr.Invalid(apperr.Field("priority", "invalid"))
		}
		if in.Lifecycle != nil && *in.Lifecycle != "open" && *in.Lifecycle != "on_hold" {
			return apperr.Invalid(apperr.Field("lifecycle", "matter.lifecycle_transition_not_allowed"))
		}
		if in.PracticeAreaID != nil {
			if _, _, err := courts.ItemByID(ctx, tx, courts.KindPracticeArea, *in.PracticeAreaID); err != nil {
				return err
			}
		}
		var current int
		var en, si *string
		if err := tx.QueryRow(ctx, `select row_version, title_en, title_si from matter where id = $1 for update`, id).
			Scan(&current, &en, &si); err != nil {
			return fmt.Errorf("load matter: %w", err)
		}
		if current != version {
			return apperr.VersionMismatch
		}
		newEN, newSI := deref(en), deref(si)
		if in.TitleEN != nil {
			newEN = strings.TrimSpace(textnorm.Normalise(*in.TitleEN, true))
		}
		if in.TitleSI != nil {
			newSI = strings.TrimSpace(textnorm.Normalise(*in.TitleSI, true))
		}
		if newEN == "" && newSI == "" {
			return apperr.Invalid(apperr.Field("title_en", "required"))
		}
		if _, err := tx.Exec(ctx, `update matter set title_en = $2, title_si = $3, title_latin = $4,
			description = coalesce($5, description), practice_area_id = coalesce($6, practice_area_id),
			priority = coalesce($7, priority), lifecycle = coalesce($8, lifecycle),
			physical_file = coalesce($9, physical_file), updated_by = $10 where id = $1`,
			id, nullable(newEN), nullable(newSI), latinTitle(newEN, newSI), in.Description, in.PracticeAreaID,
			in.Priority, in.Lifecycle, in.PhysicalFile, p.UserID); err != nil {
			return fmt.Errorf("update matter: %w", err)
		}
		changed := map[string]any{}
		if in.Lifecycle != nil {
			changed["lifecycle"] = *in.Lifecycle
		}
		if in.Priority != nil {
			changed["priority"] = *in.Priority
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "matter.updated",
			TargetKind: "matter", TargetID: id, MatterID: id, Changes: changed})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

// AddMember adds a user to the team (matter.manage_team, full access).
func (s *Service) AddMember(ctx context.Context, p *authz.Principal, matterID uuid.UUID, in MemberInput) error {
	if !teamRoles[in.TeamRole] {
		return apperr.Invalid(apperr.Field("team_role", "invalid"))
	}
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		if _, _, err := authz.RequireMatter(ctx, tx, matterID, authz.AccessFull); err != nil {
			return err
		}
		if err := p.Require(authz.MatterManageTeam); err != nil {
			return err
		}
		return addMember(ctx, tx, p, matterID, in)
	})
}

// EndMember ends a current team membership; history is kept.
func (s *Service) EndMember(ctx context.Context, p *authz.Principal, matterID, memberID uuid.UUID) error {
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		if _, _, err := authz.RequireMatter(ctx, tx, matterID, authz.AccessFull); err != nil {
			return err
		}
		if err := p.Require(authz.MatterManageTeam); err != nil {
			return err
		}
		var userID uuid.UUID
		var role string
		err := tx.QueryRow(ctx, `update matter_member set valid_to = $3, ended_by = $4
			where id = $1 and matter_id = $2 and valid_to is null returning user_id, team_role`,
			memberID, matterID, clock.Today(), p.UserID).Scan(&userID, &role)
		if err == pgx.ErrNoRows {
			return apperr.NotFound
		}
		if err != nil {
			return fmt.Errorf("end member: %w", err)
		}
		if role == RoleResponsiblePartner {
			return apperr.New(apperr.KindConflict, "matter.responsible_partner_required")
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "matter.member_ended",
			TargetKind: "user", TargetID: userID, MatterID: matterID, Changes: map[string]any{"team_role": role}})
	})
}

// SetConfidentiality changes a matter's level (matter.confidentiality,
// step-up, full access). Access re-evaluates on the next request.
func (s *Service) SetConfidentiality(ctx context.Context, p *authz.Principal, matterID uuid.UUID, level string) error {
	switch level {
	case "standard", "restricted", "highly_restricted":
	default:
		return apperr.Invalid(apperr.Field("confidentiality", "invalid"))
	}
	return db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		if _, _, err := authz.RequireMatter(ctx, tx, matterID, authz.AccessFull); err != nil {
			return err
		}
		if err := p.Require(authz.MatterConfidentiality); err != nil {
			return err
		}
		if err := p.RequireStepUp(); err != nil {
			return err
		}
		var from string
		if err := tx.QueryRow(ctx, `select confidentiality from matter where id = $1 for update`, matterID).Scan(&from); err != nil {
			return fmt.Errorf("load matter: %w", err)
		}
		if _, err := tx.Exec(ctx, `update matter set confidentiality = $2, updated_by = $3 where id = $1`,
			matterID, level, p.UserID); err != nil {
			return fmt.Errorf("set confidentiality: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "matter.confidentiality_changed",
			TargetKind: "matter", TargetID: matterID, MatterID: matterID, Changes: map[string]any{"from": from, "to": level}})
	})
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nullUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
