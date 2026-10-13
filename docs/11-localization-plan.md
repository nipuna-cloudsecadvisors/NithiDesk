# 11 — Localization Plan (Sinhala · English · Tamil-ready)

Covers PART 21.

> Sinhala and English are **first-class from day one**. Tamil is architecturally supported (fields, locale files, fonts, collation) but not translated in MVP. **[DECISION REQUIRED]** Confirm whether the pilot firm has Tamil-language matters (Northern/Eastern courts) — if so, Tamil may move earlier.

---

## 1. Three different localization problems

| Problem | Example | Approach |
|---|---|---|
| **UI language** | Buttons, labels, messages | Locale message catalogues; user preference |
| **Content language** | Client names, notes, documents, OCR | Unicode data; bilingual fields where needed; script-aware search |
| **Communication language** | Client updates, letters, SMS | Templates per language; client preference |

A user may use the Sinhala UI while writing notes in English about a client whose name is stored in both scripts and who wants updates in Sinhala. All three are independent.

---

## 2. UI localization

| Aspect | Plan |
|---|---|
| Languages | `si-LK`, `en-LK`; `ta-LK` scaffolded |
| Preference | Per user (profile); quick toggle in header; default from firm setting |
| **Dual-label mode** | Optional per user: "Sinhala with English terms" — e.g. `නඩු අංකය / Case Number`. Useful because many legal terms are used in English even by Sinhala speakers. Three modes: English · සිංහල · සිංහල + English |
| Message format | ICU MessageFormat (plurals, gender-neutral, variables) — no string concatenation |
| Catalogues | Namespaced files per module (`locales/si/hearings.json` etc.); keys are semantic (`hearing.verification.unverified`) |
| No hardcoded text | Lint rule in CI fails on literal UI strings; pseudo-localisation build to catch untranslated/overflowing text |
| Server messages | The Go API returns **stable error and status codes with parameters** (e.g. `hearing.date_in_past`), not sentences. The frontend, email and PDF renderers translate them. Validation messages, notification text and email bodies are never composed in English inside business logic |
| Key parity | CI fails when a key exists in `en` but not in `si`. For `ta`, a missing key is reported but allowed, and falls back to English |
| Translation workflow | English source → Sinhala translation by legal-literate translator → **lawyer review of legal terms** → glossary locked |
| Reference data labels | Court names, case types, stages, roles, purposes stored **in the database with per-language labels** (they are configuration, not UI strings) |
| Layout | Sinhala text is often 20–40% wider and taller (stacked vowel signs); components must wrap, not truncate mid-cluster; test at 320 px width |
| Fonts | **Noto Sans Sinhala** (UI) and Noto Serif Sinhala (documents) self-hosted; fallback Iskoola Pota (Windows), Sinhala Sangam MN (Apple). Never images of text |
| Input | Users type Sinhala via OS keyboards (Wijesekara/phonetic) or phone apps (e.g. phonetic "Singlish" keyboards). **[ASSUMPTION]** no in-app transliteration keyboard in MVP; evaluate a "type Singlish → Sinhala" helper in Phase 2 for name fields |

## 3. Legal terminology glossary

A controlled bilingual (later trilingual) glossary is a product asset, reviewed by the lawyer. Examples (illustrative, **[NEEDS LAWYER CONFIRMATION]** preferred terms):

| English | Sinhala (proposed) | Note |
|---|---|---|
| Case number | නඩු අංකය | |
| Court | අධිකරණය | |
| Next court date | ඊළඟ නඩු දිනය | |
| Client | සේවාදායකයා | Some firms may prefer "පක්ෂකරු"? — confirm |
| Documents | ලේඛන / ගොනු | ගොනු = files/folders; confirm |
| Lawyer / Attorney-at-Law | නීතිඥ | |
| Plaintiff / Defendant | පැමිණිලිකරු / විත්තිකරු | |
| Petitioner / Respondent | පෙත්සම්කරු / වගඋත්තරකරු | |
| Accused | චූදිතයා | |
| Judgment | තීන්දුව | |
| Order | නියෝගය | |
| Journal entry | (confirm) | |
| Verified / Not verified | තහවුරු කළ / තහවුරු කර නැත | |
| Hearing / Trial | විභාගය / නඩු විභාගය | confirm distinctions |

