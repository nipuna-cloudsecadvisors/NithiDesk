package textnorm

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Phonetic returns the phonetic key of one name word, from English input or
// transliterated Sinhala. The rules absorb the common Sri Lankan spelling
// variation (Pereira/Perera, Fernando/ප්\u200Dරනාන්දු, Jayawardene/Jayawardana).
func Phonetic(word string) string {
	d := norm.NFKD.String(strings.ToLower(word))
	var b strings.Builder
	for _, r := range d {
		if r < unicode.MaxASCII && unicode.IsLetter(r) {
			b.WriteRune(r)
		}
	}
	w := b.String()

	w = strings.ReplaceAll(w, "eir", "er")  // Pereira -> Perera
	w = eyBeforeConsonant(w)                // Abeysekera -> Abesekera
	w = strings.ReplaceAll(w, "ae", "a")    // Sinhala ae vowel; English writes a or e
	w = strings.ReplaceAll(w, "ee", "i")    // Sri Lankan English: ee = long i
	w = strings.ReplaceAll(w, "oo", "u")    // oo = long u
	w = strings.ReplaceAll(w, "ch", "\x01") // keep ch distinct from c
	w = strings.ReplaceAll(w, "ck", "k")
	w = strings.ReplaceAll(w, "c", "k")
	w = strings.ReplaceAll(w, "q", "k")
	w = strings.ReplaceAll(w, "\x01", "c")
	w = strings.ReplaceAll(w, "x", "ks")
	w = strings.ReplaceAll(w, "z", "s")
	w = strings.ReplaceAll(w, "f", "p") // Fernando = ප්\u200Dරනාන්දු
	w = strings.ReplaceAll(w, "w", "v")
	w = dropAspirates(w)      // kh gh jh th dh ph bh sh
	w = anusvaraBeforeStop(w) // Priyanka/priyangka
	w = collapseRuns(w)       // doubled letters, vowel length
	if strings.HasSuffix(w, "e") {
		w = w[:len(w)-1] + "a" // word-final e ~ a (singhe / sinha)
	}
	return w
}

func isVowel(c byte) bool { return strings.IndexByte("aeiou", c) >= 0 }

func eyBeforeConsonant(w string) string {
	var b strings.Builder
	for i := 0; i < len(w); i++ {
		if w[i] == 'e' && i+2 < len(w) && w[i+1] == 'y' && !isVowel(w[i+2]) {
			b.WriteByte('e')
			i++ // skip the y
			continue
		}
		b.WriteByte(w[i])
	}
	return b.String()
}

func dropAspirates(w string) string {
	var b strings.Builder
	for i := 0; i < len(w); i++ {
		if w[i] == 'h' && i > 0 && strings.IndexByte("kgjtdpbs", w[i-1]) >= 0 {
			continue
		}
		b.WriteByte(w[i])
	}
	return b.String()
}

func anusvaraBeforeStop(w string) string {
	var b strings.Builder
	for i := 0; i < len(w); i++ {
		if w[i] == 'n' && i+2 < len(w) && w[i+1] == 'g' && strings.IndexByte("kgjcdt", w[i+2]) >= 0 {
			b.WriteByte('n')
			i++ // drop the g
			continue
		}
		b.WriteByte(w[i])
	}
	return b.String()
}

func collapseRuns(w string) string {
	var b strings.Builder
	for i := 0; i < len(w); i++ {
		if i > 0 && w[i] == w[i-1] {
			continue
		}
		b.WriteByte(w[i])
	}
	return b.String()
}

// Skeleton is the consonant skeleton of a phonetic key: first letter kept,
// later vowels dropped. It absorbs a/e vowel drift (Wickrama/Wickreme).
func Skeleton(key string) string {
	if key == "" {
		return ""
	}
	var b strings.Builder
	b.WriteByte(key[0])
	for i := 1; i < len(key); i++ {
		if !isVowel(key[i]) {
			b.WriteByte(key[i])
		}
	}
	return b.String()
}

// Token is one part of a personal name.
type Token struct {
	Initial bool   // an initial ("W." or "W" in "Perera WA")
	Latin   string // lower-case Latin form
}

