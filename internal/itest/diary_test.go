package itest

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type hearingView struct {
	ID                string `json:"id"`
	Date              string `json:"date"`
	ListingStatus     string `json:"listing_status"`
	VerificationState string `json:"verification_state"`
	NextHearingID     string `json:"next_hearing_id"`
	PreviousHearingID string `json:"previous_hearing_id"`
	Assertions        []struct {
		ID        string `json:"id"`
		Date      string `json:"date"`
		Authority string `json:"authority"`
		Kind      string `json:"kind"`
		Current   bool   `json:"current"`
	} `json:"date_assertions"`
}

type envelope struct {
	Data     hearingView `json:"data"`
	Warnings []struct {
		Code string `json:"code"`
	} `json:"warnings"`
}

func (c *client) hearing(id string) hearingView {
	c.e.t.Helper()
	var h hearingView
	if st := c.do("GET", "/api/v1/hearings/"+id, nil, &h); st != 200 {
		c.e.t.Fatalf("GET hearing = %d", st)
	}
	return h
}

func current(h hearingView) string {
	for _, a := range h.Assertions {
		if a.Current {
			return a.ID
		}
	}
	return ""
}

// The diary's core rules, end to end (doc 06 §3–4, logical model §8).
func TestHearingDateProvenanceAndVerification(t *testing.T) {
	w := newWorld(t)
	f := w.partner.matter("Diary matter", "standard",
		[]map[string]any{member(w.e.users["partner@alpha.lk"], "responsible_partner"),
			member(w.e.users["senior@alpha.lk"], "lead")}, dayStr(5))
	h := w.senior.hearing(f.HearingID)
	if h.VerificationState != "unverified" || h.Assertions[0].Authority != "primary" {
		t.Fatalf("a lawyer-entered date must start unverified/primary, got %s/%s", h.VerificationState, h.Assertions[0].Authority)
	}

	recordSrc := w.clerk.ref("date-source-types", "case_record")
	clientSrc := w.clerk.ref("date-source-types", "client")

	// A non-authoritative source cannot verify.
	if st, code := w.clerk.errCode("POST", "/api/v1/hearings/"+f.HearingID+"/verifications", map[string]any{
		"date_assertion_id": current(h), "source_type_id": clientSrc, "evidence_reference": "client said"}); st != 422 ||
		code != "verification.source_not_authoritative" {
		t.Fatalf("verify from client = %d %s", st, code)
	}
	// The clerk verifies from the case record.
	if st := w.clerk.do("POST", "/api/v1/hearings/"+f.HearingID+"/verifications", map[string]any{
		"date_assertion_id": current(h), "source_type_id": recordSrc, "evidence_reference": "JE 24"}, nil); st != 201 {
		t.Fatalf("clerk verify = %d", st)
	}
	if h = w.senior.hearing(f.HearingID); h.VerificationState != "verified" {
		t.Fatalf("after verification state = %s", h.VerificationState)
	}

	// Date edits through PATCH are refused.
	if st, code := w.senior.errCode("PATCH", "/api/v1/hearings/"+f.HearingID, map[string]any{"date": dayStr(6)},
		"If-Match", `"1"`); st != 422 || code != "hearing.date_change_requires_assertion" {
		t.Fatalf("PATCH date = %d %s", st, code)
	}

	// A contradicting claim never silently moves a verified date (I-HE-4).
	var env envelope
	if st := w.senior.do("POST", "/api/v1/hearings/"+f.HearingID+"/date-assertions", map[string]any{
		"date": dayStr(6), "session": "morning", "source_type_id": clientSrc, "source_detail": "Client phoned"}, &env,
		"Idempotency-Key", idemKey()); st != 201 {
		t.Fatalf("assert date = %d", st)
	}
	if env.Data.VerificationState != "conflict" || env.Data.Date != dayStr(6) {
		t.Fatalf("after contradicting claim: state %s date %s, want conflict / %s", env.Data.VerificationState, env.Data.Date, dayStr(6))
	}
	// Verifying a stale assertion is refused; verifying the current one resolves.
	if st, code := w.clerk.errCode("POST", "/api/v1/hearings/"+f.HearingID+"/verifications", map[string]any{
		"date_assertion_id": h.Assertions[0].ID, "source_type_id": recordSrc, "evidence_reference": "JE 24"}); st != 409 ||
		code != "verification.assertion_not_current" {
		t.Fatalf("verify stale = %d %s", st, code)
	}
	h = w.senior.hearing(f.HearingID)
	if st := w.clerk.do("POST", "/api/v1/hearings/"+f.HearingID+"/verifications", map[string]any{
		"date_assertion_id": current(h), "source_type_id": recordSrc, "evidence_reference": "JE 25"}, nil); st != 201 {
		t.Fatalf("verify new date = %d", st)
	}
	if h = w.senior.hearing(f.HearingID); h.VerificationState != "verified" || len(h.Assertions) != 2 {
		t.Fatalf("resolved: state %s assertions %d", h.VerificationState, len(h.Assertions))
	}
	if w.e.auditCount("alpha", "hearing.verified") != 2 || w.e.auditCount("alpha", "hearing.date_asserted") != 1 {
		t.Fatal("verification and assertion events missing from the audit log")
	}
	w.e.verifyChain("alpha")
}

