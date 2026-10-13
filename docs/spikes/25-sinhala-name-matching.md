# Spike #25 — Sinhala normalisation and cross-script name matching

> Status: **Results for owner review** · Date: 2026-10-13 · Issue: #25 · Code: [`spikes/sinhala-matching/`](../../spikes/sinhala-matching/README.md) (throwaway)
> Markers: **[NEEDS LAWYER CONFIRMATION]** · **[MAKE CONFIGURABLE]** · **[ASSUMPTION]** · **[DECISION REQUIRED]**

## 1. Question

Conflict checking (doc 04 §3) and global search (doc 07 §9) must find the same person whether the name was
typed in Sinhala or English, with Sri Lankan spelling variants, in any initials order, and regardless of how
the Sinhala was keyed. Can a normaliser + phonetic key + pg_trgm-style trigram similarity do this well enough,
and what must live in PostgreSQL versus application code?

## 2. Method

Go and PostgreSQL were not installed, so the spike is a Python (stdlib) prototype whose trigram function
reproduces pg_trgm `similarity()` (words padded with two leading and one trailing space; Jaccard over trigram sets).

1. **Normaliser** (`sinhala.py: normalise`)
   - NFC, then remove invisible characters (ZWSP, ZWNJ, BOM, soft hyphen, word joiner).
   - Reorder legacy `ZWJ + al-lakuna` to `al-lakuna + ZWJ`.
   - Move a kombuva typed in visual order (before its consonant or yansaya/rakaransaya conjunct) after the consonant — only when it cannot belong to a preceding consonant.
   - Compose legacy two-part sequences into single code points (අ+ා→ආ, එ+්→ඒ, ෙ+ෙ→ෛ, උ+ෟ→ඌ …), then NFC again (ෙ+ා→ො, ෙ+්→ේ).
   - Keep ZWJ only between al-lakuna and a consonant (display form); drop it entirely in the comparison form.
2. **Transliteration** (`sinhala.py: transliterate`) — consonant + inherent *a* unless followed by a vowel sign or al-lakuna; anusvara → `ng`; **vowel length dropped** because English spellings do not mark it reliably.
3. **Phonetic key** (`match.py: phonetic`), applied to both English input and transliterated Sinhala:
   `eir→er` (Pereira), `ey+consonant→e` (Abeysekera), `ae→a`, `ee→i`, `oo→u` (Sri Lankan English), `c/ck/q→k` but `ch→c`, `f→p` (Fernando = ප්‍රනාන්දු), `w→v`, drop aspirate `h` after k/g/j/t/d/p/b/s, anusvara before a stop → `n`, collapse doubled letters, word-final `e→a` (Jayawardene/Jayawardana, singhe/sinha).
4. **Consonant skeleton** — first letter plus consonants (absorbs a↔e vowel drift: Wickrama/Wickreme).
5. **Tokeniser** — separates initials from name tokens: `W.A. Perera`, `Perera W.A.`, `Perera WA`, `Perera, W. A.`, and Sinhala spelled-out initials (`ඩබ්ලිව්. ඒ.` = W. A.).
6. **Scorers compared**
   - **V1** raw pg_trgm on the stored strings (no shadow columns).
   - **V2** trigram on transliterated text.
   - **V3** trigram on an order-insensitive phonetic key (one shadow column; pure pg_trgm).
   - **V4** token-level: each name token takes the best of phonetic-trigram and 0.9 × skeleton-trigram, **averaged**; times an initials-compatibility factor.
   - **V5** as V4 but every token on the shorter side must match (**minimum** instead of mean).
7. **Test set** — 77 fabricated pairs of common Sri Lankan names (55 should match, 22 should not): 25 Sinhala-vs-English, 15 English spelling variants, 8 initials/ordering, 7 Sinhala typing variants, 22 near-miss non-matches (Jayasinghe/Jayasekara, Ranasinghe/Munasinghe, Bandara/Bandaranayake, same surname with different given names or initials …).

## 3. Results

| Scorer | t=0.3 | t=0.4 | t=0.5 | t=0.6 | t=0.7 |
|---|---|---|---|---|---|
| V1 raw pg_trgm | P 0.61 / R 0.49 | P 0.62 / R 0.45 | P 0.69 / R 0.40 | P 0.83 / R 0.35 | P 0.90 / R 0.16 |
| V2 transliterated | P 0.72 / R 0.98 | P 0.75 / R 0.93 | P 0.83 / R 0.89 | P 0.91 / R 0.73 | P 0.97 / R 0.51 |
| V3 phonetic key | P 0.71 / R 0.98 | P 0.73 / R 0.96 | P 0.80 / R 0.95 | P 0.91 / R 0.89 | P 0.98 / R 0.87 |
| V4 structured, mean | P 0.71 / R 1.00 | P 0.71 / R 1.00 | P 0.72 / R 1.00 | P 0.77 / R 1.00 | P 0.90 / R 1.00 |
| **V5 structured, min** | P 0.80 / R 1.00 | P 0.87 / R 0.98 | **P 0.96 / R 0.98** | **P 0.98 / R 0.96** | P 0.98 / R 0.96 |

P = precision, R = recall, over 77 pairs. V5 at t=0.6 by category: cross-script 25/25 · initials 8/8 · typing 7/7 · spelling 13/15 · non-matches correctly rejected 21/22.

**Typing variants:** all 7 Sinhala keying variants (ZWJ order, rakaransaya without ZWJ, visual-order kombuva, decomposed vowel signs, ZWSP, අ+ා) become **byte-identical** after `normalise()`.

**Findings**

