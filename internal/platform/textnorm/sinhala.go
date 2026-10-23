// Package textnorm normalises Sinhala and English text for storage and for
// matching, and maintains the Latin "shadow" forms that search indexes.
//
// It is the Go implementation of spike #25 (docs/spikes/25-sinhala-name-
// matching.md). PostgreSQL's pg_trgm produces no trigrams for raw Sinhala on
// a typical server locale, so the database only ever indexes the Latin
// shadows built here; raw Sinhala is stored for display.
//
// Rules are versioned by RulesVersion. Changing any rule must bump it, and a
// backfill job recomputes stored shadows.
package textnorm

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// RulesVersion identifies the normalisation and phonetic rule set that
// produced a stored shadow.
const RulesVersion = 1

const (
	alLakuna = '්' // virama
	zwj      = '\u200D'
	kombuva  = 'ෙ' // e-sign
	kombuDek = 'ෛ' // ai-sign
	yansa    = 'ය'
	rakar    = 'ර'
)

// invisible characters carry no meaning in a name and are removed.
var invisible = map[rune]bool{
	'\u200B': true, // zero-width space
	'\u200C': true, // zero-width non-joiner
	'\uFEFF': true, // byte-order mark
	'\u00AD': true, // soft hyphen
	'\u2060': true, // word joiner
	'\u180E': true, // Mongolian vowel separator
}

// compose maps legacy two-part sequences to their canonical code points.
var compose = []struct{ from, to string }{
	{"ෙෙ", "ෛ"}, // kombu + kombu -> kombu deka (ai)
	{"අා", "ආ"}, // a + aa-sign -> aa
	{"අැ", "ඇ"}, // a + ae-sign -> ae
	{"අෑ", "ඈ"}, // a + aae-sign -> aae
	{"උෟ", "ඌ"}, // u + gayanukitta -> uu
	{"එ්", "ඒ"}, // e + al-lakuna -> ee
	{"ඔ්", "ඕ"}, // o + al-lakuna -> oo
	{"ඔෟ", "ඖ"}, // o + gayanukitta -> au
}

func isConsonant(r rune) bool { return r >= 'ක' && r <= 'ෆ' }

// IsSinhala reports whether s contains any Sinhala character.
func IsSinhala(s string) bool {
	for _, r := range s {
		if r >= '඀' && r <= '෿' {
			return true
		}
	}
	return false
}