Rule: where a term is ambiguous or commonly used in English in practice, **dual-label mode shows both**; the glossary records the decision.

## 4. Content storage

| Rule | Detail |
|---|---|
| Encoding | UTF-8 everywhere (database, API, files, logs, CSV import/export with BOM option for Excel) |
| Database | PostgreSQL UTF-8; ICU collations for sorting (`si-LK` / `en-LK` / `ta-LK`) |
| Names & titles | Parallel fields: `name_en`, `name_si`, `name_ta` (+ name variants list). Either may be empty; at least one required. Display picks the user's language, falls back to the other, and shows both where space allows |
| Free text (notes, descriptions, outcomes) | Single field, any script or mixed; language auto-detected for search/OCR hints |
| Addresses | Free text bilingual + structured district/province from reference list (bilingual labels) |
| **Normalisation** | Stored as entered (for fidelity) **plus** a normalised shadow for search: Unicode NFC; consistent ZWJ (U+200D) / ZWNJ handling for yansaya, rakaransaya and touching letters; removal of stray zero-width characters; consistent vowel-sign ordering |
| No separate databases per language | Single database, single record per entity |

## 5. Search across scripts

| Capability | Approach | Phase |
|---|---|---|
| Sinhala exact/partial | Normalised text + trigram index | MVP |
| English exact/partial | Lower-cased, punctuation-stripped, initials-aware | MVP |
| Cross-script names ("Perera" ↔ "පෙරේරා") | Generate **transliteration keys** for every name variant at write time (Sinhala→Latin phonetic key; Latin→phonetic key) and compare keys; seeded **dictionary of common Sri Lankan names** with curated equivalents (Fernando ↔ ප්‍රනාන්දු, Silva ↔ සිල්වා, Jayasinghe ↔ ජයසිංහ) | MVP for names (needed for conflict search) |
| Cross-script document text | Query expansion using transliteration + glossary | Phase 2 |
| Semantic multilingual | Multilingual embeddings | Phase 3 |
| Stemming | No reliable open Sinhala stemmer assumed; rely on normalisation + trigram; evaluate later | — |

**Transliteration is assistive, never authoritative** — it widens matches for human review.

## 6. Dates, numbers, names

| Item | Plan |
|---|---|
| Dates | Display `DD/MM/YYYY` (Sri Lankan convention, **[NEEDS LAWYER CONFIRMATION]**) with localised month/day names in long form (e.g. "2026 ඔක්තෝබර් 12, සඳුදා") |
| Timezone | Asia/Colombo display; local dates for court events (doc 06) |
| Numbers | Western Arabic digits in both languages |
| Currency | LKR, `Rs. 1,250,000.00` |
| Personal names | Support initials-first ("W.A.D. Perera"), full-name, and ge-names / vasagama conventions; no forced first/last split |
| Sorting | Sinhala collation for Sinhala lists; mixed lists sort by display script |

## 7. OCR for Sinhala

| Option | Pros | Cons |
|---|---|---|
| Tesseract (open source) with Sinhala (`sin`) + English models | Self-hosted; no data leaves the firm; free | Moderate accuracy on poor scans, stamps, handwriting; needs pre-processing (deskew, denoise, binarise) |
| Cloud OCR services that support Sinhala (e.g. major cloud document-AI providers) | Generally higher accuracy, layout handling | **Confidential documents leave the environment**; cost; data residency; needs firm consent |
| Hybrid | Self-hosted default; cloud OCR opt-in per document/matter | Complexity |

**Recommendation:** OCR behind a provider interface; start self-hosted (Tesseract) in Phase 2; run an **accuracy benchmark on 50–100 real (anonymised or consented) Sinhala court documents** before committing; offer cloud OCR only as an explicit per-firm opt-in. Always store confidence, flag low-confidence pages, and never treat OCR text as authoritative (doc 19, risk R12). Handwritten journal/minutes: assume OCR **will not** work reliably.

## 8. Legacy Sinhala encodings (critical for migration)

Many older Sri Lankan Word documents use **non-Unicode legacy fonts** (e.g. FM-family and DL-family fonts) where Sinhala glyphs are mapped onto Latin code points. Such text looks correct in Word but is gibberish to search and OCR text layers.

