package itest

import (
	"context"
	"strings"
	"testing"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
)

// world is the shared cast for authorisation scenarios:
//
//	partner  managing partner, on the team of "open" and "restricted"
//	senior   senior lawyer, on "open" only
//	outsider senior lawyer, on no team
//	walled   senior lawyer, walled off "open" (and so off its hearings)
//	clerk    court clerk (diary scope)
//	admin    system administrator
type world struct {
	e                                               *env
	partner, senior, outsider, walled, clerk, admin *client
	open, restricted, highly                        fixture
}

func newWorld(t *testing.T) *world {
	e := newEnv(t)
	e.firm("alpha")
	partnerID := e.user("alpha", "partner@alpha.lk", "managing_partner")
	seniorID := e.user("alpha", "senior@alpha.lk", "senior_lawyer")
	e.user("alpha", "outsider@alpha.lk", "senior_lawyer")
	walledID := e.user("alpha", "walled@alpha.lk", "senior_lawyer")
	e.user("alpha", "clerk@alpha.lk", "court_clerk")
	e.user("alpha", "admin@alpha.lk", "system_administrator")

	w := &world{e: e}
	w.partner = e.login("alpha", "partner@alpha.lk")
	w.senior = e.login("alpha", "senior@alpha.lk")
	w.outsider = e.login("alpha", "outsider@alpha.lk")
	w.walled = e.login("alpha", "walled@alpha.lk")
	w.clerk = e.login("alpha", "clerk@alpha.lk")
	w.admin = e.login("alpha", "admin@alpha.lk")

	w.open = w.partner.matter("Open matter Perera", "standard",
		[]map[string]any{member(partnerID, "responsible_partner"), member(seniorID, "lead"), member(walledID, "supporting")}, todayStr())
	w.restricted = w.partner.matter("Restricted matter Jayasinghe", "restricted",
		[]map[string]any{member(partnerID, "responsible_partner")}, todayStr())
	w.highly = w.partner.matter("Highly restricted Dissanayake", "highly_restricted",
		[]map[string]any{member(partnerID, "responsible_partner")}, todayStr())

	// Wall the walled lawyer off the open matter. Walls need step-up; a
	// login counts as a fresh step-up for five minutes.
	if st, code := w.partner.errCode("POST", "/api/v1/ethical-walls", map[string]any{
		"reason": "Acted for the opposing party", "matter_ids": []string{w.open.MatterID},
		"user_ids": []string{walledID.String()}}); st != 201 {
		t.Fatalf("create wall: %d %s", st, code)
	}
	return w
}

