package textnorm_test

import (
	"testing"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/textnorm"
)

// The labelled pairs from spike #25 (fabricated combinations of common Sri
// Lankan names; no real clients). They are a regression suite: a rule change
// that loses a match the spike proved must fail here.
type pair struct {
	category string
	a, b     string
	same     bool
}

const (
	zwj = "\u200D"
	al  = "්"
)

var pairs = []pair{
	{"cross-script", "නිමල් පෙරේරා", "Nimal Perera", true},
	{"cross-script", "සුනිල් ප්\u200Dරනාන්දු", "Sunil Fernando", true},
	{"cross-script", "කමල් ද සිල්වා", "Kamal de Silva", true},
	{"cross-script", "චමරි ජයවර්ධන", "Chamari Jayawardena", true},
	{"cross-script", "රුවන් වික්\u200Dරමසිංහ", "Ruwan Wickramasinghe", true},
	{"cross-script", "දිලානි ගුණසේකර", "Dilani Gunasekera", true},
	{"cross-script", "සඳුන් විජේසිංහ", "Sandun Wijesinghe", true},
	{"cross-script", "තිලිණි දිසානායක", "Thilini Dissanayake", true},
	{"cross-script", "කසුන් බණ්ඩාර", "Kasun Bandara", true},
	{"cross-script", "අනුෂා රත්නායක", "Anusha Rathnayake", true},
	{"cross-script", "ප්\u200Dරියංකා ජයසිංහ", "Priyanka Jayasinghe", true},
	{"cross-script", "මහින්ද හේරත්", "Mahinda Herath", true},
	{"cross-script", "නාමල් රාජපක්ෂ", "Namal Rajapaksa", true},
	{"cross-script", "ලසන්ත සේනානායක", "Lasantha Senanayake", true},
	{"cross-script", "චාමික වීරසිංහ", "Chamika Weerasinghe", true},
	{"cross-script", "සමන් අබේසේකර", "Saman Abeysekera", true},
	{"cross-script", "නුවන් සමරවීර", "Nuwan Samaraweera", true},
	{"cross-script", "ඉරේෂා ලියනගේ", "Iresha Liyanage", true},
	{"cross-script", "අජිත් කරුණාරත්න", "Ajith Karunaratne", true},
	{"cross-script", "ශාන්ති ජයතිලක", "Shanthi Jayathilaka", true},
	{"cross-script", "රංජිත් සෙනෙවිරත්න", "Ranjith Seneviratne", true},
	{"cross-script", "උපුල් ධර්මසේන", "Upul Dharmasena", true},
	{"cross-script", "ගයාන් මෙන්ඩිස්", "Gayan Mendis", true},
	{"cross-script", "ඩබ්ලිව්. ඒ. පෙරේරා", "W.A. Perera", true},
	{"cross-script", "කේ. පී. ගමගේ", "K.P. Gamage", true},

	{"spelling", "Ruwan Wickramasinghe", "Ruwan Wickremesinghe", true},
	{"spelling", "Ruwan Wickramasinghe", "Ruwan Wikramasinghe", true},
	{"spelling", "Chamari Jayawardena", "Chamari Jayawardene", true},
	{"spelling", "Chamari Jayawardena", "Chamari Jayewardene", true},
	{"spelling", "Nimal Perera", "Nimal Pereira", true},
	{"spelling", "Dilani Gunasekara", "Dilani Goonesekera", true},
	{"spelling", "Anusha Ratnayake", "Anusha Rathnayaka", true},
	{"spelling", "Namal Rajapaksa", "Namal Rajapakshe", true},
	{"spelling", "Saman Abeysekera", "Saman Abeyesekara", true},
	{"spelling", "Chamika Weerasinghe", "Chamika Virasinghe", true},
	{"spelling", "Shanthi Jayatilaka", "Shanthi Jayatilleke", true},
	{"spelling", "Thilini Dissanayake", "Thilini Disanayaka", true},
	{"spelling", "Ajith Karunaratne", "Ajith Karunaratna", true},
	{"spelling", "Sandun Wijesinghe", "Sandun Wijesinha", true},
	{"spelling", "Lasantha Senanayake", "Lasanta Senanayaka", true},

	{"initials", "W.A. Perera", "Perera W.A.", true},
	{"initials", "W.A. Perera", "Perera, W. A.", true},
	{"initials", "W A Perera", "Perera WA", true},
	{"initials", "K.P. Gamage", "Gamage K P", true},
	{"initials", "R.M. Bandara", "Bandara R.M.", true},
	{"initials", "Nimal Perera", "Perera Nimal", true},
	{"initials", "W.A. Perera", "Wanigasekara Arachchige Perera", true},
	{"initials", "H.M. Dissanayake", "Herath Mudiyanselage Dissanayake", true},

	{"typing", "ප්" + zwj + "රනාන්දු", "ප" + zwj + al + "රනාන්දු", true},
	{"typing", "ප්" + zwj + "රනාන්දු", "ප්රනාන්දු", true},
	{"typing", "පෙරේරා", "ෙපරේරා", true},
	{"typing", "ගමගේ", "ගමගේ", true},
	{"typing", "ජයතිලක", "ජය\u200Bතිලක", true},
	{"typing", "ආරියරත්න", "අාරියරත්න", true},
	{"typing", "රෝහණ", "රෝහණ", true},

	{"non-match", "Nimal Perera", "Sunil Perera", false},
	{"non-match", "Nimal Perera", "Nimal Fernando", false},
	{"non-match", "Kasun Jayasinghe", "Kasun Jayasekara", false},
	{"non-match", "Sandun Wijesinghe", "Sandun Wijeratne", false},
	{"non-match", "Anusha Kumara", "Anusha Kumari", false},
	{"non-match", "Kasun Bandara", "Kasun Bandaranayake", false},
	{"non-match", "Dilani Gunasekara", "Dilani Gunawardena", false},
	{"non-match", "Thilini Dissanayake", "Thilini Senanayake", false},
	{"non-match", "Gayan Ranasinghe", "Gayan Munasinghe", false},
	{"non-match", "Ruwan Wickramasinghe", "Ruwan Wickramaratne", false},
	{"non-match", "W.A. Perera", "K.P. Perera", false},
	{"non-match", "R.M. Bandara", "R.M. Bandaranayake", false},
	{"non-match", "Saman Kumara", "Kamal Kumara", false},
	{"non-match", "Chamari Jayawardena", "Chamari Jayasundara", false},
	{"non-match", "Nuwan Samaraweera", "Nuwan Samarakoon", false},
	{"non-match", "Mahinda Herath", "Mahinda Hettiarachchi", false},
	{"non-match", "නිමල් පෙරේරා", "Sunil Perera", false},
	{"non-match", "කසුන් ජයසිංහ", "Kasun Jayasekara", false},
	{"non-match", "ගයාන් රණසිංහ", "Gayan Munasinghe", false},
	{"non-match", "අජිත් කරුණාරත්න", "Ajith Karunanayake", false},
	{"non-match", "Upul Dharmasena", "Upul Dharmadasa", false},
	{"non-match", "Lasantha Silva", "Lasantha Silvapulle", false},
}

