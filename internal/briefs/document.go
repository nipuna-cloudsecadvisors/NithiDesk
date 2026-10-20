package briefs

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/assets"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/hearings"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/clock"
)

// Languages of a printed brief.
const (
	LangEN   = "en"
	LangSI   = "si"
	LangBoth = "both"
)

// Document is everything printed on one brief. It is built from a Brief
// value only, so the printer can be tested without a database.
type Document struct {
	Brief       *Brief
	Lang        string
	PreparedFor string    // the requester's display name, for the watermark
	GeneratedAt time.Time // printed in Asia/Colombo
}

// fontFiles are the four static faces, by CSS family and weight. Sinhala is
// listed first in the font stack but limited by unicode-range to the
// Sinhala block and the joiners, so every Sinhala cluster (including the
// ZWJ that forms yansaya, rakaransaya and repaya) is shaped by one font,
// and Latin text, digits and punctuation always come from Noto Sans.
var fontFiles = []struct {
	file, family, weight, unicodeRange string
}{
	{"NotoSansSinhala-Regular.ttf", "ND Sinhala", "400", "U+0D80-0DFF, U+200C-200D, U+25CC"},
	{"NotoSansSinhala-Bold.ttf", "ND Sinhala", "700", "U+0D80-0DFF, U+200C-200D, U+25CC"},
	{"NotoSans-Regular.ttf", "ND Latin", "400", ""},
	{"NotoSans-Bold.ttf", "ND Latin", "700", ""},
}

// fontCSS builds @font-face rules with the fonts inlined as data: URLs, so
// the renderer never has a reason to fetch anything (ADR 0007). dir
// overrides the fonts embedded in the binary.
func fontCSS(dir string) (template.CSS, error) {
	var fsys fs.FS
	if dir != "" {
		fsys = os.DirFS(dir)
	} else {
		sub, err := fs.Sub(assets.Fonts, "fonts")
		if err != nil {
			return "", fmt.Errorf("embedded fonts: %w", err)
		}
		fsys = sub
	}
	var b strings.Builder
	for _, f := range fontFiles {
		data, err := fs.ReadFile(fsys, f.file)
		if err != nil {
			return "", fmt.Errorf("read font %s: %w", f.file, err)
		}
		// A static TrueType face starts with version 1.0. Anything else (a
		// variable font collection, an HTML error page saved by a failed
		// download) must not reach the renderer.
		if len(data) < 1024 || !bytes.Equal(data[:4], []byte{0, 1, 0, 0}) {
			return "", fmt.Errorf("font %s is not a TrueType file", f.file)
		}
		fmt.Fprintf(&b, "@font-face{font-family:'%s';font-style:normal;font-weight:%s;src:url(data:font/ttf;base64,%s) format('truetype');",
			f.family, f.weight, base64.StdEncoding.EncodeToString(data))
		if f.unicodeRange != "" {
			fmt.Fprintf(&b, "unicode-range:%s;", f.unicodeRange)
		}
		b.WriteString("}\n")
	}
	// The value is built only from files we ship and constants above, never
	// from request data, which is what makes the template.CSS conversion safe.
	return template.CSS(b.String()), nil
}

//go:embed brief.html.tmpl
var briefTemplateText string

var briefTemplate = template.Must(template.New("brief").Parse(briefTemplateText))

// Printer turns a Document into HTML and PDF.
type Printer struct {
	fonts    template.CSS
	fontsErr error
	pdf      *PDFRenderer
}

// NewPrinter loads the fonts and resolves the browser. A font problem is
// kept and reported by every render rather than stopping the process, so
// the rest of the API keeps working.
func NewPrinter(cfg config.PDF, logger *slog.Logger) *Printer {
	fonts, err := fontCSS(cfg.FontsDir)
	return &Printer{fonts: fonts, fontsErr: err, pdf: NewPDFRenderer(cfg, logger)}
}

// ErrFonts wraps a font loading failure.
var ErrFonts = errors.New("briefs: fonts unavailable")

// Available reports whether PDFs can be produced at all.
func (p *Printer) Available() bool { return p.fontsErr == nil && p.pdf.Available() }