func TestOutcomeCreatesNextHearingAtomicallyAndIdempotently(t *testing.T) {
	w := newWorld(t)
	f := w.open // listed today
	adjourned := w.senior.ref("outcome-types", "adjourned")
	body := map[string]any{"outcome_type_id": adjourned, "summary": "PW1 cross-examined", "order_text": "Tender P5-P9",
		"next_hearing": map[string]any{"date": dayStr(30), "session": "morning",
			"purpose_id": w.senior.ref("hearing-purposes", "further_trial")}}

	// Neither a next date nor a reason is refused.
	if st, code := w.senior.errCode("PUT", "/api/v1/hearings/"+f.HearingID+"/outcome",
		map[string]any{"outcome_type_id": adjourned}, "Idempotency-Key", idemKey()); st != 422 ||
		code != "hearing.outcome_requires_next_or_reason" {
		t.Fatalf("outcome without next or reason = %d %s", st, code)
	}
	key := idemKey()
	var first, replay envelope
	if st := w.senior.do("PUT", "/api/v1/hearings/"+f.HearingID+"/outcome", body, &first, "Idempotency-Key", key); st != 200 {
		t.Fatalf("record outcome = %d", st)
	}
	if first.Data.ListingStatus != "outcome_recorded" || first.Data.NextHearingID == "" {
		t.Fatalf("outcome result: %+v", first.Data)
	}
	// The offline queue retries: the same key returns the original result
	// and does not create a second next hearing.
	if st := w.senior.do("PUT", "/api/v1/hearings/"+f.HearingID+"/outcome", body, &replay, "Idempotency-Key", key); st != 200 ||
		replay.Data.NextHearingID != first.Data.NextHearingID {
		t.Fatalf("replay = %d next %s vs %s", st, replay.Data.NextHearingID, first.Data.NextHearingID)
	}
	// Same key, different body: refused.
	body["summary"] = "changed"
	if st, code := w.senior.errCode("PUT", "/api/v1/hearings/"+f.HearingID+"/outcome", body, "Idempotency-Key", key); st != 422 ||
		code != "idempotency.key_reused" {
		t.Fatalf("key reuse = %d %s", st, code)
	}
	// A second outcome with a new key is a conflict.
	if st, code := w.senior.errCode("PUT", "/api/v1/hearings/"+f.HearingID+"/outcome", body, "Idempotency-Key", idemKey()); st != 409 ||
		code != "hearing.outcome_already_recorded" {
		t.Fatalf("second outcome = %d %s", st, code)
	}

	next := w.senior.hearing(first.Data.NextHearingID)
	if next.Date != dayStr(30) || next.VerificationState != "unverified" || next.PreviousHearingID != f.HearingID ||
		next.Assertions[0].Kind != "from_outcome" || next.Assertions[0].Authority != "primary" {
		t.Fatalf("next hearing: %+v", next)
	}
	// An outcome for a future hearing is refused.
	if st, code := w.senior.errCode("PUT", "/api/v1/hearings/"+next.ID+"/outcome",
		map[string]any{"outcome_type_id": adjourned, "no_next_reason": "other"}, "Idempotency-Key", idemKey()); st != 409 ||
		code != "hearing.outcome_before_date" {
		t.Fatalf("future outcome = %d %s", st, code)
	}
}

