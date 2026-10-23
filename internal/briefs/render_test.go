package briefs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/hearings"
)

// Spike #26 §2 shaping stress strings, built with explicit ZWJ per SLS 1134.
var stress = []string{
	"වාක්\u200Dයය",              // yansaya
	"ප්\u200Dරජාතන්ත්\u200Dරික", // rakaransaya
	"ධර්\u200Dමය",               // repaya
	"ක\u200D්ව",                 // touching letters
	"කෝ කෞ කෘ කෲ රු රූ ළු",      // vowel signs
	"ශ්\u200Dරී ලංකා",           // ශ්\u200Dරී ligature
	"මහේස්ත්\u200Dරාත්",         // stacked
}

func s(v string) *string { return &v }

func party(en, si string) hearings.PartyName {
	n := name(en, si)
	return hearings.PartyName{EN: n.EN, SI: n.SI}
}

func name(en, si string) hearings.Name {
	n := hearings.Name{}
	if en != "" {
		n.EN = s(en)
	}
	if si != "" {
		n.SI = s(si)
	}
	return n
}

// syntheticBrief is two days: a 40-hearing day across six courts for the
// pagination check, and a day with nothing listed.
func syntheticBrief() *Brief {
	courtsList := []hearings.Ref{
		{ID: uuid.New(), Code: "DC-COL", Name: name("District Court of Colombo", "කොළඹ දිසා අධිකරණය")},
		{ID: uuid.New(), Code: "MC-GAM", Name: name("Magistrate's Court, Gampaha", "ගම්පහ මහේස්ත්\u200Dරාත් අධිකරණය")},
		{ID: uuid.New(), Code: "HC-KAN", Name: name("High Court of Kandy", "මහනුවර මහාධිකරණය")},
		{ID: uuid.New(), Code: "PC-NEG", Name: name("Primary Court, Negombo", "")},
		{ID: uuid.New(), Code: "CA", Name: name("Court of Appeal", "අභියාචනාධිකරණය")},
		{ID: uuid.New(), Code: "SC", Name: name("", "ශ්\u200Dරී ලංකා ශ්\u200Dරේෂ්ඨාධිකරණය")},
	}
	states := []string{hearings.Verified, hearings.Unverified, hearings.Conflict}
	preps := []string{"not_started", "in_progress", "ready", "blocked"}
	day := hearings.DayView{Date: "2026-10-21"}
	for ci, c := range courtsList {
		cd := hearings.CourtDay{Court: c}
		for i := range 7 {
			n := ci*7 + i
			if n >= 40 {
				break
			}
			card := hearings.Card{
				ID: uuid.New(), Date: day.Date, Session: []string{"morning", "afternoon", "unspecified"}[n%3],
				Court: c, MatterNumber: fmt.Sprintf("M-2026-%04d", n+1),
				CaseNumber:        s(fmt.Sprintf("DR/%d/2026", 1000+n)),
				VerificationState: states[n%3], PreparationStatus: preps[n%4], ListingStatus: hearings.StatusScheduled,
				Purpose:    &hearings.Ref{Code: "trial", Name: name("Trial", "විභාගය")},
				OurClients: []hearings.PartyName{party("Perera Holdings (Pvt) Ltd", "පෙරේරා හෝල්ඩිංග්ස් (පුද්) සමාගම")},
				Opposing:   []hearings.PartyName{party("", stress[n%len(stress)]), party("K. Silva", "")},
				Appearing:  []hearings.Person{{UserID: uuid.New(), Name: "A. Fernando"}, {UserID: uuid.New(), Name: "නිමල් ජයසූරිය"}},
				Instructions: s("Mixed line: client says " + stress[n%len(stress)] + " — bring the deed (ඔප්පුව) and " +
					"<script>alert(1)</script> copies."),
				LastOutcome: &hearings.LastOutcome{Date: "2026-09-30", OutcomeCode: "postponed",
					OutcomeName: name("Postponed", "කල් තැබිණි"), Summary: s("Re-fixed for trial; " + stress[(n+1)%len(stress)]),
					OrderText: s("Defendant to file answer / විත්තිකරු උත්තරය ගොනු කළ යුතුය")},
				Access: "full", VerificationOverdue: n%5 == 0, Clash: n%11 == 0,
			}
			if n%3 == 0 {
				t := fmt.Sprintf("%02d:%02d", 9+n%5, (n*7)%60)
				card.Time = &t
				card.Courtroom = s("3")
			}
			switch n % 9 {
			case 4: // masked (restricted) matter, as hearings.scanCards leaves it
				card.TitleMasked = true
				card.OurClients, card.Opposing = []hearings.PartyName{}, []hearings.PartyName{}
				card.Instructions, card.LastOutcome.Summary, card.LastOutcome.OrderText = nil, nil, nil
				card.Access = "diary"
			case 7: // diary access: no instructions or order text
				card.Instructions, card.LastOutcome.Summary, card.LastOutcome.OrderText = nil, nil, nil
				card.Access = "diary"
			}
			cd.Hearings = append(cd.Hearings, card)
			day.Counts.Hearings++
		}
		day.Courts = append(day.Courts, cd)
	}
	return &Brief{From: "2026-10-21", To: "2026-10-22", Scope: hearings.ScopeFirm,
		Days: []hearings.DayView{day, {Date: "2026-10-22", Courts: []hearings.CourtDay{}}}}
}