1. **Raw pg_trgm is not enough** (V1 recall 0.35–0.49). It has no way to match across scripts, and typing variants break trigrams.
2. **The phonetic key does most of the work** (V3: P 0.91 / R 0.89 at 0.6) and can live entirely in Postgres as a shadow column with a GIN trigram index.
3. **Averaging token scores is dangerous for conflict checks** (V4): a shared given name alone yields ≥ 0.5, so "Nimal Perera" vs "Nimal Fernando" scores high. Requiring *every* token to match (V5) fixes this.
4. **Initials need structure, not trigrams.** "W.A. Perera" vs "Wanigasekara Arachchige Perera" only matches when initials are derived from the *written* given names (W, not the phonetic V).

## 4. Failure cases (V5, t=0.6)

| Pair | Score | Why | Handling |
|---|---|---|---|
| Saman Abeysekera / Saman Abeyesekara | 0.40 | the extra `ye` isn't covered by the `ey→e` rule | Add a variant dictionary of known surname spellings (§5) rather than more regex. **[MAKE CONFIGURABLE]** |
| Sandun Wijesinghe / Sandun Wijesinha | 0.54 | "sinha" keeps `nh`, "singhe" becomes `nga` | Same: surname variant dictionary. Deliberately **not** fixed by tuning, to avoid overfitting this test set |
| Anusha Kumara / Anusha Kumari (non-match) | 0.90 | a one-letter gender ending; trigram can't separate it | Acceptable for conflict search (recall over precision: a human reviews every hit, doc 04 §3). Show it as "possible" rather than "strong". |

Pairs that pass but sit close to the line: Bandara vs Bandaranayake (0.47), W.A. Perera vs K.P. Perera (0.50, saved by the initials factor), Silva vs Silvapulle (0.45).

## 5. Recommendation for the Go + PostgreSQL implementation

**Store (shadow columns on the person/organisation name record, all maintained by application code on write):**

| Column | Content | Index |
|---|---|---|
| `name_display` | Exactly as entered, after `normalise(keep_zwj=True)` | none (display) |
| `name_norm` | `normalise(keep_zwj=False)`, case-folded | btree for exact lookup |
| `name_latin` | Transliteration (Sinhala) or the folded English name | GIN `gin_trgm_ops` (search-as-you-type) |
| `name_key` | Space-joined, **sorted** phonetic keys of the name tokens | GIN `gin_trgm_ops` (candidate retrieval) |
| `name_skeleton` | Sorted consonant skeletons | GIN `gin_trgm_ops` (optional second net) |
| `initials` | Sorted initials, including those derived from written given names | btree / array |

Each name variant (Sinhala, English, alias, former name) is stored as its own row against the entity, so a person entered in both scripts matches either way (doc 08: one registry, many names).

**Split of responsibilities**

- **Application code (Go), at write time:** normalisation, transliteration, phonetic key, skeleton, initials. These rules are versioned; a rule change triggers a backfill job. Postgres cannot do these well — pg_trgm has no notion of Sinhala, ZWJ or transliteration, and doing them in PL/pgSQL would be hard to test.
- **PostgreSQL, at query time:** candidate retrieval with `name_key % $1 OR name_skeleton % $2 OR name_latin % $3` (pg_trgm, `similarity_threshold` around 0.3), limited to a few hundred candidates per query.
- **Application code, re-ranking:** the V5 structured score on the candidates (token min-match and initials factor), bucketed for the conflict UI as *Exact* / *Strong* (≥ 0.8) / *Possible* (0.5–0.8). **[MAKE CONFIGURABLE]** thresholds per firm.
- **Surname variant dictionary** (reference data, editable by admins): Abeysekera ≈ Abeyesekara, Wijesinghe ≈ Wijesinha … It handles the long tail better than more regex rules. **[MAKE CONFIGURABLE]**
- **Conflict checks are tuned for recall:** run at the lower threshold (0.5), let the human decide (doc 04 §3). Global search can use 0.6.

**Input rules:** normalise on every write *and* every query; never compare un-normalised text.

## 6. What this spike cannot conclude

- **Real-world accuracy.** The test set is fabricated and the rules were written alongside it, so these numbers are **optimistic**. Re-run on a few hundred anonymised names from the firm's existing records (paper diary, Excel) before fixing thresholds. **[NEEDS LAWYER CONFIRMATION]** that an anonymised extract can be used.
- **pg_trgm behaviour on raw Sinhala.** pg_trgm only keeps "alphanumeric" characters, and whether Sinhala vowel signs (Unicode Mn/Mc) count depends on the database locale. The prototype treated them as word characters. If Postgres does not, raw-Sinhala trigrams split inside words, which is another reason to index only the Latin shadow columns. **Verify in Postgres 16+ with an ICU collation.** **[ASSUMPTION]**
- **Performance** at the firm's volume, and the GIN index size. This needs Postgres.
- **Tamil names**, and Muslim and Burgher naming conventions (e.g. "Mohamed", "bin", Portuguese-origin surnames beyond Fernando/Perera/Silva). These are not covered. **[NEEDS LAWYER CONFIRMATION]** whether the client base includes them in the MVP.
- **Legacy-font text** (FM Abhaya, Kaputa etc.) imported from old Word files must be converted to Unicode **before** normalisation. That is a separate converter, tracked under the migration plan (doc 16).
- **Name structure:** Sinhala names with *ge* names (Herath Mudiyanselage …) and honorifics need a tokeniser rule set agreed with the lawyer, e.g. whether "Mudiyanselage" ever counts for matching. **[NEEDS LAWYER CONFIRMATION]**

## 7. Next steps

1. Owner review of this result → feed §5 into the ADR on search (issue #22) and the logical data model (issue #29).
2. When Postgres is available: load the 77 pairs, verify pg_trgm word splitting on Sinhala, and measure candidate retrieval with the GIN indexes.
3. Collect an anonymised real-name sample and re-run `evaluate.py` with it.