// Tokens splits a name into initials and name words. It understands
// "W.A. Perera", "Perera W.A.", "Perera WA", "Perera, W. A." and Sinhala
// spelled-out initials ("ඩබ්ලිව්. ඒ. පෙරේරා").
func Tokens(name string) []Token {
	if IsSinhala(name) {
		name = Normalise(name, false)
		parts := splitName(strings.ReplaceAll(name, ".", ". "))
		var out []Token
		for _, p := range parts {
			dotted := strings.HasSuffix(p, ".")
			core := strings.TrimRight(p, ".")
			if core == "" {
				continue
			}
			if letter, ok := letterNames[core]; ok && (dotted || (len(parts) > 1 && runeLen(core) <= 3)) {
				out = append(out, Token{Initial: true, Latin: letter})
			} else if dotted {
				out = append(out, Token{Initial: true, Latin: firstByte(Transliterate(core))})
			} else {
				out = append(out, Token{Latin: Transliterate(core)})
			}
		}
		return out
	}

	var out []Token
	for _, p := range splitName(strings.ReplaceAll(name, ".", ". ")) {
		core := strings.Trim(p, ".")
		if core == "" {
			continue
		}
		switch {
		case runeLen(core) == 1 || (strings.HasSuffix(p, ".") && runeLen(core) <= 2):
			out = append(out, Token{Initial: true, Latin: firstByte(strings.ToLower(core))})
		case isUpperWord(core) && runeLen(core) <= 3: // "Perera WA"
			for _, r := range strings.ToLower(core) {
				out = append(out, Token{Initial: true, Latin: string(r)})
			}
		default:
			out = append(out, Token{Latin: strings.ToLower(foldASCII(core))})
		}
	}
	return out
}

func splitName(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == ',' })
}

func runeLen(s string) int { return len([]rune(s)) }

func firstByte(s string) string {
	if s == "" {
		return ""
	}
	return string([]rune(s)[:1])
}

func isUpperWord(s string) bool {
	cased := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			cased = true
		}
	}
	return cased
}

// Trigrams returns the pg_trgm-compatible trigram set of text: words padded
// with two leading spaces and one trailing space.
func Trigrams(text string) map[string]struct{} {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r))
	})
	grams := make(map[string]struct{})
	for _, w := range words {
		p := []rune("  " + w + " ")
		for i := 0; i+3 <= len(p); i++ {
			grams[string(p[i:i+3])] = struct{}{}
		}
	}
	return grams
}