func testDoc(lang string) Document {
	return Document{Brief: syntheticBrief(), Lang: lang, PreparedFor: "නිමල් ජයසූරිය",
		GeneratedAt: time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)}
}

func TestHTMLIsSelfContainedAndEscaped(t *testing.T) {
	p := NewPrinter(config.PDF{}, nil)
	for _, lang := range []string{LangEN, LangSI, LangBoth} {
		html, err := p.HTML(testDoc(lang))
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		lower := strings.ToLower(html)
		for _, bad := range []string{"<script", "url(http", "url(//", `src="http`, `href="http`, "@import"} {
			if strings.Contains(lower, bad) {
				t.Errorf("%s: HTML contains %q", lang, bad)
			}
		}
		if n := strings.Count(html, "@font-face"); n != 4 {
			t.Errorf("%s: %d @font-face rules, want 4", lang, n)
		}
		// Every label key resolved (an unresolved key prints as itself).
		for _, prefix := range []string{"session.", "prep.", "status.", "scope.", "col."} {
			if strings.Contains(html, ">"+prefix) || strings.Contains(html, ": "+prefix) {
				t.Errorf("%s: unresolved label with prefix %q", lang, prefix)
			}
		}
		if !strings.Contains(html, "2026-10-20 08:30 (Asia/Colombo)") {
			t.Errorf("%s: generated-at not shown in Asia/Colombo", lang)
		}
		if got := strings.Count(html, `<tr class="court">`); got != 6 {
			t.Errorf("%s: %d court groups, want 6", lang, got)
		}
	}
	html, _ := p.HTML(testDoc(LangEN))
	for _, want := range []string{"Restricted matter", "Verified", "Not verified", "Dates disagree",
		"Confidential — generated for නිමල් ජයසූරිය at", "No hearings listed.", "Wednesday 21 October 2026"} {
		if !strings.Contains(html, want) {
			t.Errorf("en: missing %q", want)
		}
	}
	html, _ = p.HTML(testDoc(LangBoth))
	if !strings.Contains(html, "දෛනික නඩු සාරාංශය / Daily court brief") {
		t.Error("both: title is not Sinhala / English")
	}
	if !strings.Contains(html, stress[1]) {
		t.Error("both: stress string missing")
	}
}

func TestRenderPDF(t *testing.T) {
	p := NewPrinter(config.PDF{ChromePath: os.Getenv("NITHIDESK_CHROME_PATH"), RenderTimeout: 90 * time.Second}, nil)
	if !p.pdf.Available() {
		t.Skip("no Chromium-family browser found; set NITHIDESK_CHROME_PATH to run")
	}
	for _, lang := range []string{LangBoth, LangSI} {
		started := time.Now()
		pdf, err := p.PDF(context.Background(), testDoc(lang))
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
			t.Fatalf("%s: output is not a PDF", lang)
		}
		if len(pdf) < 30*1024 {
			t.Fatalf("%s: PDF is only %d bytes", lang, len(pdf))
		}
		// Static faces embed as named TrueType subsets (spike #26 finding 2);
		// a missing name means a fallback font or a Type 3 drawing was used.
		for _, font := range []string{"NotoSansSinhala", "NotoSans-"} {
			if !bytes.Contains(pdf, []byte(font)) {
				t.Errorf("%s: PDF does not embed %s", lang, font)
			}
		}
		pages := bytes.Count(pdf, []byte("/Type /Page\n")) + bytes.Count(pdf, []byte("/Type /Page>>")) +
			bytes.Count(pdf, []byte("/Type /Page/"))
		t.Logf("%s: %d bytes, ~%d pages, %s", lang, len(pdf), pages, time.Since(started).Round(time.Millisecond))
		if dir := os.Getenv("NITHIDESK_BRIEF_OUT"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, "brief-"+lang+".pdf"), pdf, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAllowedURL(t *testing.T) {
	for u, want := range map[string]bool{
		"data:font/ttf;base64,AAAA": true, "about:blank": true,
		"https://example.com/x.png": false, "http://127.0.0.1/": false, "file:///etc/passwd": false,
		"chrome://settings": false, "ws://localhost": false,
	} {
		if got := allowedURL(u); got != want {
			t.Errorf("allowedURL(%q) = %v, want %v", u, got, want)
		}
	}
}