Plan: migration tooling detects legacy-font text in DOC/DOCX and converts it to Unicode using mapping tables (doc 16); converted output is spot-checked by a human. **[NEEDS LAWYER CONFIRMATION]** which fonts the firm uses (inventory during discovery).

## 9. Generated documents (PDF, letters, reports)

- Sinhala requires complex text shaping (conjuncts, vowel signs). Many server-side PDF libraries render Sinhala incorrectly.
- **Recommendation:** generate PDFs from HTML templates via a headless browser engine (correct shaping with embedded Noto fonts), or a PDF library with HarfBuzz shaping. **Prototype in Milestone 1** (a bilingual daily brief) to retire this risk early.
- Templates per language; bilingual templates where courts/clients expect both.
- Letters to clients use the client's preferred language; internal reports use the user's UI language.

### 9.1 Reports and exports

| Item | Rule |
|---|---|
| On-screen reports | Headings, column names and enum values come from the catalogue in the viewer's language. Data values such as names show in the viewer's preferred script with fallback |
| PDF reports (daily brief, diary, matter summary) | Same HTML-to-PDF pipeline; language chosen when generating; option for **bilingual headings** |
| CSV / Excel exports | UTF-8 with BOM (Excel opens Sinhala correctly). Column headers in the chosen language, plus an optional stable-code header row so re-imports do not depend on language |
| Dates in reports | As in §6. Court dates are never shifted by timezone |
| Report definitions | Text such as titles and descriptions lives in catalogues. The query logic does not depend on language |

## 10. Notifications, email, SMS

| Channel | Language rule | Note |
|---|---|---|
| In-app | Recipient's UI language; data values (names) in recipient's preferred script with fallback | |
| Email | Recipient's language; subject lines short | UTF-8, Noto webfont fallback |
| SMS | Client's preferred language | **Sinhala SMS uses UCS-2 encoding: ~70 characters per segment vs 160 for English → ~2–3× cost.** Templates must be written short; cost per message is a business decision |
| WhatsApp (later) | Template messages must be pre-approved per language | |

## 11. Tamil readiness (what "architecture permits Tamil" means concretely)

| Layer | Built in MVP so Tamil needs no rework |
|---|---|
| Locale code | `ta-LK` registered; catalogues scaffolded with English fallback |
| Data | `name_ta` and `label_ta` fields exist on names, titles and reference data (empty for now) |
| Search | The normalisation and transliteration-key pipeline accepts a script plug-in, so Tamil normalisation and Tamil↔Latin/Sinhala name keys are added there |
| Fonts | Noto Sans Tamil and Noto Serif Tamil can be added to the same font stack and the PDF pipeline |
| Collation | ICU `ta-LK` collation available in PostgreSQL |
| Communication | Client preferred language accepts `ta`; template lookup falls back to English and flags it for staff |
| Glossary | The glossary data model has a Tamil column |
| UI | The language switcher is driven by the list of enabled locales, not hardcoded to two |

**Not done in MVP:** Tamil translation, Tamil OCR benchmark, Tamil legal glossary review.

## 12. Coverage check against PART 21

| PART 21 item | Section |
|---|---|
| English UI · Sinhala UI | §2 |
| Mixed Sinhala/English legal terminology | §2 dual-label mode, §3 glossary |
| Unicode storage | §4 |
| Sinhala search · English search | §5 |
| Sinhala names · English names · transliterated names | §4, §5, §6 |
| Sinhala OCR | §7 |
| Sinhala PDF generation | §9 |
| Notifications · emails | §10 |
| Reports | §9.1 |
| No hardcoded frontend text | §2 |
| Tamil later | §11 |

## 13. Testing

- Browsers: desktop Chrome/Edge/Firefox, Android Chrome, iOS Safari (Sinhala rendering differences).
- Long-word and narrow-screen tests; pseudo-localisation; screenshot regression in both languages.
- Search tests with ZWJ variants, legacy-converted text, mixed script queries, transliteration pairs.
- Sinhala-speaking staff acceptance test of every MVP workflow **entirely in Sinhala UI** (success criterion, doc 01).