func TestAuthorisationScenarios(t *testing.T) {
	w := newWorld(t)
	covered := map[string]bool{}
	scenario := func(id string, fn func(t *testing.T)) {
		covered[id] = true
		t.Run(id, fn)
	}

	scenario("direct_url_without_matter_team", func(t *testing.T) {
		if st, code := w.outsider.errCode("GET", "/api/v1/matters/"+w.open.MatterID, nil); st != 404 || code != "not_found" {
			t.Fatalf("outsider GET matter = %d %s, want 404 not_found", st, code)
		}
		if st, _ := w.outsider.errCode("GET", "/api/v1/hearings/"+w.open.HearingID, nil); st != 404 {
			t.Fatalf("outsider GET hearing = %d, want 404", st)
		}
		if st, _ := w.outsider.errCode("GET", "/api/v1/proceedings/"+w.open.ProceedingID, nil); st != 404 {
			t.Fatalf("outsider GET proceeding = %d, want 404", st)
		}
		// Writes are refused the same way: not found, never "forbidden".
		if st, _ := w.outsider.errCode("POST", "/api/v1/hearings/"+w.open.HearingID+"/notes",
			map[string]any{"body": "x"}, "Idempotency-Key", idemKey()); st != 404 {
			t.Fatalf("outsider note = %d, want 404", st)
		}
	})

	scenario("restricted_matter_returns_not_found", func(t *testing.T) {
		// The walled user is excluded from the open matter even though they
		// were on its team; a malformed id looks exactly like a missing one.
		for _, path := range []string{"/api/v1/matters/" + w.open.MatterID, "/api/v1/matters/not-a-uuid"} {
			if st, code := w.walled.errCode("GET", path, nil); st != 404 || code != "not_found" {
				t.Fatalf("walled GET %s = %d %s, want 404 not_found", path, st, code)
			}
		}
	})

	scenario("restricted_matter_absent_from_search", func(t *testing.T) {
		var res struct {
			Matters []struct {
				ID string `json:"id"`
			} `json:"matters"`
			Proceedings []struct {
				MatterID string `json:"matter_id"`
			} `json:"proceedings"`
		}
		for _, q := range []string{"Perera", "Open matter", "පෙරේරා"} {
			w.walled.do("GET", "/api/v1/search?q="+urlq(q), nil, &res)
			for _, m := range res.Matters {
				if m.ID == w.open.MatterID {
					t.Fatalf("walled user found the walled matter searching %q", q)
				}
			}
			for _, p := range res.Proceedings {
				if p.MatterID == w.open.MatterID {
					t.Fatalf("walled user found the walled proceeding searching %q", q)
				}
			}
		}
		// The team can find it, including by the Sinhala party name.
		w.senior.do("GET", "/api/v1/search?q="+urlq("පෙරේරා"), nil, &res)
		if !containsMatter(res.Proceedings, w.open.MatterID) {
			t.Fatalf("team member cannot find the matter by Sinhala party name")
		}
	})

	scenario("restricted_matter_absent_from_counts_and_totals", func(t *testing.T) {
		var day struct {
			Counts struct {
				Hearings int `json:"hearings"`
			} `json:"counts"`
		}
		w.walled.do("GET", "/api/v1/diary/today?scope=firm", nil, &day)
		if day.Counts.Hearings != 0 {
			t.Fatalf("walled user's firm-wide Today counts %d hearings, want 0", day.Counts.Hearings)
		}
		var list struct {
			Items []struct{ ID string } `json:"items"`
		}
		w.walled.do("GET", "/api/v1/matters", nil, &list)
		if len(list.Items) != 0 {
			t.Fatalf("walled user lists %d matters, want 0", len(list.Items))
		}
	})

	scenario("managing_partner_is_not_implicitly_on_every_matter", func(t *testing.T) {
		seniorOnly := w.senior.matter("Senior's own matter", "standard",
			[]map[string]any{member(w.e.users["senior@alpha.lk"], "lead"),
				member(w.e.users["outsider@alpha.lk"], "responsible_partner")}, dayStr(3))
		if st, _ := w.partner.errCode("GET", "/api/v1/matters/"+seniorOnly.MatterID, nil); st != 404 {
			t.Fatalf("managing partner GET a matter they are not on = %d, want 404", st)
		}
	})

	diaryPath := map[string]string{
		"today_view_excludes_walled_matters":                 "/api/v1/diary/today?scope=firm",
		"weekly_view_excludes_walled_matters":                "/api/v1/diary/week?scope=firm",
		"verification_queue_excludes_walled_matters":         "/api/v1/queues/verification",
		"outcome_not_recorded_queue_excludes_walled_matters": "/api/v1/queues/outcome-not-recorded",
	}
	for id, path := range diaryPath {
		scenario(id, func(t *testing.T) {
			var raw map[string]any
			w.walled.do("GET", path, nil, &raw)
			if s := toJSON(raw); strings.Contains(s, w.open.HearingID) || strings.Contains(s, w.open.MatterID) {
				t.Fatalf("%s leaked the walled matter: %s", path, s)
			}
		})
	}

	scenario("administrator_sees_no_matter_content", func(t *testing.T) {
		for _, path := range []string{"/api/v1/matters/" + w.open.MatterID, "/api/v1/hearings/" + w.open.HearingID} {
			if st, _ := w.admin.errCode("GET", path, nil); st != 404 {
				t.Fatalf("admin GET %s = %d, want 404", path, st)
			}
		}
		var day struct {
			Counts struct {
				Hearings int `json:"hearings"`
			} `json:"counts"`
		}
		w.admin.do("GET", "/api/v1/diary/today?scope=firm", nil, &day)
		if day.Counts.Hearings != 0 {
			t.Fatalf("admin sees %d hearings, want 0", day.Counts.Hearings)
		}
		// The administrator can still administer.
		var users struct {
			Items []struct{ Email string } `json:"items"`
		}
		if st := w.admin.do("GET", "/api/v1/users", nil, &users); st != 200 || len(users.Items) < 6 {
			t.Fatalf("admin users list = %d (%d users)", st, len(users.Items))
		}
		// And cannot add themself to a team (I-AC-3).
		if st, code := w.partner.errCode("POST", "/api/v1/matters/"+w.open.MatterID+"/members",
			member(w.e.users["admin@alpha.lk"], "supporting")); st != 422 {
			t.Fatalf("adding the admin to a team = %d %s, want 422", st, code)
		}
	})

	scenario("audit_records_cannot_be_modified", func(t *testing.T) {
		ctx := context.Background()
		for _, q := range []string{`update audit_event set action = 'x.y'`, `delete from audit_event`} {
			tx, err := w.e.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = tx.Exec(ctx, `select set_config('app.firm_id', $1, true)`, w.e.firms["alpha"].String())
			if _, err := tx.Exec(ctx, q); err == nil {
				_ = tx.Rollback(ctx)
				t.Fatalf("the application role could run %q", q)
			}
			_ = tx.Rollback(ctx)
		}
		// Even the owner is stopped by the append-only trigger.
		if _, err := w.e.owner.Exec(ctx, `update audit_event set reason = 'tampered'`); err == nil {
			t.Fatal("the schema owner could update audit_event")
		}
		w.e.verifyChain("alpha")
	})

	for _, s := range authz.RequiredReadPathScenarios {
		if !covered[s.ID] {
			t.Errorf("required authorisation scenario %q has no test", s.ID)
		}
	}
}

