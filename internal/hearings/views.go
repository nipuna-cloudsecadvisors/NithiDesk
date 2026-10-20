package hearings

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/audit"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/courts"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/clock"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/textnorm"
)

// Name is a bilingual label.
type Name struct {
	EN *string `json:"en,omitempty"`
	SI *string `json:"si,omitempty"`
}

// Ref is a reference item or court as shown on a card.
type Ref struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name Name      `json:"name"`
}

// Person is a user shown on a card.
type Person struct {
	UserID uuid.UUID `json:"user_id"`
	Name   string    `json:"name"`
}

// LastOutcome is the previous hearing's result, for "what happened last time".
type LastOutcome struct {
	HearingID   uuid.UUID `json:"hearing_id"`
	Date        string    `json:"date"`
	OutcomeCode string    `json:"outcome_code"`
	OutcomeName Name      `json:"outcome_name"`
	Summary     *string   `json:"summary,omitempty"`
	OrderText   *string   `json:"order_text,omitempty"`
}

// Card is one hearing in Today, the week view and the queues.
type Card struct {
	ID                  uuid.UUID    `json:"id"`
	Date                string       `json:"date"`
	Session             string       `json:"session"`
	Time                *string      `json:"time,omitempty"`
	Court               Ref          `json:"court"`
	Courtroom           *string      `json:"courtroom,omitempty"`
	Purpose             *Ref         `json:"purpose,omitempty"`
	PurposeNote         *string      `json:"purpose_note,omitempty"`
	ListingStatus       string       `json:"listing_status"`
	VerificationState   string       `json:"verification_state"`
	VerificationOverdue bool         `json:"verification_overdue"`
	PreparationStatus   string       `json:"preparation_status"`
	MatterID            uuid.UUID    `json:"matter_id"`
	MatterNumber        string       `json:"matter_number"`
	MatterTitle         Name         `json:"matter_title"`
	TitleMasked         bool         `json:"title_masked,omitempty"`
	ProceedingID        uuid.UUID    `json:"proceeding_id"`
	ProceedingTitle     Name         `json:"proceeding_title"`
	CaseNumber          *string      `json:"case_number,omitempty"`
	OurClients          []Name       `json:"our_clients"`
	Opposing            []Name       `json:"opposing"`
	Appearing           []Person     `json:"appearing"`
	Instructions        *string      `json:"instructions,omitempty"`
	LastOutcome         *LastOutcome `json:"last_outcome,omitempty"`
	Access              string       `json:"access"`
	Clash               bool         `json:"clash,omitempty"`
}

// cardSQL selects cards; callers append where/order. $1 is today.
const cardSQL = `
	select h.id, to_char(h.hearing_date, 'YYYY-MM-DD'), h.session, to_char(h.time_of_day, 'HH24:MI'),
	       c.id, c.code, c.name_en, c.name_si, cr.label,
	       pu.id, pu.code, pu.name_en, pu.name_si, h.purpose_note,
	       h.listing_status, h.verification_state, h.preparation_status,
	       m.id, m.matter_number, m.title_en, m.title_si, vm.title_masked, vm.access,
	       pr.id, pr.title_en, pr.title_si,
	       (select cn.raw from case_number_record cn where cn.proceeding_id = pr.id and cn.valid_to is null),
	       coalesce((select json_agg(json_build_object('en', e.name_en, 'si', e.name_si) order by prr.created_at)
	                 from party_role prr join entity e on e.id = prr.entity_id
	                 where prr.proceeding_id = pr.id and prr.side = 'our_client' and prr.status = 'active'), '[]'),
	       coalesce((select json_agg(json_build_object('en', e.name_en, 'si', e.name_si) order by coalesce(prr.ordinal, 0), prr.created_at)
	                 from party_role prr join entity e on e.id = prr.entity_id
	                 where prr.proceeding_id = pr.id and prr.side = 'opposing' and prr.status = 'active'), '[]'),
	       coalesce((select json_agg(json_build_object('user_id', u.id, 'name', coalesce(u.display_name_en, u.display_name_si))
	                                 order by u.display_name_en)
	                 from hearing_appearance ha join app_user u on u.id = ha.user_id where ha.hearing_id = h.id), '[]'),
	       h.instructions,
	       ph.id, to_char(ph.hearing_date, 'YYYY-MM-DD'), ot.code, ot.name_en, ot.name_si, po.summary, po.order_text
	from hearing h
	join vm on vm.matter_id = h.matter_id
	join matter m on m.id = h.matter_id
	join proceeding pr on pr.id = h.proceeding_id and pr.deleted_at is null
	join court c on c.id = h.court_id
	left join courtroom cr on cr.id = h.courtroom_id
	left join reference_item pu on pu.id = h.purpose_id
	left join hearing ph on ph.id = h.previous_hearing_id
	left join lateral (select o.* from outcome o where o.hearing_id = ph.id
	                   and not exists (select 1 from outcome c2 where c2.corrects_id = o.id)
	                   order by o.recorded_at desc limit 1) po on true
	left join reference_item ot on ot.id = po.outcome_type_id`

