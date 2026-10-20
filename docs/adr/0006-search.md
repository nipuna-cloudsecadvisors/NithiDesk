# 0006. Search

- **Status:** Proposed. **The decision awaits the results of spike #25** (Sinhala normalisation + cross-script name matching).
- **Date:** 2026-10-13
- **Issue:** #22
- **Deciders:** product owner (approves); lead engineer (proposes)
- **Blocked on:** spike **#25**
- **Related:** [doc 07 §8–9](../07-document-workflow.md), [doc 11 §4–5](../11-localization-plan.md), [doc 04 §3 conflict checking](../04-legal-matter-workflow.md), [ADR 0001](0001-backend-stack-and-module-layout.md)

## Context

Search serves three critical jobs:
1. **Conflict checking** across Sinhala, English and transliterated names. A missed match is a professional-conduct risk.
2. **Global search** by case number, NIC, phone, party, client or document title, with grouped results.
3. **Document search:** metadata in MVP, full text later, OCR in Phase 2, semantic in Phase 3.

Every search must be **permission-filtered inside the query** (doc 07 §8.3): restricted matters must never show up in results, snippets or counts. Sinhala has no reliable open stemmer (doc 11 §5); matching relies on Unicode normalisation (NFC, ZWJ/ZWNJ), trigrams and transliteration keys. Volumes for one firm are modest: thousands of matters and tens of thousands of documents (**[ASSUMPTION]**, confirm with lawyer Q6 / #7).

## Decision drivers

- Permission filtering in the same query (no post-filtering leaks).
- Correct Sinhala handling (normalisation, cross-script).
- Few moving parts; consistency with the transactional data.
- Good-enough relevance at single-firm scale.
- A path to OCR text and embeddings later.

## Considered options

1. **PostgreSQL only:**
   - normalised shadow columns; `pg_trgm` trigram indexes;
   - full-text search with the `simple` configuration and custom normalisation done in Go;
   - transliteration-key columns; `pgvector` later.
2. **Dedicated search engine** (OpenSearch/Elasticsearch) fed from Postgres.
3. **Lightweight search engine** (Meilisearch/Typesense) fed from Postgres.

### Option 1: PostgreSQL only
- Good: the permission filter is just a `JOIN`/`WHERE` on the accessible-matter set, in the same transaction, with no sync lag. One system to back up. Trigram matching is script-agnostic and works on normalised Sinhala.
- Bad: relevance tuning is more manual; heavy full-text search at large scale is slower than a dedicated engine.

### Option 2: OpenSearch/Elasticsearch
- Good: rich relevance and analysers; scales.
- Bad: permission filtering must be copied into the index (ACL fields) and kept in sync, which is a known leak risk when walls change. Heavy to operate. No first-class Sinhala analyser either, so we would still need custom normalisation.

### Option 3: Meilisearch/Typesense
- Good: easy typo-tolerant search.
- Bad: same ACL-sync problem; weaker fit for complex permission filters; another service to run.

## Decision

**Provisionally option 1, to be confirmed by spike #25.**
- **Write path:** normalisation in Go (`internal/platform/i18n`). Each searchable name or title is stored as entered, plus:
  - `*_norm`: NFC, with zero-width characters handled and vowel signs ordered;
  - `*_translit_key`: a phonetic key from Sinhala → Latin and Latin → key;
  - entries for curated name equivalents (Perera ↔ පෙරේරා, Fernando ↔ ප්‍රනාන්දු).
- **Indexes:**
  - GIN `pg_trgm` on the normalised columns;
  - B-tree on exact keys (normalised case number, NIC blind index, phone in E.164);
  - `tsvector` with the `simple` configuration on titles and notes.
- **Queries** are built through a single search service that always joins the caller's accessible-matter set (from `internal/authz`). Results are grouped by type, and counts come from the same filtered query.
- **Conflict search** uses the same machinery with lower thresholds and shows *why* each candidate matched (exact, trigram score, transliteration, relationship). A human always decides.
- **Revisit** a dedicated engine only if measured latency or relevance fails at real volume, and only with an ACL-sync design reviewed against the walls requirements.

## Consequences

- Positive: no permission-sync leak path; one datastore; Sinhala handling under our control.
- Negative: transliteration keys and normalisation are custom code that needs a good test corpus. Relevance tuning is manual.
- Follow-up: the corpus and metrics from #25 become a regression suite in CI.

## Confirmation

Spike #25 must report, on a sample of anonymised or synthetic Sri Lankan names:
- recall of cross-script matches (target ≥ 95% for curated common names; exact target **[DECISION REQUIRED]**),
- precision at the conflict threshold,
- p95 query latency at 10× expected volume.

Authorisation tests: a restricted matter is absent from results, snippets and counts.

## Open questions

- Who curates the name-equivalence dictionary, and how? **[NEEDS LAWYER CONFIRMATION]** (clerk input)
- Should search query text be logged, and for how long? **[MAKE CONFIGURABLE]** (doc 07 §8.3)
- Do legacy Sinhala fonts (non-Unicode) appear in data to migrate? This affects the normalisation scope (doc 11 §8). **[NEEDS LAWYER CONFIRMATION]**
