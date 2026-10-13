# Spike: Sinhala normalisation + cross-script name matching (issue #25)

**Throwaway code.** This is a Phase 0 experiment allowed under Gate G0 (docs/18 §4). It is not part of
the product, is not imported by `cmd/` or `internal/`, and must not be copied into production as-is.
The production implementation will be Go + PostgreSQL; this prototype is Python (stdlib only) because
neither was available when the spike ran.

| File | Purpose |
|---|---|
| `sinhala.py` | Unicode normaliser (NFC, ZWJ/ZWNJ, invisible chars, legacy typing order) and Sinhala → Latin transliteration |
| `match.py` | Phonetic key, consonant skeleton, pg_trgm-compatible trigram similarity, scorers V1–V5 |
| `pairs.py` | 77 synthetic labelled pairs (fabricated names, no real clients) |
| `evaluate.py` | Precision/recall table per scorer and threshold |

Run:

```
python -I -X utf8 evaluate.py            # summary table
python -I -X utf8 evaluate.py --detail   # every pair with its score and key
```

Results and recommendation: [`docs/spikes/25-sinhala-name-matching.md`](../../docs/spikes/25-sinhala-name-matching.md).