func TestVacateAndListingStateMachine(t *testing.T) {
	w := newWorld(t)
	f := w.partner.matter("Vacate matter", "standard",
		[]map[string]any{member(w.e.users["partner@alpha.lk"], "responsible_partner")}, dayStr(2))
	var env envelope
	if st := w.partner.do("POST", "/api/v1/hearings/"+f.HearingID+"/vacate", map[string]any{
		"reason": "Judge on leave", "replacement": map[string]any{"date": dayStr(9), "session": "morning",
			"source_type_id": w.partner.ref("date-source-types", "registry_phone")}}, &env); st != 200 {
		t.Fatalf("vacate = %d", st)
	}
	if env.Data.ID == f.HearingID || env.Data.Date != dayStr(9) {
		t.Fatalf("vacate should return the replacement hearing: %+v", env.Data)
	}
	if old := w.partner.hearing(f.HearingID); old.ListingStatus != "vacated" {
		t.Fatalf("old hearing status %s", old.ListingStatus)
	}
	if st, code := w.partner.errCode("POST", "/api/v1/hearings/"+f.HearingID+"/listing-status",
		map[string]any{"status": "held"}); st != 409 || code != "hearing.illegal_transition" {
		t.Fatalf("vacated -> held = %d %s", st, code)
	}
	if st, _ := w.partner.errCode("POST", "/api/v1/hearings/"+env.Data.ID+"/listing-status",
		map[string]any{"status": "cancelled"}); st != 422 {
		t.Fatalf("cancel without reason = %d, want 422", st)
	}
}

func TestCourtListCheck(t *testing.T) {
	w := newWorld(t)
	var pr struct {
		CurrentCaseNumber string `json:"current_case_number"`
	}
	w.partner.do("GET", "/api/v1/proceedings/"+w.open.ProceedingID, nil, &pr)
	var res struct {
		Matched []struct {
			HearingID string `json:"hearing_id"`
			Verified  bool   `json:"verified"`
		} `json:"matched"`
		Missing    []struct{ ID string } `json:"missing"`
		Unexpected []string              `json:"unexpected"`
	}
	if st := w.clerk.do("POST", "/api/v1/diary/court-list-check", map[string]any{
		"court_id": w.clerk.courtID("DC-COLOMBO"), "list_date": todayStr(),
		// Spacing and leading zeros differ from how it was entered.
		"numbers":        []string{strings.ToLower(strings.ReplaceAll(pr.CurrentCaseNumber, "/", " / ")), "XYZ/999/26"},
		"verify_matches": true}, &res); st != 200 {
		t.Fatalf("court list check = %d", st)
	}
	if len(res.Matched) != 1 || !res.Matched[0].Verified || res.Matched[0].HearingID != w.open.HearingID {
		t.Fatalf("matched: %+v", res.Matched)
	}
	if len(res.Unexpected) != 1 || res.Unexpected[0] != "XYZ/999/26" {
		t.Fatalf("unexpected: %+v", res.Unexpected)
	}
	// Restricted and highly restricted hearings today at the same court are
	// "missing" for the clerk only if visible: the highly restricted one is not.
	for _, m := range res.Missing {
		if m.ID == w.highly.HearingID {
			t.Fatal("court-list check revealed a highly restricted hearing to the clerk")
		}
	}
	if h := w.senior.hearing(w.open.HearingID); h.VerificationState != "verified" {
		t.Fatalf("matched hearing not verified: %s", h.VerificationState)
	}
}