func scanCards(rows pgx.Rows) ([]Card, error) {
	defer rows.Close()
	out := []Card{}
	for rows.Next() {
		var c Card
		var purposeID *uuid.UUID
		var purposeCode, purposeEN, purposeSI *string
		var clients, opposing, appearing []byte
		var prevID *uuid.UUID
		var prevDate, outCode, outEN, outSI, outSummary, outOrder *string
		if err := rows.Scan(&c.ID, &c.Date, &c.Session, &c.Time,
			&c.Court.ID, &c.Court.Code, &c.Court.Name.EN, &c.Court.Name.SI, &c.Courtroom,
			&purposeID, &purposeCode, &purposeEN, &purposeSI, &c.PurposeNote,
			&c.ListingStatus, &c.VerificationState, &c.PreparationStatus,
			&c.MatterID, &c.MatterNumber, &c.MatterTitle.EN, &c.MatterTitle.SI, &c.TitleMasked, &c.Access,
			&c.ProceedingID, &c.ProceedingTitle.EN, &c.ProceedingTitle.SI, &c.CaseNumber,
			&clients, &opposing, &appearing, &c.Instructions,
			&prevID, &prevDate, &outCode, &outEN, &outSI, &outSummary, &outOrder); err != nil {
			return nil, fmt.Errorf("scan hearing card: %w", err)
		}
		if purposeID != nil {
			c.Purpose = &Ref{ID: *purposeID, Code: deref(purposeCode), Name: Name{EN: purposeEN, SI: purposeSI}}
		}
		_ = json.Unmarshal(clients, &c.OurClients)
		_ = json.Unmarshal(opposing, &c.Opposing)
		_ = json.Unmarshal(appearing, &c.Appearing)
		if prevID != nil && outCode != nil {
			c.LastOutcome = &LastOutcome{HearingID: *prevID, Date: deref(prevDate), OutcomeCode: *outCode,
				OutcomeName: Name{EN: outEN, SI: outSI}, Summary: outSummary, OrderText: outOrder}
		}
		// Diary access (clerk scope): metadata only. Notes, instructions and
		// order text stay with the matter team (doc 09 §2.1); a non-standard
		// matter also hides its title and parties.
		if c.Access != string(authz.AccessFull) {
			c.Instructions = nil
			if c.LastOutcome != nil {
				c.LastOutcome.Summary, c.LastOutcome.OrderText = nil, nil
			}
		}
		if c.TitleMasked {
			c.MatterTitle, c.ProceedingTitle = Name{}, Name{}
			c.OurClients, c.Opposing = []Name{}, []Name{}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// verificationWindow is the firm setting verification_window_days (default 3
// working days, doc 06 §4.3 [MAKE CONFIGURABLE]).
func verificationWindow(ctx context.Context, tx pgx.Tx, firmID uuid.UUID) (int, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `select settings from firm where id = $1`, firmID).Scan(&raw); err != nil {
		return 0, fmt.Errorf("firm settings: %w", err)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if v, ok := m["verification_window_days"].(float64); ok && v >= 0 && v <= 30 {
		return int(v), nil
	}
	return 3, nil
}

// windowEnd returns the date `days` working days after today, skipping
// weekends and firm-wide calendar days.
func windowEnd(ctx context.Context, tx pgx.Tx, days int) (time.Time, error) {
	today := clock.Today()
	cal, err := courts.CalendarDays(ctx, tx, today, today.AddDate(0, 0, days*3+14))
	if err != nil {
		return today, err
	}
	closed := map[string]bool{}
	for _, d := range cal {
		if d.CourtID == nil {
			closed[d.Day] = true
		}
	}
	end := today
	for n := 0; n < days; {
		end = end.AddDate(0, 0, 1)
		if wd := end.Weekday(); wd == time.Saturday || wd == time.Sunday || closed[clock.FormatDate(end)] {
			continue
		}
		n++
	}
	return end, nil
}

func markOverdue(cards []Card, end time.Time) {
	for i := range cards {
		d, err := clock.ParseDate(cards[i].Date)
		if err == nil && cards[i].ListingStatus == StatusScheduled && cards[i].VerificationState != Verified &&
			!d.After(end) {
			cards[i].VerificationOverdue = true
		}
	}
}

// markClashes flags a lawyer listed at two courts in the same session.
func markClashes(cards []Card) {
	type key struct {
		user    uuid.UUID
		date    string
		session string
	}
	courtsBy := map[key]map[uuid.UUID]bool{}
	for _, c := range cards {
		if c.ListingStatus != StatusScheduled {
			continue
		}
		for _, a := range c.Appearing {
			for _, s := range sessionsOf(c.Session) {
				k := key{a.UserID, c.Date, s}
				if courtsBy[k] == nil {
					courtsBy[k] = map[uuid.UUID]bool{}
				}
				courtsBy[k][c.Court.ID] = true
			}
		}
	}
	for i, c := range cards {
		for _, a := range c.Appearing {
			for _, s := range sessionsOf(c.Session) {
				if len(courtsBy[key{a.UserID, c.Date, s}]) > 1 {
					cards[i].Clash = true
				}
			}
		}
	}
}

// An unspecified session may clash with either.
func sessionsOf(s string) []string {
	if s == "unspecified" {
		return []string{"morning", "afternoon"}
	}
	return []string{s}
}

// DayView is Today: hearings grouped by court.
type DayView struct {
	Date        string               `json:"date"`
	Courts      []CourtDay           `json:"courts"`
	Counts      DayCounts            `json:"counts"`
	CalendarDay []courts.CalendarDay `json:"calendar_days"`
}

// CourtDay is one court's hearings on a day.
type CourtDay struct {
	Court    Ref    `json:"court"`
	Hearings []Card `json:"hearings"`
}

// DayCounts are the badges at the top of Today.
type DayCounts struct {
	Hearings            int `json:"hearings"`
	Unverified          int `json:"unverified"`
	Conflicts           int `json:"conflicts"`
	OutcomesNotRecorded int `json:"outcomes_not_recorded"`
	Clashes             int `json:"clashes"`
}

// Scopes for diary views.
const (
	ScopeMine = "mine"
	ScopeTeam = "team"
	ScopeFirm = "firm"
)

// scopeFilter limits a diary query: mine = I appear; team = matters I am on;
// firm = everything I may see (clerks and partners).
func scopeFilter(scope string) string {
	switch scope {
	case ScopeMine:
		return ` and exists (select 1 from hearing_appearance a where a.hearing_id = h.id and a.user_id = $2)`
	case ScopeTeam:
		return ` and exists (select 1 from matter_member mm where mm.matter_id = h.matter_id and mm.user_id = $2
		         and mm.valid_to is null)`
	default:
		return ` and $2::uuid is not null`
	}
}

// Today returns one day's hearings for the caller.
func (s *Service) Today(ctx context.Context, p *authz.Principal, date time.Time, scope string, courtID *uuid.UUID) (*DayView, error) {
	if scope == "" {
		scope = ScopeMine
	}
	out := &DayView{Date: clock.FormatDate(date), Courts: []CourtDay{}}
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+cardSQL+`
			where h.hearing_date = $3 and h.listing_status not in ('vacated', 'cancelled')
			  and ($4::uuid is null or h.court_id = $4)`+scopeFilter(scope)+`
			  and $1::date is not null
			order by c.sort_order, c.name_en, h.session, h.time_of_day nulls last, m.matter_number`,
			clock.Today(), p.UserID, date, courtID)
		if err != nil {
			return fmt.Errorf("today: %w", err)
		}
		cards, err := scanCards(rows)
		if err != nil {
			return err
		}
		days, err := verificationWindow(ctx, tx, p.FirmID)
		if err != nil {
			return err
		}
		end, err := windowEnd(ctx, tx, days)
		if err != nil {
			return err
		}
		markOverdue(cards, end)
		markClashes(cards)
		for _, c := range cards {
			if n := len(out.Courts); n == 0 || out.Courts[n-1].Court.ID != c.Court.ID {
				out.Courts = append(out.Courts, CourtDay{Court: c.Court})
			}
			out.Courts[len(out.Courts)-1].Hearings = append(out.Courts[len(out.Courts)-1].Hearings, c)
			out.Counts.Hearings++
			if c.VerificationState == Unverified {
				out.Counts.Unverified++
			}
			if c.VerificationState == Conflict {
				out.Counts.Conflicts++
			}
			if c.Clash {
				out.Counts.Clashes++
			}
		}
		if err := tx.QueryRow(ctx, `with `+authz.VisibleMattersCTE+`
			select count(*) from hearing h join vm on vm.matter_id = h.matter_id
			where h.hearing_date < $1 and h.listing_status in ('scheduled', 'held', 'not_reached', 'court_not_sitting')`+
			scopeFilter(scope), clock.Today(), p.UserID).Scan(&out.Counts.OutcomesNotRecorded); err != nil {
			return fmt.Errorf("outcome count: %w", err)
		}
		out.CalendarDay, err = courts.CalendarDays(ctx, tx, date, date)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// WeekView is the weekly planner (doc 06 §7).
type WeekView struct {
	Start      string     `json:"start"`
	End        string     `json:"end"`
	Days       []DayGroup `json:"days"`
	Highlights Highlights `json:"highlights"`
}

// DayGroup is one day in the planner.
type DayGroup struct {
	Date     string `json:"date"`
	Hearings []Card `json:"hearings"`
}

// Highlights are the planner's priority counts.
type Highlights struct {
	Conflicts           int `json:"conflicts"`
	VerificationOverdue int `json:"verification_overdue"`
	Clashes             int `json:"clashes"`
	NotReady            int `json:"not_ready"`
	NoAppearingLawyer   int `json:"no_appearing_lawyer"`
}

// Week returns days start…start+days-1, optionally for one lawyer or court.
func (s *Service) Week(ctx context.Context, p *authz.Principal, start time.Time, days int, scope string, lawyer, courtID *uuid.UUID) (*WeekView, error) {
	if days <= 0 || days > 31 {
		days = 7
	}
	if scope == "" {
		scope = ScopeTeam
	}
	end := start.AddDate(0, 0, days-1)
	out := &WeekView{Start: clock.FormatDate(start), End: clock.FormatDate(end), Days: []DayGroup{}}
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+cardSQL+`
			where h.hearing_date between $3 and $4 and h.listing_status not in ('vacated', 'cancelled')
			  and ($5::uuid is null or h.court_id = $5)
			  and ($6::uuid is null or exists (select 1 from hearing_appearance a where a.hearing_id = h.id and a.user_id = $6))`+
			scopeFilter(scope)+` and $1::date is not null
			order by h.hearing_date, c.sort_order, h.session, m.matter_number`,
			clock.Today(), p.UserID, start, end, courtID, lawyer)
		if err != nil {
			return fmt.Errorf("week: %w", err)
		}
		cards, err := scanCards(rows)
		if err != nil {
			return err
		}
		window, err := verificationWindow(ctx, tx, p.FirmID)
		if err != nil {
			return err
		}
		wEnd, err := windowEnd(ctx, tx, window)
		if err != nil {
			return err
		}
		markOverdue(cards, wEnd)
		markClashes(cards)
		for _, c := range cards {
			if n := len(out.Days); n == 0 || out.Days[n-1].Date != c.Date {
				out.Days = append(out.Days, DayGroup{Date: c.Date})
			}
			out.Days[len(out.Days)-1].Hearings = append(out.Days[len(out.Days)-1].Hearings, c)
			if c.VerificationState == Conflict {
				out.Highlights.Conflicts++
			}
			if c.VerificationOverdue {
				out.Highlights.VerificationOverdue++
			}
			if c.Clash {
				out.Highlights.Clashes++
			}
			if c.ListingStatus == StatusScheduled && c.PreparationStatus != "ready" {
				out.Highlights.NotReady++
			}
			if c.ListingStatus == StatusScheduled && len(c.Appearing) == 0 {
				out.Highlights.NoAppearingLawyer++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// VerificationQueue lists scheduled hearings whose date is unverified or in
// conflict, nearest first; conflicts first (doc 06 §7.2).
func (s *Service) VerificationQueue(ctx context.Context, p *authz.Principal, courtID *uuid.UUID, withinDays int) ([]Card, error) {
	if withinDays <= 0 || withinDays > 120 {
		withinDays = 30
	}
	var out []Card
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+cardSQL+`
			where h.listing_status = 'scheduled' and h.verification_state <> 'verified'
			  and h.hearing_date between $1 and $1 + $3::int and ($4::uuid is null or h.court_id = $4)
			  and $2::uuid is not null
			order by (h.verification_state = 'conflict') desc, h.hearing_date, c.sort_order, m.matter_number`,
			clock.Today(), p.UserID, withinDays, courtID)
		if err != nil {
			return fmt.Errorf("verification queue: %w", err)
		}
		if out, err = scanCards(rows); err != nil {
			return err
		}
		window, err := verificationWindow(ctx, tx, p.FirmID)
		if err != nil {
			return err
		}
		end, err := windowEnd(ctx, tx, window)
		if err != nil {
			return err
		}
		markOverdue(out, end)
		return nil
	})
	return out, err
}

// OutcomeNotRecorded lists past hearings with no outcome (I-OU-5). It
// should be empty; anything here is a possible missed next date.
func (s *Service) OutcomeNotRecorded(ctx context.Context, p *authz.Principal, scope string) ([]Card, error) {
	if scope == "" {
		scope = ScopeFirm
	}
	var out []Card
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+cardSQL+`
			where h.hearing_date < $1 and h.listing_status in ('scheduled', 'held', 'not_reached', 'court_not_sitting')`+
			scopeFilter(scope)+` order by h.hearing_date, c.sort_order`, clock.Today(), p.UserID)
		if err != nil {
			return fmt.Errorf("outcome queue: %w", err)
		}
		out, err = scanCards(rows)
		return err
	})
	return out, err
}