// The spike's measured result at threshold 0.6 (V5): every cross-script,
// initials and typing pair matched; 13/15 spelling pairs; 21/22 non-matches
// rejected. These are the floors; doing better is fine.
func TestNamePairsMeetSpikeResults(t *testing.T) {
	const threshold = 0.6
	floor := map[string]int{"cross-script": 25, "spelling": 13, "initials": 8, "typing": 7, "non-match": 21}
	got := map[string]int{}
	for _, p := range pairs {
		score := textnorm.ScoreNames(p.a, p.b)
		correct := (score >= threshold) == p.same
		if correct {
			got[p.category]++
		} else {
			t.Logf("%-12s %.2f  %q vs %q", p.category, score, p.a, p.b)
		}
	}
	for category, want := range floor {
		if got[category] < want {
			t.Errorf("%s: %d correct, want at least %d", category, got[category], want)
		}
	}
}

func TestTypingVariantsNormaliseIdentically(t *testing.T) {
	for _, p := range pairs {
		if p.category != "typing" {
			continue
		}
		a, b := textnorm.Normalise(p.a, false), textnorm.Normalise(p.b, false)
		if a != b {
			t.Errorf("Normalise(%q)=%q but Normalise(%q)=%q", p.a, a, p.b, b)
		}
	}
}