// Normalise returns the canonical form of s. With keepZWJ it is the storage
// and display form; without, the comparison form. It fixes the ways the same
// Sinhala name can be keyed differently (ZWJ order, visual-order kombuva,
// decomposed vowel signs, stray invisible characters) so they compare equal.
func Normalise(s string, keepZWJ bool) string {
	s = norm.NFC.String(s)
	s = strings.Map(func(r rune) rune {
		if invisible[r] {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")

	// Legacy order: ZWJ typed before al-lakuna.
	s = strings.ReplaceAll(s, string([]rune{zwj, alLakuna}), string([]rune{alLakuna, zwj}))
	s = reorderKombuva(s)
	for _, c := range compose {
		s = strings.ReplaceAll(s, c.from, c.to)
	}
	s = norm.NFC.String(s) // composes kombu + aa / al-lakuna into o / ee signs
	s = dropStrayZWJ(s)
	if !keepZWJ {
		s = strings.ReplaceAll(s, string(zwj), "")
	}
	return s
}

// reorderKombuva moves a kombuva typed in visual order (before its consonant,
// or before a yansaya/rakaransaya conjunct) to after the consonant, but only
// where it cannot belong to a preceding consonant.
func reorderKombuva(s string) string {
	in := []rune(s)
	out := make([]rune, 0, len(in))
	for i := 0; i < len(in); i++ {
		r := in[i]
		if (r == kombuva || r == kombuDek) && i+1 < len(in) && isConsonant(in[i+1]) &&
			(i == 0 || !(isConsonant(in[i-1]) || in[i-1] == alLakuna || in[i-1] == zwj)) {
			end := i + 2 // group = consonant
			if end < len(in) && in[end] == alLakuna {
				j := end + 1
				if j < len(in) && in[j] == zwj {
					j++
				}
				if j < len(in) && (in[j] == yansa || in[j] == rakar) {
					end = j + 1
				}
			}
			out = append(out, in[i+1:end]...)
			out = append(out, r)
			i = end - 1
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// dropStrayZWJ keeps a ZWJ only between al-lakuna and a consonant, where it
// forms yansaya, rakaransaya or touching letters.
func dropStrayZWJ(s string) string {
	in := []rune(s)
	out := make([]rune, 0, len(in))
	for i, r := range in {
		if r == zwj {
			prevOK := i > 0 && in[i-1] == alLakuna
			nextOK := i+1 < len(in) && isConsonant(in[i+1])
			if !prevOK || !nextOK {
				continue
			}
		}
		out = append(out, r)
	}
	return string(out)
}

var consonants = map[rune]string{
	'ක': "k", 'ඛ': "kh", 'ග': "g", 'ඝ': "gh", 'ඞ': "ng", 'ඟ': "ng", 'ච': "ch", 'ඡ': "chh", 'ජ': "j",
	'ඣ': "jh", 'ඤ': "ny", 'ඥ': "gny", 'ඦ': "nj", 'ට': "t", 'ඨ': "th", 'ඩ': "d", 'ඪ': "dh", 'ණ': "n",
	'ඬ': "nd", 'ත': "t", 'ථ': "th", 'ද': "d", 'ධ': "dh", 'න': "n", 'ඳ': "nd", 'ප': "p", 'ඵ': "ph",
	'බ': "b", 'භ': "bh", 'ම': "m", 'ඹ': "mb", 'ය': "y", 'ර': "r", 'ල': "l", 'ව': "v", 'ශ': "sh",
	'ෂ': "sh", 'ස': "s", 'හ': "h", 'ළ': "l", 'ෆ': "f",
}

// Vowel length is dropped on purpose: English spellings of Sinhala names do
// not mark it reliably.
var vowelSigns = map[rune]string{
	'ා': "a", 'ැ': "ae", 'ෑ': "ae", 'ි': "i", 'ී': "i", 'ු': "u", 'ූ': "u", 'ෘ': "ru", 'ෲ': "ru",
	'ෙ': "e", 'ේ': "e", 'ෛ': "ai", 'ො': "o", 'ෝ': "o", 'ෞ': "au", 'ෟ': "lu",
}

var vowels = map[rune]string{
	'අ': "a", 'ආ': "a", 'ඇ': "ae", 'ඈ': "ae", 'ඉ': "i", 'ඊ': "i", 'උ': "u", 'ඌ': "u", 'ඍ': "ru",
	'ඎ': "ru", 'ඏ': "lu", 'ඐ': "lu", 'එ': "e", 'ඒ': "e", 'ඓ': "ai", 'ඔ': "o", 'ඕ': "o", 'ඖ': "au",
}

var otherSigns = map[rune]string{'ං': "ng", 'ඃ': "h"}

// letterNames are Sinhala spellings of English initials ("ඩබ්ලිව්. ඒ." = W. A.).
var letterNames = map[string]string{
	"ඒ": "a", "බී": "b", "සී": "c", "ඩී": "d", "ඊ": "e", "එෆ්": "f", "ජී": "g", "එච්": "h", "අයි": "i",
	"ජේ": "j", "කේ": "k", "එල්": "l", "එම්": "m", "එන්": "n", "ඕ": "o", "පී": "p", "කිව්": "q",
	"ආර්": "r", "එස්": "s", "ටී": "t", "යූ": "u", "වී": "v", "ඩබ්ලිව්": "w", "ඩබ්": "w", "එක්ස්": "x",
	"වයි": "y", "ඉසෙඩ්": "z",
}

// Transliterate converts Sinhala to lower-case ASCII. Other characters pass
// through unchanged.
func Transliterate(s string) string {
	s = Normalise(s, false)
	var b strings.Builder
	pending := false // a consonant whose inherent "a" has not been written
	for _, r := range s {
		if v, ok := vowelSigns[r]; ok {
			b.WriteString(v)
			pending = false
			continue
		}
		if r == alLakuna {
			pending = false
			continue
		}
		if pending {
			b.WriteByte('a')
			pending = false
		}
		switch {
		case consonants[r] != "":
			b.WriteString(consonants[r])
			pending = true
		case vowels[r] != "":
			b.WriteString(vowels[r])
		case otherSigns[r] != "":
			b.WriteString(otherSigns[r])
		default:
			b.WriteRune(r)
		}
	}
	if pending {
		b.WriteByte('a')
	}
	return b.String()
}

// LatinText returns a lower-case Latin form of free text (titles), for
// trigram search: Sinhala is transliterated, everything else lower-cased.
func LatinText(s string) string {
	if s == "" {
		return ""
	}
	s = Normalise(s, false)
	if IsSinhala(s) {
		s = Transliterate(s)
	}
	return strings.ToLower(foldASCII(s))
}

// foldASCII strips Latin accents (NFKD, then drops the combining diacritics
// block) so "Pérera" matches "Perera". Marks of other scripts — Sinhala,
// Tamil — are left alone because they are letters' vowel signs, not accents.
func foldASCII(s string) string {
	d := norm.NFKD.String(s)
	return strings.Map(func(r rune) rune {
		if r >= '̀' && r <= 'ͯ' && unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, d)
}