// CourtListResult is the outcome of a court-list tick-off (doc 06 §4.4).
type CourtListResult struct {
	Matched       []CourtListMatch `json:"matched"`
	DifferentDate []CourtListMatch `json:"different_date"`
	Missing       []Card           `json:"missing"`    // ours, expected, not on the list
	Unexpected    []string         `json:"unexpected"` // on the list, not in our diary
}

// CourtListMatch pairs a number seen on the list with our hearing.
type CourtListMatch struct {
	Number    string `json:"number"`
	HearingID string `json:"hearing_id"`
	Verified  bool   `json:"verified"`
}

// CourtListInput is what the clerk read off the list.
type CourtListInput struct {
	CourtID      uuid.UUID `json:"court_id"`
	ListDate     string    `json:"list_date"`
	Numbers      []string  `json:"numbers"`
	SourceDetail string    `json:"source_detail"`
	// VerifyMatches verifies the current date of every matched hearing with
	// source "official court list".
	VerifyMatches bool `json:"verify_matches"`
}

// CourtListCheck compares the numbers on a court list with our diary for
// that court and date. Matches can be verified; a number whose hearing we
// hold for a different date gets an official-court-list assertion (and so a
// conflict to resolve); misses and unexpected listings are reported.
func (s *Service) CourtListCheck(ctx context.Context, p *authz.Principal, in CourtListInput) (*CourtListResult, error) {
	if err := p.Require(authz.CourtListCheck); err != nil {
		return nil, err
	}
	date, err := clock.ParseDate(in.ListDate)
	if err != nil {
		return nil, apperr.Invalid(apperr.Field("list_date", "date.invalid"))
	}
	if len(in.Numbers) > 2000 {
		return nil, apperr.Invalid(apperr.Field("numbers", "too_many"))
	}
	seen := map[string]string{}
	for _, raw := range in.Numbers {
		if n := courts.NormaliseCaseNumber(raw); n != "" {
			seen[n] = strings.TrimSpace(raw)
		}
	}
	res := &CourtListResult{Matched: []CourtListMatch{}, DifferentDate: []CourtListMatch{}, Missing: []Card{}, Unexpected: []string{}}
	err = db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		listSrc, _, err := courts.ItemByCode(ctx, tx, courts.KindDateSourceType, "official_court_list")
		if err != nil {
			return err
		}
		detail := strings.TrimSpace(in.SourceDetail)
		if detail == "" {
			detail = "Court list " + in.ListDate
		}
		// Our hearings at this court on the date.
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+cardSQL+`
			where h.court_id = $3 and h.hearing_date = $4 and h.listing_status = 'scheduled' and $2::uuid is not null
			  and $1::date is not null`, clock.Today(), p.UserID, in.CourtID, date)
		if err != nil {
			return fmt.Errorf("court list: %w", err)
		}
		ours, err := scanCards(rows)
		if err != nil {
			return err
		}
		matchedNorm := map[string]bool{}
		for _, c := range ours {
			norm := courts.NormaliseCaseNumber(deref(c.CaseNumber))
			raw, onList := seen[norm]
			if c.CaseNumber == nil || !onList {
				res.Missing = append(res.Missing, c)
				continue
			}
			matchedNorm[norm] = true
			m := CourtListMatch{Number: raw, HearingID: c.ID.String()}
			if in.VerifyMatches && c.VerificationState != Verified {
				curID, _, _, err := currentAssertion(ctx, tx, c.ID)
				if err != nil {
					return err
				}
				if err := verifyInTx(ctx, tx, p, c.ID, c.MatterID, VerifyInput{DateAssertionID: curID,
					SourceTypeID: listSrc, EvidenceReference: detail}); err != nil {
					return err
				}
				m.Verified = true
			}
			res.Matched = append(res.Matched, m)
		}
		// Numbers not matched today: our hearing for that case is on another
		// date (assert the list date), or the case is not in our diary.
		for norm, raw := range seen {
			if matchedNorm[norm] {
				continue
			}
			var hearingID, matterID uuid.UUID
			err := tx.QueryRow(ctx, `with `+authz.VisibleMattersCTE+`
				select h.id, h.matter_id from hearing h join vm on vm.matter_id = h.matter_id
				join case_number_record cn on cn.proceeding_id = h.proceeding_id and cn.valid_to is null
				where cn.court_id = $1 and cn.normalised = $2 and h.listing_status = 'scheduled'
				order by abs(h.hearing_date - $3) limit 1`, in.CourtID, norm, date).Scan(&hearingID, &matterID)
			if err == pgx.ErrNoRows {
				res.Unexpected = append(res.Unexpected, raw)
				continue
			}
			if err != nil {
				return fmt.Errorf("court list lookup: %w", err)
			}
			curID, _, _, err := currentAssertion(ctx, tx, hearingID)
			if err != nil {
				return err
			}
			claim := parsedClaim{date: date, session: "unspecified", sourceID: listSrc,
				authority: courts.AuthorityAuthoritative, detail: &detail}
			if _, err := insertAssertion(ctx, tx, p, hearingID, claim, KindFromCourtList, &curID); err != nil {
				return err
			}
			if err := recompute(ctx, tx, hearingID); err != nil {
				return err
			}
			res.DifferentDate = append(res.DifferentDate, CourtListMatch{Number: raw, HearingID: hearingID.String()})
		}
		sort.Strings(res.Unexpected)
		resultJSON, _ := json.Marshal(map[string]int{"matched": len(res.Matched), "different_date": len(res.DifferentDate),
			"missing": len(res.Missing), "unexpected": len(res.Unexpected)})
		numbers := make([]string, 0, len(seen))
		for _, raw := range seen {
			numbers = append(numbers, raw)
		}
		if _, err := tx.Exec(ctx, `insert into court_list_check (firm_id, court_id, list_date, numbers_seen, result,
			source_note, checked_by) values ($1, $2, $3, $4, $5, $6, $7)`, p.FirmID, in.CourtID, date, numbers, resultJSON,
			nullable(in.SourceDetail), p.UserID); err != nil {
			if code, _, ok := db.Constraint(err); ok && code == "23503" {
				return apperr.Invalid(apperr.Field("court_id", "reference.unknown"))
			}
			return fmt.Errorf("record court list check: %w", err)
		}
		return audit.Append(ctx, tx, audit.Event{FirmID: p.FirmID, Actor: actorOf(p), Action: "diary.court_list_checked",
			TargetKind: "court", TargetID: in.CourtID, Changes: map[string]any{"list_date": in.ListDate,
				"matched": len(res.Matched), "different_date": len(res.DifferentDate), "missing": len(res.Missing),
				"unexpected": len(res.Unexpected)}})
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Assertion is one date claim in a hearing's history.
type Assertion struct {
	ID         uuid.UUID `json:"id"`
	Date       string    `json:"date"`
	Session    string    `json:"session"`
	Time       *string   `json:"time,omitempty"`
	SourceCode string    `json:"source_code"`
	SourceName Name      `json:"source_name"`
	Authority  string    `json:"authority"`
	Detail     *string   `json:"source_detail,omitempty"`
	Kind       string    `json:"kind"`
	AssertedBy *string   `json:"asserted_by,omitempty"`
	AssertedAt time.Time `json:"asserted_at"`
	Current    bool      `json:"current"`
}

