"""Sinhala Unicode normalisation and Sinhala -> Latin transliteration (throwaway spike, issue #25)."""
import re
import unicodedata

AL = "්"      # al-lakuna (virama)
ZWJ = "‍"
CONS = "[ක-ෆ]"
KOMBU = "[ෙෛ]"

# Invisible characters that carry no meaning in a name.
INVISIBLE = dict.fromkeys(map(ord, "​‌﻿­⁠᠎"), None)

# Sequences produced by legacy keyboards / visual-order typing -> canonical code points.
COMPOSE = [
    ("ෙෙ", "ෛ"),  # kombu + kombu -> kombu deka (ai)
    ("අා", "ආ"),  # a + aa-sign -> aa
    ("අැ", "ඇ"),  # a + ae-sign -> ae
    ("අෑ", "ඈ"),  # a + aae-sign -> aae
    ("උෟ", "ඌ"),  # u + gayanukitta -> uu
    ("එ්", "ඒ"),  # e + al-lakuna -> ee
    ("ඔ්", "ඕ"),  # o + al-lakuna -> oo
    ("ඔෟ", "ඖ"),  # o + gayanukitta -> au
]


def normalise(s: str, keep_zwj: bool = True) -> str:
    """Canonical form for storage/display (keep_zwj=True) or comparison (keep_zwj=False)."""
    s = unicodedata.normalize("NFC", s).translate(INVISIBLE)
    s = re.sub(r"\s+", " ", s).strip()
    # Legacy order: ZWJ typed before al-lakuna.
    s = s.replace(ZWJ + AL, AL + ZWJ)
    # Visual-order typing: kombuva typed before its consonant (or conjunct with yansaya/rakaransaya),
    # only where it cannot belong to a preceding consonant.
    s = re.sub(rf"(?<![ක-ෆ්‍]){KOMBU}({CONS}(?:{AL}{ZWJ}?[යර])?)",
               lambda m: m.group(1) + m.group(0)[0], s)
    for a, b in COMPOSE:
        s = s.replace(a, b)
    s = unicodedata.normalize("NFC", s)  # composes kombu + aa / al-lakuna into o / ee signs
    # ZWJ is only meaningful between al-lakuna and a consonant (yansaya, rakaransaya, touching letters).
    s = re.sub(rf"(?<!{AL}){ZWJ}|{ZWJ}(?!{CONS})", "", s)
    if not keep_zwj:
        s = s.replace(ZWJ, "")
    return s


CONSONANTS = {
    "ක": "k", "ඛ": "kh", "ග": "g", "ඝ": "gh", "ඞ": "ng", "ඟ": "ng", "ච": "ch", "ඡ": "chh", "ජ": "j",
    "ඣ": "jh", "ඤ": "ny", "ඥ": "gny", "ඦ": "nj", "ට": "t", "ඨ": "th", "ඩ": "d", "ඪ": "dh", "ණ": "n",
    "ඬ": "nd", "ත": "t", "ථ": "th", "ද": "d", "ධ": "dh", "න": "n", "ඳ": "nd", "ප": "p", "ඵ": "ph",
    "බ": "b", "භ": "bh", "ම": "m", "ඹ": "mb", "ය": "y", "ර": "r", "ල": "l", "ව": "v", "ශ": "sh",
    "ෂ": "sh", "ස": "s", "හ": "h", "ළ": "l", "ෆ": "f",
}
# Vowel length is dropped on purpose: English spellings of Sinhala names do not mark it reliably.
VOWEL_SIGNS = {
    "ා": "a", "ැ": "ae", "ෑ": "ae", "ි": "i", "ී": "i", "ු": "u", "ූ": "u", "ෘ": "ru", "ෲ": "ru",
    "ෙ": "e", "ේ": "e", "ෛ": "ai", "ො": "o", "ෝ": "o", "ෞ": "au", "ෟ": "lu",
}
VOWELS = {
    "අ": "a", "ආ": "a", "ඇ": "ae", "ඈ": "ae", "ඉ": "i", "ඊ": "i", "උ": "u", "ඌ": "u", "ඍ": "ru",
    "ඎ": "ru", "ඏ": "lu", "ඐ": "lu", "එ": "e", "ඒ": "e", "ඓ": "ai", "ඔ": "o", "ඕ": "o", "ඖ": "au",
}
OTHER = {"ං": "ng", "ඃ": "h"}

# Sinhala spellings of English initials ("ඩබ්ලිව්. ඒ. පෙරේරා" = "W. A. Perera").
LETTER_NAMES = {
    "ඒ": "a", "බී": "b", "සී": "c", "ඩී": "d", "ඊ": "e", "එෆ්": "f", "ජී": "g", "එච්": "h", "අයි": "i",
    "ජේ": "j", "කේ": "k", "එල්": "l", "එම්": "m", "එන්": "n", "ඕ": "o", "පී": "p", "කිව්": "q",
    "ආර්": "r", "එස්": "s", "ටී": "t", "යූ": "u", "වී": "v", "ඩබ්ලිව්": "w", "ඩබ්": "w", "එක්ස්": "x",
    "වයි": "y", "ඉසෙඩ්": "z",
}


def is_sinhala(s: str) -> bool:
    return any("඀" <= c <= "෿" for c in s)


def transliterate(s: str) -> str:
    """Sinhala -> lowercase ASCII. Non-Sinhala characters pass through."""
    s = normalise(s, keep_zwj=False)
    out, pending = [], False
    for c in s:
        if c in VOWEL_SIGNS:
            out.append(VOWEL_SIGNS[c]); pending = False; continue
        if c == AL:
            pending = False; continue
        if pending:
            out.append("a"); pending = False
        if c in CONSONANTS:
            out.append(CONSONANTS[c]); pending = True
        elif c in VOWELS:
            out.append(VOWELS[c])
        elif c in OTHER:
            out.append(OTHER[c])
        else:
            out.append(c)
    if pending:
        out.append("a")
    return "".join(out)