// HTML renders the document as one self-contained HTML page.
func (p *Printer) HTML(doc Document) (string, error) {
	if p.fontsErr != nil {
		return "", errors.Join(ErrFonts, p.fontsErr)
	}
	view := buildView(doc)
	view.FontCSS = p.fonts
	var b bytes.Buffer
	if err := briefTemplate.Execute(&b, view); err != nil {
		return "", fmt.Errorf("brief template: %w", err)
	}
	return b.String(), nil
}

// PDF renders the document to PDF.
func (p *Printer) PDF(ctx context.Context, doc Document) ([]byte, error) {
	html, err := p.HTML(doc)
	if err != nil {
		return nil, err
	}
	return p.pdf.Print(ctx, html)
}

// ---------------------------------------------------------------------------
// View model: every string already resolved for the language, so the
// template holds layout only.
// ---------------------------------------------------------------------------

type pageView struct {
	Lang        string
	FontCSS     template.CSS
	Title       string
	Range       string
	Scope       string
	Generated   string
	PreparedFor string
	Watermark   string
	Cols        []string
	Days        []dayView
}

type dayView struct {
	Heading string
	Summary string
	Empty   string
	Courts  []courtView
}

type courtView struct {
	Name  string
	Count string
	Rows  []rowView
}

type rowView struct {
	Time         string
	Session      string
	Courtroom    string
	CaseNumber   string
	MatterNumber string
	Parties      string
	Restricted   bool
	Purpose      string
	VerifyClass  string // ok, unv, conf: picks the symbol; the text says the same
	Verify       string
	Overdue      string
	Status       string
	Prep         string
	Appearing    string
	Clash        string
	LastDate     string
	LastOutcome  string
	LastSummary  string
	LastOrder    string
	OrderLabel   string
	Instructions string
}

const dash = "—"

func buildView(doc Document) pageView {
	lang := doc.Lang
	if lang != LangEN && lang != LangSI {
		lang = LangBoth
	}
	L := func(key string) string { return label(key, lang) }
	b := doc.Brief
	if b == nil {
		b = &Brief{}
	}
	at := doc.GeneratedAt.In(clock.Colombo).Format("2006-01-02 15:04") + " (Asia/Colombo)"
	v := pageView{
		Lang:        map[string]string{LangEN: "en", LangSI: "si", LangBoth: "si"}[lang],
		Title:       L("title"),
		Range:       L("dates") + ": " + dateRange(b.From, b.To, lang),
		Scope:       L("scope") + ": " + L("scope."+b.Scope),
		Generated:   L("generated") + ": " + at,
		PreparedFor: L("prepared_for") + ": " + doc.PreparedFor,
		Watermark:   watermark(doc.PreparedFor, at, lang),
	}
	for _, c := range []string{"col.time", "col.case", "col.parties", "col.purpose", "col.verify", "col.prep",
		"col.appearing", "col.last", "col.instructions"} {
		v.Cols = append(v.Cols, L(c))
	}
	for _, d := range b.Days {
		dv := dayView{
			Heading: dayHeading(d.Date, lang),
			Summary: fmt.Sprintf("%s: %d · %s: %d · %s: %d · %s: %d", L("count.hearings"), d.Counts.Hearings,
				L("unverified"), d.Counts.Unverified, L("conflict"), d.Counts.Conflicts, L("clash"), d.Counts.Clashes),
		}
		if len(d.Courts) == 0 {
			dv.Empty = L("no_hearings")
		}
		for _, c := range d.Courts {
			cv := courtView{Name: pick(c.Court.Name, lang), Count: fmt.Sprintf("%d", len(c.Hearings))}
			if cv.Name == "" {
				cv.Name = c.Court.Code
			}
			for _, h := range c.Hearings {
				cv.Rows = append(cv.Rows, buildRow(h, lang))
			}
			dv.Courts = append(dv.Courts, cv)
		}
		v.Days = append(v.Days, dv)
	}
	return v
}

