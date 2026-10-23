# NithiDesk

The daily operating platform for a Sri Lankan law practice: a verified court diary, matters with their full proceeding history across courts, and a court-day mobile mode — in Sinhala and English.

- Product and planning: [docs/README.md](docs/README.md), management summary: [NITHIDESK_PLANNING_SUMMARY.md](NITHIDESK_PLANNING_SUMMARY.md)
- Architecture decisions: [docs/adr/](docs/adr/README.md) · data model: [docs/design/logical-data-model.md](docs/design/logical-data-model.md) · API: [docs/design/api-outline.md](docs/design/api-outline.md) · threat model: [docs/security/threat-model.md](docs/security/threat-model.md)

> **Status: M1 "Foundation + Court Diary Core" — built ahead of the lawyer's Gate G0 answers.** Every Sri Lankan practice rule that is not yet confirmed lives in configuration (reference data, firm settings, role templates) so the answers change data, not code. See [M1 decisions](#m1-decisions-taken-before-g0) below.

## What M1 does

| Area | In M1 |
|---|---|
| Sign-in | Password (argon2id) + mandatory TOTP, recovery codes, server-side sessions (idle 30 min desktop / 15 min mobile, 12 h absolute), mobile PIN unlock, step-up for sensitive actions, device list and revocation, account lockout |
| Access | Role templates (configurable) + matter team + confidentiality level − ethical wall, decided in **one** database function (`nd_visible_matters()`) that every list, queue, count and search joins; firm isolation by PostgreSQL row-level security on every table; invisible = 404 |
| Audit | Append-only, hash-chained per firm, written in the same transaction as the change; `nithidesk verify-audit` checks the chain |
| Courts | Seeded court levels, courts, proceeding types, party roles, hearing purposes, outcome types, relationship types, date-source types with authority — all editable data; court calendar (holidays, Poya, vacations); case-number normalisation and pattern warnings |
| Matters | Thin matters with clients, team (one responsible partner, one lead), confidentiality, matter numbering (`{YYYY}/{SEQ:4}`, firm setting), attention flags |
| Proceedings | Case-number **history** (transfers keep old numbers), appeal/revision/transfer links with cycle checks, parties with roles and ordinals, duplicate-number warnings that never reveal a hidden matter |
| Hearings | Dates as append-only **assertions with source and authority**; verification against an authoritative source; derived state *unverified / verified / conflict*; listing status state machine; vacate → linked replacement; outcome → next hearing in one transaction; idempotent outcome/note writes for the offline queue |
| Views | Today, weekly planner (clashes, overdue verification, not ready, no appearing lawyer), clerk verification queue, outcome-not-recorded queue, court-list tick-off, search by matter number / any case number / title / party name in either script |
| Sinhala | Unicode normalisation (ZWJ, visual-order kombuva, decomposed vowel signs), transliteration and phonetic keys so "පෙරේරා" finds "Perera" (spike #25 port, regression-tested) |
| Import | Diary CSV: upload → dry-run report → commit (imported dates are unverified) → rollback while untouched |
| Brief | In-app daily brief and a bilingual printable PDF rendered by headless Chromium with bundled static Noto fonts (ADR 0007) |
| Web | React + TypeScript PWA in `web/` (Sinhala/English, dual-label mode, court-day mode, offline outcome/note queue) |

## Run it locally (Windows, no Docker)

Prerequisites: Go 1.27, Node 24 + pnpm, Chrome or Edge. PostgreSQL 18 runs from a portable copy under `.data/` (git-ignored).

```bash
# 1. PostgreSQL (portable binaries in .data/pgsql, cluster in .data/pgdata, port 55432)
.data/pgsql/bin/pg_ctl.exe -D .data/pgdata -l .data/pg.log -w start

# 2. Environment for this shell (.data/dev.env holds local-only secrets)
set -a; . .data/dev.env; set +a

# 3. Schema, runtime login, a demo firm with synthetic data
go run ./cmd/nithidesk migrate
go run ./cmd/nithidesk create-app-login -name nithidesk_api
go run ./cmd/nithidesk create-firm -code demo -name-en "Demo Chambers" -name-si "ආදර්ශ නීති කාර්යාලය"
go run ./cmd/nithidesk seed-demo -firm demo      # prints demo users and the shared demo TOTP secret

# 4. API (serves the built web app too when NITHIDESK_WEB_DIR is set) and worker
(cd web && pnpm install && pnpm build)
NITHIDESK_WEB_DIR=web/dist go run ./cmd/api
go run ./cmd/worker                              # renders PDF briefs
```

Open http://localhost:8080, sign in as e.g. `junior@demo.nithidesk.lk` with `NITHIDESK_USER_PASSWORD` from `.data/dev.env`, and the code from an authenticator app holding the printed demo TOTP secret. For frontend work, `cd web && pnpm dev` proxies `/api` to the Go API.

A first `.data/dev.env` is created by hand from this template (values are local-only; generate your own):

```
NITHIDESK_ENV=development
NITHIDESK_MIGRATE_DATABASE_URL=postgres://postgres:<superuser-password>@localhost:55432/nithidesk
NITHIDESK_DATABASE_URL=postgres://nithidesk_api:<app-password>@localhost:55432/nithidesk
NITHIDESK_APP_DB_PASSWORD=<app-password>
NITHIDESK_SECRETS_KEY=<32 random bytes, base64>
NITHIDESK_DEFAULT_FIRM=demo
NITHIDESK_COOKIE_SECURE=false
NITHIDESK_OBJECTSTORE_DRIVER=fs
NITHIDESK_OBJECTSTORE_FS_ROOT=<absolute path>/.data/objects
NITHIDESK_CHROME_PATH="C:/Program Files/Google/Chrome/Application/chrome.exe"
NITHIDESK_USER_PASSWORD=<12+ characters, for seed-demo users>
```

## Tests

```bash
go test ./...                                                     # unit tests
NITHIDESK_TEST_DATABASE_URL="$NITHIDESK_MIGRATE_DATABASE_URL" go test ./internal/itest/ -count=1 -v
(cd web && pnpm test && pnpm lint:i18n)
```

`internal/itest` creates a throw-away database per run, applies the real migrations, starts the real router and signs users in with password + TOTP. It is the **authorisation merge gate**: it fails if any scenario in `internal/authz.RequiredReadPathScenarios` has no test, and it proves walls, confidentiality, clerk diary scope, administrator blindness, firm isolation (RLS), CSRF, append-only audit, the hearing verification rules, idempotent outcome replay and import dry-run/commit/rollback.

## Layout

```
cmd/api, cmd/worker      the two binaries (ADR 0001)
cmd/nithidesk            operator tool: migrate, create-app-login, create-firm, create-user, verify-audit, seed-demo
migrations/              versioned SQL, embedded; never edit an applied file
internal/authz           the access rule and role templates (every context calls it)
internal/identity        users, MFA, sessions
internal/courts          reference data and case numbers
internal/registry        people and organisations (thin in M1)
internal/matters         matters, team, ethical walls
internal/proceedings     proceedings, case-number history, links, parties
internal/hearings        the diary: assertions, verification, outcomes, views, court-list check
internal/imports         diary CSV import
internal/briefs          daily brief and PDF
internal/audit           hash-chained audit log
internal/platform/*      db (tenant transactions), migrate, textnorm (Sinhala), apperr, clock, secretbox, objectstore
internal/httpapi         HTTP transport only
internal/itest           end-to-end and authorisation suite
web/                     React PWA
```

## M1 decisions taken before G0

These were open in the ADRs; M1 needed an answer, so each is the reversible default. They are recorded here for review.

| Question | M1 choice | Where to change |
|---|---|---|
| Keys (LDM Q1) | UUIDv7 (`uuidv7()`, PostgreSQL 18) | — |
| RLS on `firm_id` (LDM Q2) | On, every table; app role never owns tables | migrations |
| Data access (ADR 0001) | Hand-written SQL with pgx; no sqlc yet (queries are dynamic around the visibility CTE) | revisit after M1 |
| Managing partner scope (LDM Q3) | Team member only (`matter_scope = member`) | role template `managing_partner` |
| Lawyer-in-court date = verified? (Q4) | No: "announced in court" is *primary*; only *authoritative* sources verify unless firm setting `verification_allow_primary` is true | `firm.settings` |
| Verification window | 3 working days | `firm.settings.verification_window_days` |
| Duplicate case number (Q5) | Warning; hidden matters are never named | — |
| Transfer (Q6) | Same proceeding, new number at the new court; old number kept | — |
| Matter number format (Q7) | `{YYYY}/{SEQ:4}` | `firm.settings.matter_number_format` |
| Appearance by a non-member | Must join the team first (appearance-scoped access is Phase 2) | — |
| Error envelope (A1) | `{"error": {"code", "request_id", "params", "fields"}}` | — |
| Cookie (A5) | `SameSite=Lax`, HttpOnly, Secure outside development, plus CSRF token header | — |
| Milestone names (A6) | "M1" | — |

Not in M1 (by plan): intake and conflict checks, documents, tasks, communications, notifications, client portal, OCR, AI, connectors.