// Similarity is pg_trgm's similarity(): Jaccard over trigram sets.
func Similarity(a, b string) float64 {
	ta, tb := Trigrams(a), Trigrams(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	shared := 0
	for g := range ta {
		if _, ok := tb[g]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(ta)+len(tb)-shared)
}

// NameKey is the order-insensitive phonetic key of a name: phonetic keys of
// the name words and the initials, sorted and space-joined.
func NameKey(name string) string {
	var parts []string
	for _, t := range Tokens(name) {
		if t.Initial {
			parts = append(parts, t.Latin)
		} else {
			parts = append(parts, Phonetic(t.Latin))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func tokenSimilarity(x, y string) float64 {
	return max(Similarity(x, y), 0.9*Similarity(Skeleton(x), Skeleton(y)))
}

// ScoreNames scores how likely two names denote the same person, in [0, 1].
// Every name word on the shorter side must find a counterpart (spike V5:
// averaging lets a shared given name alone score high), and initials are
// compared structurally, derived from written given names where one side
// spells them out. A human always decides; this ranks candidates.
func ScoreNames(a, b string) float64 {
	ta, tb := Tokens(a), Tokens(b)
	var ra, rb, na, nb, ia, ib []string
	for _, t := range ta {
		if t.Initial {
			ia = append(ia, t.Latin)
		} else {
			ra = append(ra, t.Latin)
			na = append(na, Phonetic(t.Latin))
		}
	}
	for _, t := range tb {
		if t.Initial {
			ib = append(ib, t.Latin)
		} else {
			rb = append(rb, t.Latin)
			nb = append(nb, Phonetic(t.Latin))
		}
	}
	if len(na) == 0 || len(nb) == 0 {
		return 0
	}

	short, long := na, nb
	if len(na) > len(nb) {
		short, long = nb, na
	}
	used := make(map[int]bool)
	nameSim := 1.0
	for _, s := range short {
		best, bestJ := 0.0, -1
		for j, l := range long {
			if used[j] {
				continue
			}
			if v := tokenSimilarity(s, l); v > best {
				best, bestJ = v, j
			}
		}
		if bestJ >= 0 {
			used[bestJ] = true
		}
		nameSim = min(nameSim, best)
	}
	if len(short) != len(long) && len(ia) == 0 && len(ib) == 0 {
		nameSim *= 0.9 // unexplained extra given names
	}
	return nameSim * initialsFactor(ia, ib, na, nb, ra, rb)
}

func initialsFactor(ia, ib, na, nb, ra, rb []string) float64 {
	switch {
	case len(ia) > 0 && len(ib) > 0:
		sa, sb := toSet(ia), toSet(ib)
		if setEqual(sa, sb) {
			return 1.0
		}
		if subset(sa, sb) || subset(sb, sa) {
			return 0.85
		}
		return 0.5
	case len(ia) > 0 || len(ib) > 0:
		inits, namesInitSide, otherNames, otherRaw := ia, na, nb, rb
		if len(ia) == 0 {
			inits, namesInitSide, otherNames, otherRaw = ib, nb, na, ra
		}
		var rest []string
		for i, o := range otherNames {
			best := 0.0
			for _, n := range namesInitSide {
				best = max(best, tokenSimilarity(o, n))
			}
			if best < 0.5 {
				rest = append(rest, otherRaw[i])
			}
		}
		if len(rest) == 0 {
			return 1.0
		}
		// Initials come from the written name, not the phonetic key (W, not V).
		derived := map[string]struct{}{}
		for _, r := range rest {
			derived[firstByte(r)] = struct{}{}
		}
		si := toSet(inits)
		if subset(si, derived) || subset(derived, si) {
			return 1.0
		}
		return 0.5
	default:
		return 1.0
	}
}

func toSet(xs []string) map[string]struct{} {
	s := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		s[x] = struct{}{}
	}
	return s
}

func subset(a, b map[string]struct{}) bool {
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func setEqual(a, b map[string]struct{}) bool { return len(a) == len(b) && subset(a, b) }

// Match strength buckets for the conflict and search UI (spike #25 §5).
const (
	StrongThreshold   = 0.8
	PossibleThreshold = 0.5
)

// Shadows are the machine-maintained search forms of a set of name variants
// (the same person written in English, Sinhala, Tamil). Each field joins the
// per-variant forms with " / " so pg_trgm word similarity matches any one.
type Shadows struct {
	Norm     string
	Latin    string
	Key      string
	Skeleton string
	Initials string
}

// NameShadows builds the shadow columns for one entity's name variants.
func NameShadows(variants ...string) Shadows {
	var norms, latins, keys, skeletons []string
	initials := map[string]struct{}{}
	seen := map[string]bool{}
	for _, v := range variants {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		n := strings.ToLower(Normalise(v, false))
		if seen[n] {
			continue
		}
		seen[n] = true
		norms = append(norms, n)

		var latin, sk []string
		for _, t := range Tokens(v) {
			latin = append(latin, t.Latin)
			if t.Initial {
				initials[t.Latin] = struct{}{}
			} else {
				sk = append(sk, Skeleton(Phonetic(t.Latin)))
			}
		}
		sort.Strings(sk)
		latins = append(latins, strings.Join(latin, " "))
		keys = append(keys, NameKey(v))
		skeletons = append(skeletons, strings.Join(sk, " "))
	}
	var inits []string
	for i := range initials {
		inits = append(inits, i)
	}
	sort.Strings(inits)
	return Shadows{
		Norm:     strings.Join(norms, " / "),
		Latin:    strings.Join(latins, " / "),
		Key:      strings.Join(keys, " / "),
		Skeleton: strings.Join(skeletons, " / "),
		Initials: strings.Join(inits, " "),
	}
}

// QueryKey returns the phonetic key of a search query, for retrieval against
// the stored Key shadow.
func QueryKey(q string) string { return NameKey(q) }