func TestWallBlocksTeamAndAppearance(t *testing.T) {
	w := newWorld(t)
	if st, code := w.partner.errCode("POST", "/api/v1/matters/"+w.open.MatterID+"/members",
		member(w.e.users["walled@alpha.lk"], "supporting")); st != 409 || code != "wall.member_excluded" {
		t.Fatalf("re-adding a walled user = %d %s, want 409 wall.member_excluded", st, code)
	}
	if st, code := w.partner.errCode("POST", "/api/v1/hearings/"+w.open.HearingID+"/appearances",
		map[string]any{"user_id": w.e.users["walled@alpha.lk"].String()}); st != 409 || code != "wall.member_excluded" {
		t.Fatalf("walled appearance = %d %s, want 409", st, code)
	}
	// Removing a wall needs two different people.
	var wall struct {
		ID string `json:"id"`
	}
	w.partner.do("POST", "/api/v1/ethical-walls", map[string]any{"reason": "Second wall",
		"matter_ids": []string{w.restricted.MatterID}, "user_ids": []string{w.e.users["outsider@alpha.lk"].String()}}, &wall)
	if st := w.partner.do("POST", "/api/v1/ethical-walls/"+wall.ID+"/removal-request", map[string]any{"reason": "Resolved"}, nil); st != 204 {
		t.Fatalf("removal request = %d", st)
	}
	if st, code := w.partner.errCode("POST", "/api/v1/ethical-walls/"+wall.ID+"/removal-approval", nil); st != 409 ||
		code != "wall.second_approver_required" {
		t.Fatalf("self-approval = %d %s, want 409 wall.second_approver_required", st, code)
	}
}

func TestClerkDiaryScopeSeesMetadataOnly(t *testing.T) {
	w := newWorld(t)
	var h struct {
		Instructions *string `json:"instructions"`
		MatterTitle  struct {
			EN *string `json:"en"`
		} `json:"matter_title"`
		Access      string `json:"access"`
		TitleMasked bool   `json:"title_masked"`
	}
	if st := w.clerk.do("GET", "/api/v1/hearings/"+w.open.HearingID, nil, &h); st != 200 {
		t.Fatalf("clerk GET standard hearing = %d, want 200 (diary scope)", st)
	}
	if h.Access != "diary" || h.Instructions != nil {
		t.Fatalf("clerk sees access=%q instructions=%v; want diary access and no instructions", h.Access, h.Instructions)
	}
	h = struct {
		Instructions *string `json:"instructions"`
		MatterTitle  struct {
			EN *string `json:"en"`
		} `json:"matter_title"`
		Access      string `json:"access"`
		TitleMasked bool   `json:"title_masked"`
	}{}
	if st := w.clerk.do("GET", "/api/v1/hearings/"+w.restricted.HearingID, nil, &h); st != 200 || !h.TitleMasked || h.MatterTitle.EN != nil {
		t.Fatalf("clerk on restricted hearing: status %d masked=%v title=%v", st, h.TitleMasked, h.MatterTitle.EN)
	}
	if st, _ := w.clerk.errCode("GET", "/api/v1/hearings/"+w.highly.HearingID, nil); st != 404 {
		t.Fatalf("clerk on highly restricted hearing = %d, want 404", st)
	}
	// The clerk cannot write matter content.
	if st, _ := w.clerk.errCode("POST", "/api/v1/hearings/"+w.open.HearingID+"/notes", map[string]any{"body": "x"},
		"Idempotency-Key", idemKey()); st != 404 {
		t.Fatalf("clerk note = %d, want 404", st)
	}
}

