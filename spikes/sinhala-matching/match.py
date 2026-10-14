"""Phonetic keys, pg_trgm-style trigram similarity and name scorers (throwaway spike, issue #25)."""
import re
import unicodedata

from sinhala import LETTER_NAMES, is_sinhala, normalise, transliterate

# Ordered rewrite rules applied to lowercase ASCII (from English input or Sinhala transliteration).
RULES = [
    (r"eir", "er"),                 # Pereira -> Perera
    (r"ey(?=[^aeiou])", "e"),       # Abeysekera -> Abesekera
    (r"ae", "a"),                   # Sinhala ae vowel; English writes a or e
    (r"ee", "i"), (r"oo", "u"),     # Sri Lankan English: ee = long i, oo = long u
    (r"ch", "C"), (r"ck", "k"), (r"c", "k"), (r"q", "k"), (r"C", "c"),
    (r"x", "ks"), (r"z", "s"), (r"f", "p"), (r"w", "v"),
    (r"([kgjtdpbs])h", r"\1"),      # aspirates and sh/th/dh: kh gh jh th dh ph bh sh
    (r"ng(?=[kgjcdt])", "n"),       # anusvara before a stop: Priyanka/priyangka, Ranjith/rangjit
    (r"(.)\1+", r"\1"),             # doubled letters / vowel length
    (r"e$", "a"),                   # word-final e ~ a (Jayawardene / Jayawardana, singhe / sinha)
]


def phonetic(word: str) -> str:
    w = unicodedata.normalize("NFKD", word.lower())
    w = "".join(c for c in w if c.isascii() and c.isalpha())
    for pat, rep in RULES:
        w = re.sub(pat, rep, w)
    return w


def skeleton(key: str) -> str:
    """Consonant skeleton: first letter kept, later vowels dropped."""
    return key[:1] + re.sub(r"[aeiou]", "", key[1:])


def tokens(name: str):
    """Split a name into (is_initial, latin) tokens. Handles 'W.A. Perera', 'Perera W A', Sinhala initials."""
    if is_sinhala(name):
        name = normalise(name, keep_zwj=False)
        parts = [p for p in re.split(r"[\s,]+", name.replace(".", ". ")) if p]
        out = []
        for p in parts:
            dotted, core = p.endswith("."), p.rstrip(".")
            if not core:
                continue
            if core in LETTER_NAMES and (dotted or len(parts) > 1 and len(core) <= 3):
                out.append((True, LETTER_NAMES[core]))
            elif dotted:
                out.append((True, transliterate(core)[:1]))
            else:
                out.append((False, transliterate(core)))
        return out
    out = []
    for p in re.split(r"[\s,]+", name.replace(".", ". ")):
        core = p.strip(".")
        if not core:
            continue
        if len(core) == 1 or p.endswith(".") and len(core) <= 2:
            out.append((True, core.lower()[:1]))
        elif core.isupper() and len(core) <= 3:          # "Perera WA"
            out.extend((True, c.lower()) for c in core)
        else:
            out.append((False, core.lower()))
    return out


# --- pg_trgm-compatible similarity -------------------------------------------------------------

def _is_word_char(c: str) -> bool:
    # pg_trgm keeps "alphanumeric" chars per locale; we treat Sinhala vowel signs (Mn/Mc) as word chars.
    return c.isalnum() or unicodedata.category(c).startswith("M")


def trigrams(text: str) -> set:
    words = "".join(c if _is_word_char(c) else " " for c in text.lower()).split()
    grams = set()
    for w in words:
        p = "  " + w + " "
        grams.update(p[i:i + 3] for i in range(len(p) - 2))
    return grams


def similarity(a: str, b: str) -> float:
    ta, tb = trigrams(a), trigrams(b)
    return len(ta & tb) / len(ta | tb) if ta and tb else 0.0


# --- Scorers -----------------------------------------------------------------------------------

def score_raw(a, b):
    """V1: plain pg_trgm on the stored strings (what Postgres gives with no shadow columns)."""
    return similarity(a, b)


def _latin(name):
    return " ".join(t for _, t in tokens(name))


def score_translit(a, b):
    """V2: trigram on transliterated text, no phonetic rules."""
    return similarity(_latin(a), _latin(b))


def _key(name):
    return " ".join(sorted(phonetic(t) if not i else t for i, t in tokens(name)))


def score_phonetic(a, b):
    """V3: trigram on the order-insensitive phonetic key (one shadow column, pure pg_trgm)."""
    return similarity(_key(a), _key(b))


def _tok_sim(x, y):
    return max(similarity(x, y), 0.9 * similarity(skeleton(x), skeleton(y)))


def _initials_factor(ia, ib, na, nb, ra, rb):
    """Compare initials; if one side has only initials, derive initials from the other side's unmatched given names."""
    if ia and ib:
        if set(ia) == set(ib):
            return 1.0
        return 0.85 if set(ia) <= set(ib) or set(ib) <= set(ia) else 0.5
    if ia or ib:
        inits, names_init_side, other_names, other_raw = (ia, na, nb, rb) if ia else (ib, nb, na, ra)
        rest = [raw for o, raw in zip(other_names, other_raw)
                if max((_tok_sim(o, n) for n in names_init_side), default=0) < 0.5]
        if not rest:
            return 1.0
        derived = {r[0] for r in rest}  # initials come from the written name, not the phonetic key (W, not V)
        return 1.0 if set(inits) <= derived or derived <= set(inits) else 0.5
    return 1.0


def score_structured(a, b, combine=min):
    """V5: token-level phonetic + skeleton matching with initials awareness (app-side re-ranking).
    Every name token on the shorter side must find a counterpart (min); V4 averages instead."""
    ta, tb = tokens(a), tokens(b)
    ra = [t for i, t in ta if not i]
    rb = [t for i, t in tb if not i]
    na = [phonetic(t) for i, t in ta if not i]
    nb = [phonetic(t) for i, t in tb if not i]
    ia = [t for i, t in ta if i]
    ib = [t for i, t in tb if i]
    if not na or not nb:
        return 0.0
    short, long_ = (na, nb) if len(na) <= len(nb) else (nb, na)
    used, sims = set(), []
    for s in short:                     # greedy best match without reuse
        best, bj = 0.0, None
        for j, l in enumerate(long_):
            if j not in used and (v := _tok_sim(s, l)) > best:
                best, bj = v, j
        if bj is not None:
            used.add(bj)
        sims.append(best)
    name_sim = combine(sims)
    if len(short) != len(long_) and not (ia or ib):
        name_sim *= 0.9                 # unexplained extra given names
    return name_sim * _initials_factor(ia, ib, na, nb, ra, rb)


def score_structured_mean(a, b):
    return score_structured(a, b, combine=lambda v: sum(v) / len(v))


SCORERS = {
    "V1 raw pg_trgm": score_raw,
    "V2 transliterated": score_translit,
    "V3 phonetic key": score_phonetic,
    "V4 structured (mean of tokens)": score_structured_mean,
    "V5 structured (min of tokens)": score_structured,
}
