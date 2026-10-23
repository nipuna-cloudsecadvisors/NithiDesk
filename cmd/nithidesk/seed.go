package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/courts"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/hearings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/identity"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/matters"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/clock"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/secretbox"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/proceedings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/registry"
)

// demoUsers are synthetic people for a development firm. All names are
// fabricated combinations of common names; none is a real client or lawyer.
var demoUsers = []struct {
	email, nameEN, nameSI, role string
}{
	{"partner@demo.nithidesk.lk", "Anoma Jayasekara", "අනෝමා ජයසේකර", "managing_partner"},
	{"senior@demo.nithidesk.lk", "Ruwan Wickramasinghe", "රුවන් වික්\u200Dරමසිංහ", "senior_lawyer"},
	{"junior@demo.nithidesk.lk", "Dilani Gunasekara", "දිලානි ගුණසේකර", "junior_lawyer"},
	{"clerk@demo.nithidesk.lk", "Sunil Bandara", "සුනිල් බණ්ඩාර", "court_clerk"},
	{"admin@demo.nithidesk.lk", "System Admin", "පද්ධති පරිපාලක", "system_administrator"},
	{"walled@demo.nithidesk.lk", "Kasun Herath", "කසුන් හේරත්", "senior_lawyer"},
}

// seedDemo fills a development firm with users and a realistic diary, using
// the same service code the API uses. It refuses to run outside development.
func seedDemo(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("seed-demo", flag.ExitOnError)
	firm := fs.String("firm", "demo", "firm code")
	_ = fs.Parse(args)
	if env := os.Getenv("NITHIDESK_ENV"); env != "" && env != "development" {
		return errors.New("seed-demo runs only with NITHIDESK_ENV=development")
	}
	password := os.Getenv("NITHIDESK_USER_PASSWORD")
	if identity.CheckPasswordPolicy(password, "x@y") != "" {
		return errors.New("set NITHIDESK_USER_PASSWORD (12+ characters) for the demo users")
	}
	key, err := base64.StdEncoding.DecodeString(os.Getenv("NITHIDESK_SECRETS_KEY"))
	if err != nil || len(key) != 32 {
		return errors.New("NITHIDESK_SECRETS_KEY (32 bytes, base64) is needed to pre-enrol demo TOTP")
	}
	box, err := secretbox.New(key)
	if err != nil {
		return err
	}
	conn, err := ownerConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	hash, err := identity.HashPassword(password)
	if err != nil {
		return err
	}

	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var firmID uuid.UUID
		if err := tx.QueryRow(ctx, `select id from firm where code = $1`, *firm).Scan(&firmID); err != nil {
			return fmt.Errorf("unknown firm %q (run create-firm first)", *firm)
		}
		ids := map[string]uuid.UUID{}
		// One shared, printed TOTP secret for every demo user: development only.
		secret := []byte("nithidesk-demo-totp!")
		for _, u := range demoUsers {
			id, err := insertUser(ctx, tx, *firm, u.email, u.nameEN, u.nameSI, "en", hash, []string{u.role})
			if err != nil {
				return err
			}
			sealed, err := box.Seal(secret, []byte("totp:"+id.String()))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `insert into mfa_factor (firm_id, user_id, kind, secret_encrypted, confirmed_at)
				values ($1, $2, 'totp', $3, now())`, firmID, id, sealed); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update app_user set status = 'active' where id = $1`, id); err != nil {
				return err
			}
			ids[u.role+":"+u.email] = id
		}
		partner := ids["managing_partner:partner@demo.nithidesk.lk"]
		senior := ids["senior_lawyer:senior@demo.nithidesk.lk"]
		junior := ids["junior_lawyer:junior@demo.nithidesk.lk"]
		walled := ids["senior_lawyer:walled@demo.nithidesk.lk"]

		// Act as the managing partner, with the tenant context set so the
		// visibility function behaves as it does for the API.
		p := &authz.Principal{FirmID: firmID, UserID: partner, MatterScope: authz.ScopeMember,
			Capabilities: map[authz.Capability]bool{}}
		if _, err := tx.Exec(ctx, `select set_config('app.firm_id', $1, true), set_config('app.user_id', $2, true),
			set_config('app.matter_scope', 'member', true)`, firmID.String(), partner.String()); err != nil {
			return err
		}

		ref := func(kind, code string) uuid.UUID {
			id, _, err := courts.ItemByCode(ctx, tx, kind, code)
			if err != nil {
				panic(fmt.Sprintf("seed reference %s/%s missing", kind, code))
			}
			return id
		}
		court := func(code string) uuid.UUID {
			var id uuid.UUID
			if err := tx.QueryRow(ctx, `select id from court where firm_id = $1 and code = $2`, firmID, code).Scan(&id); err != nil {
				panic("seed court missing: " + code)
			}
			return id
		}
		today := clock.Today()
		day := func(offset int) string { return clock.FormatDate(today.AddDate(0, 0, offset)) }
		lawyerSrc := ref(courts.KindDateSourceType, "lawyer_entry")
		recordSrc := ref(courts.KindDateSourceType, "case_record")

		type demoCase struct {
			titleEN, titleSI, clientEN, clientSI, oppEN, oppSI string
			court, ptype, caseNo, area, conf                   string
			hearings                                           []struct {
				offset           int
				session, purpose string
				verify           bool
			}
		}
		h := func(offset int, session, purpose string, verify bool) struct {
			offset           int
			session, purpose string
			verify           bool
		} {
			return struct {
				offset           int
				session, purpose string
				verify           bool
			}{offset, session, purpose, verify}
		}
		cases := []demoCase{
			{"Perera v. Silva — land at Kotte", "පෙරේරා එදිරිව සිල්වා — කෝට්ටේ ඉඩම", "Nimal Perera", "නිමල් පෙරේරා",
				"Kamal Silva", "කමල් සිල්වා", "DC-COLOMBO", "dc_land", "L/1234/24", "land", "standard",
				[]struct {
					offset           int
					session, purpose string
					verify           bool
				}{h(-14, "morning", "trial", true), h(0, "morning", "further_trial", true)}},
			{"ABC (Pvt) Ltd v. Fernando — money recovery", "ඒබීසී පුද්. සමාගම එදිරිව ප්\u200Dරනාන්දු", "ABC (Private) Limited",
				"ඒබීසී පුද්ගලික සමාගම", "Sunil Fernando", "සුනිල් ප්\u200Dරනාන්දු", "DC-COLOMBO", "dc_money", "DMR/556/25",
				"money_recovery", "standard",
				[]struct {
					offset           int
					session, purpose string
					verify           bool
				}{h(0, "morning", "answer", false)}},
			{"Police v. Jayasinghe — bail", "පොලිසිය එදිරිව ජයසිංහ", "Chamari Jayasinghe", "චමරි ජයසිංහ",
				"Officer-in-Charge, Fort Police", "භාරනිලධාරී, කොටුව පොලිසිය", "MC-COLOMBO-FORT", "mc_b_report",
				"B/1234/26", "criminal", "restricted",
				[]struct {
					offset           int
					session, purpose string
					verify           bool
				}{h(0, "morning", "bail", true), h(7, "morning", "remand_extension", false)}},
			{"Dissanayake divorce", "දිසානායක දික්කසාද", "Thilini Dissanayake", "තිලිණි දිසානායක",
				"Ajith Dissanayake", "අජිත් දිසානායක", "DC-GAMPAHA", "dc_divorce", "DDV/88/25", "family", "highly_restricted",
				[]struct {
					offset           int
					session, purpose string
					verify           bool
				}{h(2, "morning", "inquiry", false)}},
			{"Gunawardena testamentary", "ගුණවර්ධන අන්තිම කැමති පත්\u200Dරය", "Saman Gunawardena", "සමන් ගුණවර්ධන",
				"Heirs of the deceased", "මියගිය අයගේ උරුමක්කාරයෝ", "DC-MT-LAVINIA", "dc_testamentary", "T/45/23",
				"testamentary", "standard",
				[]struct {
					offset           int
					session, purpose string
					verify           bool
				}{h(-3, "morning", "mention", false), h(1, "afternoon", "mention", false)}},
			{"Rajapaksa v. Ceylon Bank — appeal", "රාජපක්ෂ එදිරිව ලංකා බැංකුව", "Namal Rajapaksa", "නාමල් රාජපක්ෂ",
				"Ceylon Bank PLC", "ලංකා බැංකුව", "HCCA-WP-COLOMBO", "hcca_appeal", "WP/HCCA/COL/12/2024(F)", "commercial",
				"standard",
				[]struct {
					offset           int
					session, purpose string
					verify           bool
				}{h(0, "morning", "argument", false), h(4, "morning", "written_submissions", false)}},
		}
		plaintiff := ref(courts.KindPartyRoleType, "plaintiff")
		defendant := ref(courts.KindPartyRoleType, "defendant")
		var firstMatter uuid.UUID
		for i, c := range cases {
			area := ref(courts.KindPracticeArea, c.area)
			team := []matters.MemberInput{{UserID: partner, TeamRole: matters.RoleResponsiblePartner},
				{UserID: senior, TeamRole: matters.RoleLead}, {UserID: junior, TeamRole: matters.RoleSupporting}}
			if i == 0 {
				team = append(team, matters.MemberInput{UserID: walled, TeamRole: matters.RoleAssistant})
			}
			mid, err := matters.CreateInTx(ctx, tx, p, matters.CreateInput{TitleEN: c.titleEN, TitleSI: c.titleSI,
				PracticeAreaID: &area, Confidentiality: c.conf, CreatedVia: "system",
				Clients: []matters.ClientInput{{New: &registry.Input{Kind: kindOf(c.clientEN), NameEN: c.clientEN,
					NameSI: c.clientSI}}}, Team: team})
			if err != nil {
				return fmt.Errorf("seed matter %d: %w", i, err)
			}
			if i == 0 {
				firstMatter = mid
			}
			pid, _, err := proceedings.CreateInTx(ctx, tx, p, mid, proceedings.CreateInput{TypeID: ref(courts.KindProceedingType, c.ptype),
				CourtID: court(c.court), CaseNumber: c.caseNo, OurSide: "plaintiff_side", Parties: []proceedings.PartyInput{
					{New: &registry.Input{Kind: kindOf(c.clientEN), NameEN: c.clientEN, NameSI: c.clientSI}, RoleTypeID: plaintiff, Side: "our_client"},
					{New: &registry.Input{Kind: kindOf(c.oppEN), NameEN: c.oppEN, NameSI: c.oppSI}, RoleTypeID: defendant, Side: "opposing"},
				}})
			if err != nil {
				return fmt.Errorf("seed proceeding %d: %w", i, err)
			}
			for _, hh := range c.hearings {
				purpose := ref(courts.KindHearingPurpose, hh.purpose)
				appearing := []uuid.UUID{junior}
				if i%2 == 1 {
					appearing = []uuid.UUID{senior}
				}
				hid, _, err := hearings.CreateInTx(ctx, tx, p, pid, mid, hearings.CreateInput{
					DateClaim: hearings.DateClaim{Date: day(hh.offset), Session: hh.session, SourceTypeID: lawyerSrc,
						SourceDetail: "Demo data"},
					PurposeID: &purpose, Appearing: appearing, Instructions: "Demo instruction: confirm next steps with the client.",
					CreatedVia: "system"})
				if err != nil {
					return fmt.Errorf("seed hearing %d: %w", i, err)
				}
				if hh.verify {
					var aid uuid.UUID
					if err := tx.QueryRow(ctx, `select id from date_assertion where hearing_id = $1`, hid).Scan(&aid); err != nil {
						return err
					}
					if _, err := tx.Exec(ctx, `insert into verification (firm_id, hearing_id, date_assertion_id, source_type_id,
						evidence_reference, verified_by) values ($1, $2, $3, $4, 'Demo JE 12', $5)`,
						firmID, hid, aid, recordSrc, partner); err != nil {
						return err
					}
					if _, err := tx.Exec(ctx, `update hearing set verification_state = 'verified' where id = $1`, hid); err != nil {
						return err
					}
				}
			}
		}
		// One past hearing with an outcome (so "last time" shows), and one
		// verified date contradicted by a later claim (a conflict to resolve).
		if _, err := tx.Exec(ctx, `update hearing set listing_status = 'outcome_recorded'
			where firm_id = $1 and hearing_date < $2 and purpose_id = $3`, firmID, today, ref(courts.KindHearingPurpose, "trial")); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into outcome (firm_id, hearing_id, outcome_type_id, summary, order_text, next_hearing_id, recorded_by)
			select h.firm_id, h.id, $3, 'PW1 examined in chief; cross-examination to continue.',
			       'Plaintiff to tender documents marked P5 to P9 on the next date.', n.id, $4
			from hearing h join hearing n on n.proceeding_id = h.proceeding_id and n.hearing_date = $2
			where h.firm_id = $1 and h.listing_status = 'outcome_recorded' limit 1`,
			firmID, today, ref(courts.KindOutcomeType, "adjourned"), partner); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update hearing n set previous_hearing_id = h.id from outcome o join hearing h on h.id = o.hearing_id
			where n.id = o.next_hearing_id and n.firm_id = $1`, firmID); err != nil {
			return err
		}
		// The written-submissions date was verified from the record, then a
		// client said it moved a day later: the hearing is now in conflict.
		var subID, subAssertion uuid.UUID
		if err := tx.QueryRow(ctx, `select h.id, a.id from hearing h join date_assertion a on a.hearing_id = h.id
			where h.firm_id = $1 and h.purpose_id = $2 limit 1`, firmID, ref(courts.KindHearingPurpose, "written_submissions")).
			Scan(&subID, &subAssertion); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into verification (firm_id, hearing_id, date_assertion_id, source_type_id,
			evidence_reference, verified_by) values ($1, $2, $3, $4, 'Demo JE 7', $5)`,
			firmID, subID, subAssertion, recordSrc, partner); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into date_assertion (firm_id, hearing_id, asserted_date, asserted_session,
			source_type_id, authority, source_detail, kind, supersedes_id, asserted_by)
			select firm_id, hearing_id, asserted_date + 1, asserted_session, $3, 'hearsay', 'Client phoned', 'correction', id, $4
			from date_assertion where id = $2 and firm_id = $1`,
			firmID, subAssertion, ref(courts.KindDateSourceType, "client"), partner); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update hearing set hearing_date = hearing_date + 1, verification_state = 'conflict'
			where id = $1`, subID); err != nil {
			return err
		}
		// Wall the "walled" lawyer off the first matter (they were on its team).
		var wallID uuid.UUID
		if err := tx.QueryRow(ctx, `insert into ethical_wall (firm_id, reason, created_by)
			values ($1, 'Demo: previously advised the opposing party', $2) returning id`, firmID, partner).Scan(&wallID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into ethical_wall_matter values ($1, $2, $3)`, firmID, wallID, firstMatter); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into ethical_wall_user values ($1, $2, $3)`, firmID, wallID, walled); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update matter_member set valid_to = $3 where matter_id = $1 and user_id = $2 and valid_to is null`,
			firstMatter, walled, today); err != nil {
			return err
		}
		fmt.Println("demo firm seeded:")
		for _, u := range demoUsers {
			fmt.Printf("  %-28s %s\n", u.email, u.role)
		}
		fmt.Println("  password: NITHIDESK_USER_PASSWORD")
		fmt.Println("  TOTP secret for every demo user (add to an authenticator app):", identity.EncodeSecret(secret))
		return nil
	})
}

func kindOf(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "limited") || strings.Contains(lower, "plc") || strings.Contains(lower, "(pvt)"):
		return registry.KindOrganisation
	case strings.Contains(lower, "police") || strings.Contains(lower, "officer-in-charge"):
		return registry.KindGovernment
	}
	return registry.KindPerson
}