func TestDiaryImportDryRunCommitRollback(t *testing.T) {
	w := newWorld(t)
	csv := "\xEF\xBB\xBFmatter_title_en,client_name_en,client_name_si,responsible_partner_email,lead_email,court_code,proceeding_type,case_number,our_party_role,opposing_party_en,opposing_party_role,hearing_date,hearing_session,hearing_purpose,appearing_email\n" +
		"Imported Perera,Nimal Perera,නිමල් පෙරේරා,partner@alpha.lk,senior@alpha.lk,DC-COLOMBO,dc_land,L/700/24,plaintiff,Kamal Silva,defendant," + dayStr(10) + ",morning,trial,senior@alpha.lk\n" +
		"Imported Perera,Nimal Perera,නිමල් පෙරේරා,partner@alpha.lk,senior@alpha.lk,DC-COLOMBO,dc_land,L/700/24,plaintiff,Kamal Silva,defendant," + dayStr(40) + ",morning,further_trial,senior@alpha.lk\n" +
		"Bad row,Someone,,nobody@alpha.lk,,NO-SUCH-COURT,dc_land,X/1/24,,,," + "not-a-date" + ",,,\n"
	batch := w.partner.upload(csv)
	var b struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		ErrorCount int    `json:"error_count"`
		OKCount    int    `json:"ok_count"`
		Version    int    `json:"row_version"`
	}
	w.partner.do("POST", "/api/v1/imports/"+batch+"/validate", nil, &b)
	if b.Status != "validated" || b.ErrorCount != 1 || b.OKCount != 2 {
		t.Fatalf("dry run: %+v", b)
	}
	// Nothing was written to business tables by the dry run.
	var list struct {
		Items []struct{ ID string } `json:"items"`
	}
	w.partner.do("GET", "/api/v1/matters?q=Imported", nil, &list)
	if len(list.Items) != 0 {
		t.Fatal("dry run created matters")
	}
	// A batch with errors cannot be committed.
	if st, code := w.partner.errCode("POST", "/api/v1/imports/"+batch+"/commit", nil,
		"Idempotency-Key", idemKey(), "If-Match", jsonInt(b.Version)); st != 409 || code != "import.has_errors" {
		t.Fatalf("commit with errors = %d %s", st, code)
	}

	// Fix the file: drop the bad row.
	good := strings.Join(strings.Split(csv, "\n")[:3], "\n") + "\n"
	batch = w.partner.upload(good)
	w.partner.do("POST", "/api/v1/imports/"+batch+"/validate", nil, &b)
	if st := w.partner.do("POST", "/api/v1/imports/"+batch+"/commit", nil, &b, "Idempotency-Key", idemKey(),
		"If-Match", jsonInt(b.Version)); st != 200 || b.Status != "committed" {
		t.Fatalf("commit = %d %+v", st, b)
	}
	w.partner.do("GET", "/api/v1/matters?q=Imported", nil, &list)
	if len(list.Items) != 1 {
		t.Fatalf("two rows for one case should make one matter, got %d", len(list.Items))
	}
	// Imported dates are hearsay → unverified, in the clerk's queue.
	var queue struct {
		Items []struct {
			MatterID          string `json:"matter_id"`
			VerificationState string `json:"verification_state"`
		} `json:"items"`
	}
	w.clerk.do("GET", "/api/v1/queues/verification?within_days=60", nil, &queue)
	found := 0
	for _, it := range queue.Items {
		if it.MatterID == list.Items[0].ID && it.VerificationState == "unverified" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("imported hearings in verification queue: %d, want 2", found)
	}
	// Untouched, the batch rolls back.
	if st := w.partner.do("POST", "/api/v1/imports/"+batch+"/rollback", nil, &b); st != 200 || b.Status != "rolled_back" {
		t.Fatalf("rollback = %d %+v", st, b)
	}
	w.partner.do("GET", "/api/v1/matters?q=Imported", nil, &list)
	if len(list.Items) != 0 {
		t.Fatal("rollback left the matter visible")
	}
}

func (c *client) upload(content string) string {
	c.e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("kind", "diary_csv")
	fw, _ := mw.CreateFormFile("file", "diary.csv")
	_, _ = io.WriteString(fw, content)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", c.e.server.URL+"/api/v1/imports", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", c.csrf)
	resp, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 201 {
		c.e.t.Fatalf("upload = %d", resp.StatusCode)
	}
	return out.ID
}

func jsonInt(v int) string {
	b, _ := json.Marshal(v)
	return `"` + string(b) + `"`
}

func urlq(s string) string { return url.QueryEscape(s) }

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