func TestFirmsAreIsolated(t *testing.T) {
	w := newWorld(t)
	w.e.firm("beta")
	w.e.user("beta", "partner@beta.lk", "managing_partner")
	beta := w.e.login("beta", "partner@beta.lk")
	for _, path := range []string{"/api/v1/matters/" + w.open.MatterID, "/api/v1/hearings/" + w.open.HearingID} {
		if st, _ := beta.errCode("GET", path, nil); st != 404 {
			t.Fatalf("other firm GET %s = %d, want 404", path, st)
		}
	}
	// RLS is the backstop: with firm beta's context the database itself
	// returns none of alpha's rows, even without any application filter.
	ctx := context.Background()
	tx, err := w.e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, _ = tx.Exec(ctx, `select set_config('app.firm_id', $1, true)`, w.e.firms["beta"].String())
	var n int
	if err := tx.QueryRow(ctx, `select count(*) from matter`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("firm beta context sees %d matters (err %v), want 0", n, err)
	}
	// With no firm set, nothing at all is visible.
	_, _ = tx.Exec(ctx, `select set_config('app.firm_id', '', true)`)
	if err := tx.QueryRow(ctx, `select count(*) from hearing`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("no-firm context sees %d hearings, want 0", n)
	}
}

func TestSessionGates(t *testing.T) {
	w := newWorld(t)
	// CSRF: a write without the token is refused even with a valid cookie.
	saved := w.senior.csrf
	w.senior.csrf = ""
	if st, code := w.senior.errCode("POST", "/api/v1/hearings/"+w.open.HearingID+"/notes", map[string]any{"body": "x"},
		"Idempotency-Key", idemKey()); st != 403 || code != "auth.csrf_invalid" {
		t.Fatalf("write without CSRF = %d %s, want 403 auth.csrf_invalid", st, code)
	}
	w.senior.csrf = saved
	// Unauthenticated.
	anon := &client{e: w.e, http: w.e.server.Client()}
	if st, code := anon.errCode("GET", "/api/v1/matters", nil); st != 401 || code != "auth.session_required" {
		t.Fatalf("anonymous = %d %s", st, code)
	}
	// Wrong password and unknown account look the same.
	_, c1 := anon.errCode("POST", "/api/v1/auth/login", map[string]any{"email": "senior@alpha.lk", "password": "wrong-password-x"})
	_, c2 := anon.errCode("POST", "/api/v1/auth/login", map[string]any{"email": "nobody@alpha.lk", "password": "wrong-password-x"})
	if c1 != "auth.invalid_credentials" || c1 != c2 {
		t.Fatalf("login errors differ: %q vs %q", c1, c2)
	}
	// Logout kills the session server-side.
	if st := w.outsider.do("POST", "/api/v1/auth/logout", nil, nil); st != 204 {
		t.Fatalf("logout = %d", st)
	}
	if st, _ := w.outsider.errCode("GET", "/api/v1/me", nil); st != 401 {
		t.Fatalf("after logout = %d, want 401", st)
	}
	// Disabling a user ends their session at once.
	if st := w.admin.do("POST", "/api/v1/admin/users/"+w.e.users["walled@alpha.lk"].String()+"/disable", nil, nil); st != 204 {
		t.Fatalf("disable = %d", st)
	}
	if st, _ := w.walled.errCode("GET", "/api/v1/me", nil); st != 401 {
		t.Fatalf("disabled user's session = %d, want 401", st)
	}
}

func containsMatter(ps []struct {
	MatterID string `json:"matter_id"`
}, id string) bool {
	for _, p := range ps {
		if p.MatterID == id {
			return true
		}
	}
	return false
}