// VerificationView is one verification.
type VerificationView struct {
	ID           uuid.UUID  `json:"id"`
	AssertionID  uuid.UUID  `json:"date_assertion_id"`
	SourceCode   string     `json:"source_code"`
	Evidence     string     `json:"evidence_reference"`
	VerifiedBy   string     `json:"verified_by"`
	VerifiedAt   time.Time  `json:"verified_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokeReason *string    `json:"revoke_reason,omitempty"`
}

// OutcomeView is a recorded outcome.
type OutcomeView struct {
	ID            uuid.UUID  `json:"id"`
	OutcomeCode   string     `json:"outcome_code"`
	OutcomeName   Name       `json:"outcome_name"`
	Summary       *string    `json:"summary,omitempty"`
	OrderText     *string    `json:"order_text,omitempty"`
	NextHearingID *uuid.UUID `json:"next_hearing_id,omitempty"`
	NoNextReason  *string    `json:"no_next_reason,omitempty"`
	InformClient  bool       `json:"inform_client"`
	CertifiedCopy bool       `json:"certified_copy"`
	RecordedBy    string     `json:"recorded_by"`
	RecordedAt    time.Time  `json:"recorded_at"`
	Corrects      *uuid.UUID `json:"corrects_id,omitempty"`
}

// Note is a court-day note.
type Note struct {
	ID        uuid.UUID `json:"id"`
	Body      string    `json:"body"`
	By        string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Detail is a full hearing.
type Detail struct {
	Card
	Notes           *string            `json:"notes,omitempty"`
	PreparationNote *string            `json:"preparation_note,omitempty"`
	StatusReason    *string            `json:"status_reason,omitempty"`
	PreviousHearing *uuid.UUID         `json:"previous_hearing_id,omitempty"`
	ReplacesHearing *uuid.UUID         `json:"replaces_hearing_id,omitempty"`
	ReplacedBy      *uuid.UUID         `json:"replaced_by_hearing_id,omitempty"`
	NextHearing     *uuid.UUID         `json:"next_hearing_id,omitempty"`
	AssignedClerk   *Person            `json:"assigned_clerk,omitempty"`
	Assertions      []Assertion        `json:"date_assertions"`
	Verifications   []VerificationView `json:"verifications"`
	Outcomes        []OutcomeView      `json:"outcomes"`
	CourtNotes      []Note             `json:"court_notes"`
	Version         int                `json:"row_version"`
}

// Get returns a hearing with its date history (diary access or better).
func (s *Service) Get(ctx context.Context, p *authz.Principal, id uuid.UUID) (*Detail, error) {
	var d Detail
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+cardSQL+` where h.id = $2 and $1::date is not null`,
			clock.Today(), id)
		if err != nil {
			return fmt.Errorf("load hearing: %w", err)
		}
		cards, err := scanCards(rows)
		if err != nil {
			return err
		}
		if len(cards) == 0 {
			return apperr.NotFound
		}
		d.Card = cards[0]
		full := d.Access == string(authz.AccessFull)
		var clerkID *uuid.UUID
		var clerkName *string
		if err := tx.QueryRow(ctx, `select h.notes, h.preparation_note, h.status_reason, h.previous_hearing_id,
			h.replaces_hearing_id, (select r.id from hearing r where r.replaces_hearing_id = h.id limit 1),
			(select o.next_hearing_id from outcome o where o.hearing_id = h.id and o.corrects_id is null),
			h.assigned_clerk_id, coalesce(u.display_name_en, u.display_name_si), h.row_version
			from hearing h left join app_user u on u.id = h.assigned_clerk_id where h.id = $1`, id).
			Scan(&d.Notes, &d.PreparationNote, &d.StatusReason, &d.PreviousHearing, &d.ReplacesHearing, &d.ReplacedBy,
				&d.NextHearing, &clerkID, &clerkName, &d.Version); err != nil {
			return fmt.Errorf("load hearing: %w", err)
		}
		if clerkID != nil {
			d.AssignedClerk = &Person{UserID: *clerkID, Name: deref(clerkName)}
		}
		if !full {
			d.Notes, d.PreparationNote = nil, nil
		}
		if d.Assertions, err = loadAssertions(ctx, tx, id); err != nil {
			return err
		}
		if d.Verifications, err = loadVerifications(ctx, tx, id); err != nil {
			return err
		}
		if d.Outcomes, err = loadOutcomes(ctx, tx, id, full); err != nil {
			return err
		}
		d.CourtNotes = []Note{}
		if full {
			d.CourtNotes, err = loadNotes(ctx, tx, id)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	window := 3
	_ = db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		w, err := verificationWindow(ctx, tx, p.FirmID)
		if err == nil {
			window = w
			end, err := windowEnd(ctx, tx, window)
			if err == nil {
				cards := []Card{d.Card}
				markOverdue(cards, end)
				d.Card = cards[0]
			}
		}
		return nil
	})
	return &d, nil
}

func loadAssertions(ctx context.Context, tx pgx.Tx, id uuid.UUID) ([]Assertion, error) {
	rows, err := tx.Query(ctx, `select a.id, to_char(a.asserted_date, 'YYYY-MM-DD'), a.asserted_session,
		to_char(a.asserted_time, 'HH24:MI'), s.code, s.name_en, s.name_si, a.authority, a.source_detail, a.kind,
		coalesce(u.display_name_en, u.display_name_si), a.asserted_at,
		not exists (select 1 from date_assertion b where b.supersedes_id = a.id)
		from date_assertion a join reference_item s on s.id = a.source_type_id left join app_user u on u.id = a.asserted_by
		where a.hearing_id = $1 order by a.asserted_at, a.id`, id)
	if err != nil {
		return nil, fmt.Errorf("load assertions: %w", err)
	}
	defer rows.Close()
	out := []Assertion{}
	for rows.Next() {
		var a Assertion
		if err := rows.Scan(&a.ID, &a.Date, &a.Session, &a.Time, &a.SourceCode, &a.SourceName.EN, &a.SourceName.SI,
			&a.Authority, &a.Detail, &a.Kind, &a.AssertedBy, &a.AssertedAt, &a.Current); err != nil {
			return nil, fmt.Errorf("load assertions: %w", err)
		}
		out = append(out, a)
	}
	// Only the latest non-superseded assertion is current.
	for i := range out {
		out[i].Current = out[i].Current && i == lastCurrent(out)
	}
	return out, rows.Err()
}

func lastCurrent(as []Assertion) int {
	idx := -1
	for i, a := range as {
		if a.Current {
			idx = i
		}
	}
	return idx
}

func loadVerifications(ctx context.Context, tx pgx.Tx, id uuid.UUID) ([]VerificationView, error) {
	rows, err := tx.Query(ctx, `select v.id, v.date_assertion_id, s.code, v.evidence_reference,
		coalesce(u.display_name_en, u.display_name_si), v.verified_at, v.revoked_at, v.revoke_reason
		from verification v join reference_item s on s.id = v.source_type_id join app_user u on u.id = v.verified_by
		where v.hearing_id = $1 order by v.verified_at`, id)
	if err != nil {
		return nil, fmt.Errorf("load verifications: %w", err)
	}
	defer rows.Close()
	out := []VerificationView{}
	for rows.Next() {
		var v VerificationView
		if err := rows.Scan(&v.ID, &v.AssertionID, &v.SourceCode, &v.Evidence, &v.VerifiedBy, &v.VerifiedAt,
			&v.RevokedAt, &v.RevokeReason); err != nil {
			return nil, fmt.Errorf("load verifications: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func loadOutcomes(ctx context.Context, tx pgx.Tx, id uuid.UUID, full bool) ([]OutcomeView, error) {
	rows, err := tx.Query(ctx, `select o.id, t.code, t.name_en, t.name_si, o.summary, o.order_text, o.next_hearing_id,
		o.no_next_reason, o.inform_client, o.certified_copy, coalesce(u.display_name_en, u.display_name_si), o.recorded_at,
		o.corrects_id
		from outcome o join reference_item t on t.id = o.outcome_type_id join app_user u on u.id = o.recorded_by
		where o.hearing_id = $1 order by o.recorded_at`, id)
	if err != nil {
		return nil, fmt.Errorf("load outcomes: %w", err)
	}
	defer rows.Close()
	out := []OutcomeView{}
	for rows.Next() {
		var o OutcomeView
		if err := rows.Scan(&o.ID, &o.OutcomeCode, &o.OutcomeName.EN, &o.OutcomeName.SI, &o.Summary, &o.OrderText,
			&o.NextHearingID, &o.NoNextReason, &o.InformClient, &o.CertifiedCopy, &o.RecordedBy, &o.RecordedAt,
			&o.Corrects); err != nil {
			return nil, fmt.Errorf("load outcomes: %w", err)
		}
		if !full {
			o.Summary, o.OrderText = nil, nil
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func loadNotes(ctx context.Context, tx pgx.Tx, id uuid.UUID) ([]Note, error) {
	rows, err := tx.Query(ctx, `select n.id, n.body, coalesce(u.display_name_en, u.display_name_si), n.created_at
		from hearing_note n join app_user u on u.id = n.created_by where n.hearing_id = $1 order by n.created_at`, id)
	if err != nil {
		return nil, fmt.Errorf("load notes: %w", err)
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Body, &n.By, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("load notes: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ForProceeding lists a proceeding's hearings, newest first.
func (s *Service) ForProceeding(ctx context.Context, p *authz.Principal, proceedingID uuid.UUID) ([]Card, error) {
	var out []Card
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+cardSQL+`
			where h.proceeding_id = $2 and $1::date is not null order by h.hearing_date desc, h.created_at desc`,
			clock.Today(), proceedingID)
		if err != nil {
			return fmt.Errorf("proceeding hearings: %w", err)
		}
		out, err = scanCards(rows)
		return err
	})
	return out, err
}

// SearchResult groups global search hits (M1: matters, proceedings,
// hearings by case number). Restricted matters never appear (doc 07 §9).
type SearchResult struct {
	Matters     []SearchHit `json:"matters"`
	Proceedings []SearchHit `json:"proceedings"`
}

// SearchHit is one search result.
type SearchHit struct {
	ID           uuid.UUID `json:"id"`
	MatterID     uuid.UUID `json:"matter_id"`
	MatterNumber string    `json:"matter_number"`
	Title        Name      `json:"title"`
	CaseNumber   *string   `json:"case_number,omitempty"`
	CourtCode    *string   `json:"court_code,omitempty"`
	MatchedOn    string    `json:"matched_on"` // matter_number, case_number, title, party
}

// Search finds matters and proceedings by matter number, any current or
// past case number, title, or party name (Sinhala or English).
func (s *Service) Search(ctx context.Context, p *authz.Principal, q string) (*SearchResult, error) {
	q = strings.TrimSpace(q)
	res := &SearchResult{Matters: []SearchHit{}, Proceedings: []SearchHit{}}
	if len([]rune(q)) < 2 {
		return res, nil
	}
	caseNorm := courts.NormaliseCaseNumber(q)
	latin := textnorm.LatinText(q)
	key := textnorm.QueryKey(q)
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with `+authz.VisibleMattersCTE+`
			select m.id, m.matter_number, case when vm.title_masked then null else m.title_en end,
			       case when vm.title_masked then null else m.title_si end,
			       case when m.matter_number ilike $1 || '%' then 'matter_number' else 'title' end
			from matter m join vm on vm.matter_id = m.id
			where m.matter_number ilike $1 || '%'
			   or (not vm.title_masked and ($2 <% m.title_latin or m.title_latin ilike '%' || $2 || '%'))
			order by (m.matter_number ilike $1 || '%') desc, word_similarity($2, m.title_latin) desc
			limit 20`, q, latin)
		if err != nil {
			return fmt.Errorf("search matters: %w", err)
		}
		for rows.Next() {
			var h SearchHit
			if err := rows.Scan(&h.ID, &h.MatterNumber, &h.Title.EN, &h.Title.SI, &h.MatchedOn); err != nil {
				rows.Close()
				return fmt.Errorf("search matters: %w", err)
			}
			h.MatterID = h.ID
			res.Matters = append(res.Matters, h)
		}
		rows.Close()
		rows, err = tx.Query(ctx, `with `+authz.VisibleMattersCTE+`,
			hits as (
				select cn.proceeding_id as id, 'case_number' as matched, 1 as rank from case_number_record cn
				where cn.normalised = $1 or cn.normalised like $1 || '%'
				union all
				select pr.id, 'title', 2 from proceeding pr where $2 <% pr.title_latin
				union all
				select prr.proceeding_id, 'party', 3 from party_role prr join entity e on e.id = prr.entity_id
				where $2 <% e.name_latin or $3 <% e.name_key
			)
			select distinct on (pr.id) pr.id, m.id, m.matter_number,
			       case when vm.title_masked then null else pr.title_en end,
			       case when vm.title_masked then null else pr.title_si end,
			       (select cn.raw from case_number_record cn where cn.proceeding_id = pr.id and cn.valid_to is null),
			       c.code, hits.matched
			from hits join proceeding pr on pr.id = hits.id and pr.deleted_at is null
			join vm on vm.matter_id = pr.matter_id join matter m on m.id = pr.matter_id
			join court c on c.id = pr.current_court_id
			where hits.matched <> 'party' or not vm.title_masked
			order by pr.id, hits.rank
			limit 30`, caseNorm, latin, key)
		if err != nil {
			return fmt.Errorf("search proceedings: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var h SearchHit
			if err := rows.Scan(&h.ID, &h.MatterID, &h.MatterNumber, &h.Title.EN, &h.Title.SI, &h.CaseNumber,
				&h.CourtCode, &h.MatchedOn); err != nil {
				return fmt.Errorf("search proceedings: %w", err)
			}
			res.Proceedings = append(res.Proceedings, h)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(res.Proceedings, func(i, j int) bool {
		return res.Proceedings[i].MatchedOn == "case_number" && res.Proceedings[j].MatchedOn != "case_number"
	})
	return res, nil
}