func buildRow(h hearings.Card, lang string) rowView {
	L := func(key string) string { return label(key, lang) }
	r := rowView{
		Time:         deref(h.Time),
		Session:      L("session." + h.Session),
		CaseNumber:   orDash(deref(h.CaseNumber)),
		MatterNumber: h.MatterNumber,
		Prep:         L("prep." + h.PreparationStatus),
		OrderLabel:   L("order"),
		Instructions: deref(h.Instructions),
	}
	if h.Courtroom != nil && *h.Courtroom != "" {
		r.Courtroom = L("courtroom") + " " + *h.Courtroom
	}
	// Parties. A masked title means the matter is restricted for this
	// reader: the card already carries no names, and the brief says why.
	switch {
	case h.TitleMasked:
		r.Restricted = true
		r.Parties = L("restricted")
	case len(h.OurClients) > 0 || len(h.Opposing) > 0:
		r.Parties = joinNames(h.OurClients, lang) + " " + L("versus") + " " + joinNames(h.Opposing, lang)
	default:
		r.Parties = orDash(pick(h.ProceedingTitle, lang))
	}
	if h.Purpose != nil {
		r.Purpose = pick(h.Purpose.Name, lang)
	}
	if h.PurposeNote != nil && *h.PurposeNote != "" {
		r.Purpose = strings.TrimSpace(r.Purpose + " " + *h.PurposeNote)
	}
	r.Purpose = orDash(r.Purpose)
	switch h.VerificationState {
	case hearings.Verified:
		r.VerifyClass, r.Verify = "ok", L("verified")
	case hearings.Conflict:
		r.VerifyClass, r.Verify = "conf", L("conflict")
	default:
		r.VerifyClass, r.Verify = "unv", L("unverified")
	}
	if h.VerificationOverdue {
		r.Overdue = L("overdue")
	}
	if h.ListingStatus != "" && h.ListingStatus != hearings.StatusScheduled {
		r.Status = L("status." + h.ListingStatus)
	}
	names := make([]string, 0, len(h.Appearing))
	for _, a := range h.Appearing {
		names = append(names, a.Name)
	}
	r.Appearing = orDash(strings.Join(names, ", "))
	if h.Clash {
		r.Clash = L("clash")
	}
	if lo := h.LastOutcome; lo != nil {
		r.LastDate = lo.Date
		r.LastOutcome = pick(lo.OutcomeName, lang)
		if r.LastOutcome == "" {
			r.LastOutcome = lo.OutcomeCode
		}
		r.LastSummary = deref(lo.Summary)
		r.LastOrder = deref(lo.OrderText)
	}
	return r
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func orDash(s string) string {
	if s == "" {
		return dash
	}
	return s
}

// pick chooses the name for the language, falling back to the other one;
// "both" shows Sinhala and English when they differ.
func pick(n hearings.Name, lang string) string {
	en, si := deref(n.EN), deref(n.SI)
	switch lang {
	case LangEN:
		if en != "" {
			return en
		}
		return si
	case LangSI:
		if si != "" {
			return si
		}
		return en
	default:
		switch {
		case en == "":
			return si
		case si == "" || si == en:
			return en
		default:
			return si + " / " + en
		}
	}
}

func joinNames(ns []hearings.Name, lang string) string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		if s := pick(n, lang); s != "" {
			out = append(out, s)
		}
	}
	return orDash(strings.Join(out, ", "))
}

func watermark(name, at, lang string) string {
	en := fmt.Sprintf("Confidential — generated for %s at %s", name, at)
	si := fmt.Sprintf("රහස්\u200Dයයි — %s සඳහා %s ට සකස් කරන ලදී", name, at)
	switch lang {
	case LangEN:
		return en
	case LangSI:
		return si
	default:
		return si + " / " + en
	}
}

func dateRange(from, to, lang string) string {
	if from == "" {
		return dash
	}
	if to == "" || to == from {
		return dayHeading(from, lang)
	}
	return dayHeading(from, lang) + " – " + dayHeading(to, lang)
}

var (
	siMonths = [...]string{"ජනවාරි", "පෙබරවාරි", "මාර්තු", "අප්\u200Dරේල්", "මැයි", "ජූනි", "ජූලි", "අගෝස්තු",
		"සැප්තැම්බර්", "ඔක්තෝබර්", "නොවැම්බර්", "දෙසැම්බර්"}
	siWeekdays = [...]string{"ඉරිදා", "සඳුදා", "අඟහරුවාදා", "බදාදා", "බ්\u200Dරහස්පතින්දා", "සිකුරාදා", "සෙනසුරාදා"}
)