func TestNormaliseKeepsMeaningfulZWJ(t *testing.T) {
	in := "ප්" + zwj + "රනාන්දු" // rakaransaya: al-lakuna + ZWJ + consonant
	if got := textnorm.Normalise(in, true); got != in {
		t.Errorf("display form dropped a meaningful ZWJ: %q", got)
	}
	if got := textnorm.Normalise("ක"+zwj+"ම", true); got != "කම" {
		t.Errorf("a stray ZWJ survived: %q", got)
	}
}

func TestTransliterate(t *testing.T) {
	cases := map[string]string{
		"පෙරේරා":          "perera",
		"නිමල්":           "nimal",
		"ප්\u200Dරනාන්දු": "pranandu",
		"සිල්වා":          "silva",
	}
	for in, want := range cases {
		if got := textnorm.Transliterate(in); got != want {
			t.Errorf("Transliterate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPhoneticAbsorbsCommonVariation(t *testing.T) {
	same := [][2]string{{"Pereira", "Perera"}, {"Jayawardene", "Jayawardena"}}
	for _, s := range same {
		if a, b := textnorm.Phonetic(s[0]), textnorm.Phonetic(s[1]); a != b {
			t.Errorf("Phonetic(%q)=%q, Phonetic(%q)=%q, want equal", s[0], a, s[1], b)
		}
	}
	// Fernando / ප්\u200Dරනාන්දු match through the consonant skeleton (p-r-n-n-d),
	// not the plain phonetic key, so check the full name score.
	if got := textnorm.ScoreNames("Fernando", "ප්\u200Dරනාන්දු"); got < 0.6 {
		t.Errorf("ScoreNames(Fernando, ප්\u200Dරනාන්දු) = %.2f, want >= 0.6", got)
	}
}

func TestSimilarityMatchesPgTrgm(t *testing.T) {
	// pg_trgm: similarity('word','word') = 1, and disjoint words score 0.
	if got := textnorm.Similarity("perera", "perera"); got != 1 {
		t.Errorf("identical words scored %v", got)
	}
	if got := textnorm.Similarity("abc", "xyz"); got != 0 {
		t.Errorf("disjoint words scored %v", got)
	}
}

func TestNameShadowsCoverEveryVariant(t *testing.T) {
	s := textnorm.NameShadows("Nimal Perera", "නිමල් පෙරේරා")
	if s.Latin == "" || s.Key == "" {
		t.Fatalf("empty shadows: %+v", s)
	}
	if textnorm.Similarity(s.Key, textnorm.QueryKey("perera nimal")) < 0.4 {
		t.Errorf("query key does not resemble stored key: %q vs %q", s.Key, textnorm.QueryKey("perera nimal"))
	}
}

func TestLatinTextTransliteratesSinhalaTitles(t *testing.T) {
	if got := textnorm.LatinText("පෙරේරා එදිරිව සිල්වා"); got != "perera ediriva silva" {
		t.Errorf("LatinText(Sinhala title) = %q, want %q", got, "perera ediriva silva")
	}
	if got := textnorm.LatinText("Pérera v. Silva"); got != "perera v. silva" {
		t.Errorf("LatinText(%q) = %q", "Pérera v. Silva", got)
	}
}
