# 0007. PDF generation with correct Sinhala shaping

- **Status:** Proposed. **The decision awaits the results of spike #26** (bilingual PDF rendering).
- **Date:** 2026-10-13
- **Issue:** #23
- **Deciders:** product owner (approves); lead engineer (proposes)
- **Blocked on:** spike **#26**
- **Related:** [doc 11 §9](../11-localization-plan.md), [doc 17](../17-mvp-scope.md) (daily printable diary in MVP), [doc 19](../19-product-risk-register.md) (continuity fallback), [ADR 0003](0003-hosting-and-deployment.md)

## Context

M1 includes a **bilingual printable daily diary/brief PDF**: the continuity fallback if the system or connectivity fails on a court day. Later phases add letters, chronologies and reports. Sinhala needs complex text shaping: conjuncts, vowel signs placed before or around consonants, and ZWJ forms such as yansaya and rakaransaya. Libraries without a shaping engine produce visibly broken text, which is unacceptable on a document a lawyer relies on in court. Fonts: Noto Sans/Serif Sinhala (SIL Open Font License), embedded.

## Decision drivers

- **Correct Sinhala rendering** (non-negotiable), plus English and later Tamil.
- Reuse the same HTML/CSS templates and localisation catalogues as the UI.
- Runs server-side in the worker; no data sent to third-party services (confidentiality).
- Acceptable resource use and operational simplicity.

## Considered options

1. **HTML templates → headless Chromium** (driven from Go via the DevTools protocol, e.g. `chromedp`, or a sidecar such as Gotenberg).
2. **Pure-Go PDF with a HarfBuzz-class shaper** (e.g. `go-text/typesetting` for shaping, plus a PDF writer).
3. **Another typesetting engine as a sidecar** (e.g. WeasyPrint, which uses HarfBuzz for shaping; or Typst).
4. **Basic Go PDF libraries without complex shaping** (e.g. gofpdf-style).

### Option 1: headless Chromium
- Good: Chromium's text stack (HarfBuzz) shapes Sinhala correctly; one template language with the UI; mature CSS print support (page breaks, headers, footers).
- Bad: heavy runtime (≈ hundreds of MB image, high memory per render); one more process to secure. It must be sandboxed and never fetch remote URLs.

### Option 2: pure Go + shaper
- Good: single static binary; low resource use.
- Bad: we would have to build layout ourselves (line breaking, tables, pagination). Combining shaper and PDF writer is less mature, so it is a high engineering cost for a small team.

### Option 3: WeasyPrint / Typst sidecar
- Good: lighter than Chromium; strong print-CSS (WeasyPrint) or typographic quality (Typst).
- Bad: another language runtime and container. Sinhala shaping quality must be proven, not assumed (**[ASSUMPTION]** unverified for Typst).

### Option 4: basic Go PDF library
- Good: trivial.
- Bad: **renders Sinhala incorrectly**, because there is no complex shaping. Rejected.

## Decision

**Provisionally option 1 (HTML → headless Chromium in the worker container), to be confirmed by spike #26.** Option 3 (WeasyPrint) is the fallback if Chromium's footprint is a problem on the chosen hosting.
- **Templates:** `html/template` with the same message catalogues as the UI. Fonts are embedded locally: Noto Sinhala, Noto Sans and later Noto Tamil.
- **Rendering:** happens in the worker with **no network access** (request interception blocks every non-`data:`/local URL) and runs as a low-privilege user.
- **Output:** PDFs are stored through ADR 0005 and audited as exports (doc 10 §8).

## Consequences

- Positive: correct shaping; one template system; the risk is retired in M1 as planned.
- Negative: a larger worker image and memory use, so PDF jobs need a concurrency limit.
- Follow-up: the M1 daily-brief epic builds on the spike result.

## Confirmation

Spike #26 must show a sample daily brief that has:
- the Sinhala test strings rendered correctly (conjuncts, ZWJ forms, mixed Sinhala-English lines), checked by a Sinhala reader;
- correct page breaks for a 40-hearing day;
- render time and peak memory measured.

Later, the PDF golden tests compare text extraction and a visual snapshot in CI.

## Open questions

- Is a Sinhala-reading reviewer available to sign off the spike output? **[NEEDS LAWYER CONFIRMATION]**
- Do courts or clients expect particular formats (e.g. A4 vs legal size, letterhead)? **[NEEDS LAWYER CONFIRMATION]**
