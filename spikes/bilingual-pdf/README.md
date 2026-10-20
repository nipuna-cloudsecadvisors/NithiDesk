# Spike #26 — Bilingual PDF rendering (THROWAWAY)

> **Throwaway spike code (Gate G0, docs/18 §4).** Not production code. Don't import it from `cmd/` or `internal/`. Results and the recommendation are in [docs/spikes/26-bilingual-pdf.md](../../docs/spikes/26-bilingual-pdf.md).

This spike generates a fictitious bilingual (Sinhala/English) daily court brief as A4 HTML in several font variants. It prints each variant to PDF with headless Chrome and Edge.

## Run

1. Download the fonts into a directory of their own. The fonts are OFL-licensed.
   - **Variable fonts:** `NotoSansSinhala.ttf` and `NotoSerifSinhala.ttf`, from `github.com/google/fonts` (folders `ofl/notosanssinhala` and `ofl/notoserifsinhala`). The downloaded files are named `…[wdth,wght].ttf`; rename them to the names above.
   - **Static fonts:** `NotoSansSinhala-Regular.ttf` and `NotoSansSinhala-Bold.ttf`, from `notofonts/notofonts.github.io` (folder `fonts/NotoSansSinhala/hinted/ttf/`).
2. Run `python build.py <fonts_dir> out`.

The script needs only the Python standard library. It expects Chrome and Edge at their default Windows paths; edit `ENGINES` if yours differ.

## Output (committed)

- `out/brief-<font>-chrome.pdf` for each font variant.
- `out/brief-<font>.html`, the source pages.
- `out/strings.txt`, the shaping test strings used for the text-extraction check.

Edge produces near-identical output because it uses the same Chromium/Skia/HarfBuzz pipeline, so its PDFs aren't committed.

All names, case numbers and the firm name are fabricated.