// dayHeading formats a court date: "Wednesday 14 October 2026" or
// "2026 ඔක්තෝබර් 14, බදාදා".
func dayHeading(date, lang string) string {
	d, err := clock.ParseDate(date)
	if err != nil {
		return date
	}
	en := d.Format("Monday 2 January 2006")
	si := fmt.Sprintf("%d %s %d, %s", d.Year(), siMonths[d.Month()-1], d.Day(), siWeekdays[d.Weekday()])
	switch lang {
	case LangEN:
		return en
	case LangSI:
		return si
	default:
		return si + " / " + en
	}
}

// labels are the brief's printed words, English then Sinhala. The PDF is a
// server-side output, so it cannot use the frontend's message catalogue; the
// keys mirror its naming so the two can be merged when the catalogue gains a
// server-side reader.
var labels = map[string][2]string{
	"title":                    {"Daily court brief", "දෛනික නඩු සාරාංශය"},
	"dates":                    {"Dates", "දින"},
	"scope":                    {"Covers", "ආවරණය"},
	"scope.mine":               {"Hearings I appear in", "මා පෙනී සිටින නඩු දින"},
	"scope.team":               {"Hearings of my matters", "මගේ ගොනුවල නඩු දින"},
	"scope.firm":               {"All firm hearings", "ආයතනයේ සියලු නඩු දින"},
	"generated":                {"Generated at", "සකස් කළ වේලාව"},
	"prepared_for":             {"Prepared for", "ලබන්නා"},
	"count.hearings":           {"Hearings", "නඩු දින"},
	"no_hearings":              {"No hearings listed.", "නඩු දින ලැයිස්තුගත කර නැත."},
	"col.time":                 {"Time / session", "වේලාව / සැසිය"},
	"col.case":                 {"Case no. / matter no.", "නඩු අංකය / ගොනු අංකය"},
	"col.parties":              {"Parties", "පාර්ශ්වකරුවන්"},
	"col.purpose":              {"Purpose", "කාරණය"},
	"col.verify":               {"Date check", "දිනය තහවුරු කිරීම"},
	"col.prep":                 {"Preparation", "සූදානම"},
	"col.appearing":            {"Appearing", "පෙනී සිටින්නේ"},
	"col.last":                 {"Last time", "පසුගිය වාරය"},
	"col.instructions":         {"Instructions", "උපදෙස්"},
	"courtroom":                {"Courtroom", "අධිකරණ ශාලාව"},
	"order":                    {"Order", "නියෝගය"},
	"versus":                   {"v.", "එදිරිව"},
	"restricted":               {"Restricted matter", "සීමාසහිත ගොනුව"},
	"verified":                 {"Verified", "තහවුරු කර ඇත"},
	"unverified":               {"Not verified", "තහවුරු කර නැත"},
	"conflict":                 {"Dates disagree", "දින නොගැළපේ"},
	"overdue":                  {"verification overdue", "තහවුරු කිරීම ප්\u200Dරමාදයි"},
	"clash":                    {"Lawyer clash", "නීතිඥ ගැටුම"},
	"session.morning":          {"Morning", "පෙරවරු"},
	"session.afternoon":        {"Afternoon", "පස්වරු"},
	"session.unspecified":      {"Session not set", "සැසිය නොදනී"},
	"prep.not_started":         {"Not started", "ආරම්භ කර නැත"},
	"prep.in_progress":         {"In progress", "කෙරෙමින් පවතී"},
	"prep.ready":               {"Ready", "සූදානම්"},
	"prep.blocked":             {"Blocked", "අවහිරයි"},
	"status.held":              {"Held", "පැවැත්විණි"},
	"status.not_reached":       {"Not reached", "කැඳවූයේ නැත"},
	"status.court_not_sitting": {"Court not sitting", "අධිකරණය රැස් නොවීය"},
	"status.outcome_recorded":  {"Outcome recorded", "ප්\u200Dරතිඵලය සටහන් කර ඇත"},
}

// label returns the word for key; "both" prints "Sinhala / English". An
// unknown key prints as itself, which a test catches before a user does.
func label(key, lang string) string {
	l, ok := labels[key]
	if !ok {
		return key
	}
	switch lang {
	case LangEN:
		return l[0]
	case LangSI:
		return l[1]
	default:
		return l[1] + " / " + l[0]
	}
}
