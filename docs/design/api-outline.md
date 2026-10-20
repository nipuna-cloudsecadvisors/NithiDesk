# API Outline — M1 (draft)

> Status: **DRAFT for Phase 0 review** — refs issue #29. Resource-oriented HTTP + JSON API for milestone M1 "Foundation + Court Diary Core" (doc 18). Entities and invariants are defined in [logical-data-model.md](logical-data-model.md).
> This is an outline, not an OpenAPI contract. The contract is written when M1 starts, after Gate G0.
> Markers: **[NEEDS LAWYER CONFIRMATION]** · **[MAKE CONFIGURABLE]** · **[ASSUMPTION]** · **[DECISION REQUIRED]**.

---

## 1. Consistency with the existing scaffold

The repository already has an API scaffold (`cmd/api`, `internal/httpapi`, `internal/authz`). This outline keeps to it:

| Scaffold fact | How this outline uses it |
|---|---|
| Go standard library `http.ServeMux` with method patterns (`GET /healthz`) | Routes below are written in the same `METHOD /path/{param}` form, which `ServeMux` supports directly. Doc summary §11 suggested a chi-style router; **no router library is needed** for this surface — note for ADR #17 |
| Operational routes `GET /healthz`, `GET /readyz`, `GET /version` at the root, unauthenticated | Kept unversioned and outside `/api/v1` |
| `X-Request-Id` generated server-side; client values not trusted | Every response and every error carries it; audit events store it |
| Error body `{"error": "internal_error"}` — a code, never a dependency message | Extended in §2.3 to `{"error": {"code": …}}`. **Divergence:** the scaffold returns a bare string; switching to the object form is a small change to `writeJSON` callers and should be done before product routes exist |
| `SecurityHeaders`: `nosniff`, `no-referrer`, `Cache-Control: no-store` | Applies to all API responses. The daily-brief PDF also uses `no-store` and `Content-Disposition: attachment` |
| Access log records path, never query string (a search term can be a client name) | Search and filter values may therefore go in query strings, but they must never be logged |
| `internal/authz`: one central policy; invisible → **404**, never 403 | §2.6 |
| `RequiredReadPathScenarios` lists read paths that must be tested | Every list/queue endpoint below maps to a scenario; new ones must be added there |

The scaffold uses "Release 1 / Foundation 1–2" and an external spec reference ("CLO-2"); the planning pack uses "M1". **[DECISION REQUIRED]** pick one milestone vocabulary.

---

## 2. Conventions

### 2.1 Versioning and media

- Base path `/api/v1`. Breaking changes → `/api/v2`; additive changes (new fields, new endpoints) stay in v1. Clients must ignore unknown fields.
- `Content-Type: application/json; charset=utf-8`. All text is UTF-8, NFC-normalised by the server on write.
- Field names `snake_case`. IDs are opaque strings (UUIDv7 recommended, see the logical model §1.2).
- **Court dates are `"YYYY-MM-DD"` strings** (local dates, no time zone). Instants are RFC 3339 UTC (`2026-10-13T04:30:00Z`). The two are never mixed.
- Bilingual names are objects: `"title": {"en": "Perera v. Silva", "si": "පෙරේරා එදිරිව සිල්වා"}`. The server never chooses a language for data; the client displays per the user's locale with fallback.

### 2.2 Localisation rule

The API returns **codes, never localised sentences** (doc 11). Enumerations (`listing_status: "not_reached"`), validation errors and warnings are codes with parameters; the frontend's message catalogue renders them in Sinhala or English. Reference-data names come back as `text_i18n` objects.

### 2.3 Errors

```json
{
  "error": {
    "code": "hearing.outcome_requires_next_or_reason",
    "request_id": "9f1c…",
    "fields": [ { "field": "no_next_reason", "code": "required_without", "params": { "other": "next_hearing" } } ]
  }
}
```

| HTTP | When | Example codes |
|---|---|---|
| 400 | Malformed JSON, wrong types | `request.malformed` |
| 401 | No/expired session | `auth.session_required`, `auth.session_expired` |
| 401 | MFA or step-up needed | `auth.mfa_required`, `auth.step_up_required` |
| 404 | Not found **or not visible** | `not_found` (one code for both — §2.6) |
| 403 | Visible, but the role lacks the capability (e.g. junior tries to verify where policy forbids) | `forbidden.capability` |
| 409 | Business-rule conflict | `hearing.date_conflict`, `wall.member_excluded`, `case_number.duplicate` (when blocking) |
| 412 | `If-Match` stale | `precondition.version_mismatch` |
| 422 | Valid JSON, failed validation | `validation.failed` with `fields[]` |
| 428 | Missing `If-Match` / `Idempotency-Key` where required | `precondition.required` |
| 429 | Rate limit (login, PIN, search) | `rate_limited` |
| 500 | Anything else | `internal_error` (scaffold) |

Warnings that do not block (e.g. a duplicate case number the firm may legitimately have) come back in a successful response as `"warnings": [ { "code": "case_number.possible_duplicate", "params": { "matter_number": "2026/LIT/0142" } } ]`. A warning must never reveal a matter the caller cannot see; in that case the code is `case_number.possible_duplicate_hidden` without parameters, and the clerk escalates. **[NEEDS LAWYER CONFIRMATION]** whether even that is acceptable across a wall.

### 2.4 Pagination, filtering, sorting

- **Cursor (keyset) pagination**: `?limit=50&cursor=<opaque>`; response `{"items": [...], "next_cursor": "…" | null}`. No offsets, no total counts by default (counts are expensive and are a leak vector across walls — when a count is shown it is computed through the same authorised query).
- Max `limit` 200. Sort orders are fixed per endpoint (e.g. diary: `hearing_date, court, session, id`).

### 2.5 Concurrency and idempotency

- **Optimistic concurrency:** every mutable resource returns `ETag: "<row_version>"`. `PATCH`/`PUT`/`DELETE` require `If-Match`; a stale version → 412 with the current representation's ETag. Append-only actions (assert a date, verify, record an outcome) don't need `If-Match` because they never overwrite — they rely on business rules (I-HE-4) instead.
- **Idempotency keys:** `Idempotency-Key: <uuid>` is **required** on `POST /hearings/{id}/outcome`, `POST /hearings/{id}/date-assertions`, `POST /hearings/{id}/notes` and `POST /imports/{id}/commit`, and accepted on any other POST. Keys are scoped to (firm, user), stored ≥ 7 days **[ASSUMPTION]**. Replay with the same body → the original response (status + body). Same key with a different body → 422 `idempotency.key_reused`. This is what makes the mobile offline queue safe to retry (doc 12).
- Client-generated IDs: the offline queue may send `"id"` for the next hearing it creates, so a retried request cannot create two hearings. **[DECISION REQUIRED]** — only possible with UUID keys.

### 2.6 Authorisation (server-side, every endpoint)

- Every handler resolves the caller from the session, then calls `internal/authz` before touching data; list queries apply the authorised-matter filter **inside the SQL**, not by filtering results afterwards.
- A resource the caller cannot see returns **404 `not_found`** — identical to a resource that doesn't exist (no timing or message difference worth exploiting).
- Lists, counts, queues, Today/weekly, search, PDFs and audit queries all go through the same filter. Each read path must have a `RequiredReadPathScenarios` entry.
- Field-level restriction: restricted fields are **omitted**, not nulled-with-a-flag, unless the UI needs a placeholder (e.g. `"title_masked": true` for a Restricted matter in clerk diary scope, doc 09 §3).
- System administrators get configuration and security endpoints only; matter endpoints return 404 for them (scaffold scenario `administrator_sees_no_matter_content`).
- Every write and every sensitive read emits an audit event in the same transaction.

### 2.7 Sessions and CSRF

- Server-side sessions; opaque token in a `Secure; HttpOnly; SameSite=Lax` cookie `nd_session` **[ASSUMPTION]** (Lax so links from emails work; state change protected by CSRF token).
- State-changing requests carry `X-CSRF-Token` (double-submit, bound to the session).
- No bearer tokens for the browser app. Service accounts for connectors are Phase 3+.

---

## 3. Endpoints — M1

Capability names (e.g. `hearing.verify`) refer to role-template capabilities in doc 09 §6.

### 3.1 Authentication, session, MFA

| Method & path | Purpose | Notes |
|---|---|---|
| `POST /api/v1/auth/login` | Email + password → partial session (`mfa_pending`) | Rate-limited per account and IP; same error for unknown email and wrong password (`auth.invalid_credentials`) |
| `POST /api/v1/auth/mfa/verify` | TOTP code → full session | |
| `POST /api/v1/auth/mfa/totp/enroll` · `…/confirm` | First-login enrolment (QR secret, confirm with a code) | Mandatory before any other endpoint works |
| `POST /api/v1/auth/step-up` | Re-authenticate for sensitive actions | Sets `step_up_at` for 5 min **[MAKE CONFIGURABLE]** |
| `POST /api/v1/auth/logout` | End session | |
| `POST /api/v1/auth/pin` · `POST /api/v1/auth/unlock` | Set / use the mobile quick-unlock PIN | 5 failures → session revoked (doc 10 §4) |
| `GET /api/v1/me` | Current user, roles, capabilities, locale, firm settings | The UI hides buttons from this, but the server still enforces |
| `PATCH /api/v1/me` | Preferred locale, display options | |
| `GET /api/v1/me/sessions` · `DELETE /api/v1/me/sessions/{id}` | Device list, revoke | |
| `POST /api/v1/admin/users` · `PATCH /api/v1/admin/users/{id}` · `POST /api/v1/admin/users/{id}/reset-mfa` | User administration | Admin capability + step-up; reset notifies user |

### 3.2 Reference data

| Method & path | Purpose | Notes |
|---|---|---|
| `GET /api/v1/reference/{kind}` | `kind` ∈ `court-levels`, `courts`, `courtrooms`, `proceeding-types`, `party-role-types`, `hearing-purposes`, `outcome-types`, `relationship-types`, `date-source-types`, `practice-areas`, `court-calendar` | Cacheable per firm (`ETag` + `If-None-Match`); every authenticated user |
| `POST /api/v1/reference/{kind}` · `PATCH /api/v1/reference/{kind}/{id}` | Maintain (`config.manage`) | End-dating instead of delete |
| `POST /api/v1/reference/courts/{id}/case-number-check` | Normalise and validate a case number against the court's pattern | Used by forms and the importer |

### 3.3 Matters (thin in M1)

| Method & path | Purpose |
|---|---|
| `GET /api/v1/matters?q=&status=&lawyer=&cursor=` | List authorised matters |
| `POST /api/v1/matters` | Create (title, practice area, client entity or new thin entity, team, confidentiality) |
| `GET /api/v1/matters/{id}` | Matter with team, proceedings summary, attention flags |
| `PATCH /api/v1/matters/{id}` | Controlled fields (`If-Match`) |
| `GET /api/v1/matters/{id}/members` · `POST …/members` · `DELETE …/members/{memberId}` | Team; adding a walled user → 409 `wall.member_excluded` |
| `POST /api/v1/matters/{id}/confidentiality` | Change level (partner + step-up; audited) |
| `GET /api/v1/entities?q=` · `POST /api/v1/entities` | Thin registry lookup/create for clients and parties (full registry M2) |
| `POST /api/v1/ethical-walls` · `GET /api/v1/ethical-walls/{id}` · `POST /api/v1/ethical-walls/{id}/removal` | Walls (managing partner/`wall.manage`, step-up); removal needs a second approver |

### 3.4 Proceedings, case numbers and parties

| Method & path | Purpose |
|---|---|
| `POST /api/v1/matters/{id}/proceedings` | Create (type, court, optional first case number, our side/role, main parties) |
| `GET /api/v1/proceedings/{id}` | With current + historical case numbers, parties, relationships, next hearing |
| `PATCH /api/v1/proceedings/{id}` | Controlled fields (`If-Match`) |
| `GET /api/v1/proceedings/{id}/case-numbers` | Full history |
| `POST /api/v1/proceedings/{id}/case-numbers` | Add a number: `{court_id, raw, reason: assigned|corrected|renumbered|transferred, valid_from}`; ends the current one in the same transaction; duplicate → warning |
| `POST /api/v1/proceedings/{id}/relationships` | Typed link (M1: `appeal_of`, `transferred_from`); cycle → 409 `proceeding.relationship_cycle` |
| `POST /api/v1/proceedings/{id}/parties` · `PATCH …/parties/{partyId}` | Main parties (entity, role type, ordinal, side) |
| `DELETE /api/v1/proceedings/{id}` | Soft-delete "created in error" — reason + partner capability |
| `GET /api/v1/case-numbers/lookup?court=&number=` | Find a proceeding by **any** current or past number (authorised) |

### 3.5 Hearings and the diary core

| Method & path | Purpose | Notes |
|---|---|---|
| `POST /api/v1/proceedings/{id}/hearings` | Create with date, session, purpose, appearing lawyers, **source of the date** | Creates the hearing **and** its first date assertion in one transaction |
| `GET /api/v1/hearings/{id}` | Full hearing: current date, listing status, verification state, assertion history, verifications, previous hearing + outcome, appearances | |
| `PATCH /api/v1/hearings/{id}` | Non-date fields only: purpose, courtroom, session, instructions, notes, clerk, preparation status (`If-Match`) | Date changes are **rejected** here (`hearing.date_change_requires_assertion`) |
| `POST /api/v1/hearings/{id}/date-assertions` | "The date was recorded wrongly": `{date, session, source_type, source_detail, evidence_document_id?}` | `Idempotency-Key` required. Contradicting a verified date → hearing becomes `conflict`; response shows the new state |
| `POST /api/v1/hearings/{id}/vacate` | "The court changed the date": `{reason, replacement: {date, session, source_type, source_detail}}` | Sets `vacated` and creates the replacement hearing linked by `replaces_hearing_id` |
| `POST /api/v1/hearings/{id}/verifications` | Verify the **current** assertion: `{date_assertion_id, authoritative_source_type, evidence_reference}` | `hearing.verify` capability. 409 `verification.assertion_not_current` if someone asserted a new date meanwhile. Non-authoritative source → 422 unless firm policy allows |
| `POST /api/v1/hearings/{id}/verifications/{vid}/revoke` | Revoke with reason | Audited |
| `POST /api/v1/hearings/{id}/listing-status` | `not_reached`, `court_not_sitting`, `held`, `cancelled` (+ reason) | State machine of doc 06 §4.2; illegal transition → 409 `hearing.illegal_transition` |
| `PUT /api/v1/hearings/{id}/outcome` | Record the outcome: `{outcome_type, summary, order_text?, next_hearing: {id?, date, session, purpose, source_detail} | null, no_next_reason?, client_recorded_at}` | **`Idempotency-Key` required.** Atomic (I-OU-2). Exactly one of `next_hearing` / `no_next_reason` (I-OU-1). Replays return the original. A later correction uses `POST …/outcome/corrections` |
| `POST /api/v1/hearings/{id}/appearances` · `DELETE …/appearances/{userId}` | Appearing lawyers | Walled user → 409; in M1, a non-member must first be added to the team (doc 09 §2.2) |
| `POST /api/v1/hearings/{id}/notes` | Quick court-day note | `Idempotency-Key` required (offline queue) |

### 3.6 Views and queues (read paths)

All are **authorised read paths** with an entry in `RequiredReadPathScenarios`.

| Method & path | Purpose | Scenario |
|---|---|---|
| `GET /api/v1/diary/today?date=YYYY-MM-DD&scope=mine|team|court` | Today: hearings grouped by court, with last outcome, purpose, instructions, badges | `today_view_excludes_walled_matters` |
| `GET /api/v1/diary/week?start=YYYY-MM-DD&lawyer=&court=` | Weekly planner: hearings, prep status, unverified dates, same-lawyer clashes (two courts, same session) | `weekly_view_excludes_walled_matters` |
| `GET /api/v1/queues/verification?court=&within_days=` | Clerk verification queue: unverified, overdue and conflicting dates, nearest first | `verification_queue_excludes_walled_matters` |
| `GET /api/v1/queues/outcome-not-recorded` | Past hearings still `scheduled` | `outcome_not_recorded_queue_excludes_walled_matters` |
| `GET /api/v1/diary/court-list-check?court=&date=` · `POST …/court-list-check` | Manual court-list tick-off (M1): returns our hearings at that court/date; posting the list of numbers seen returns matched / missing / unexpected / different-date | Each different-date match creates an `official_court_list` assertion (→ conflict). Needs a scenario entry (**to add**) |
| `GET /api/v1/search?q=` | M1: matters and proceedings by title, party name or **any** case number. Full grouped search is M4 | `restricted_matter_absent_from_search` |

### 3.7 Daily brief

| Method & path | Purpose |
|---|---|
| `GET /api/v1/briefs/daily?date=&scope=` | JSON for the in-app brief |
| `GET /api/v1/briefs/daily.pdf?date=&scope=&lang=si|en|both` | Bilingual printable PDF (next two days), rendered server-side (ADR #23); `Content-Disposition: attachment`; download audited; watermark with user and time **[ASSUMPTION]** |

### 3.8 CSV import (dry-run → commit)

| Method & path | Purpose |
|---|---|
| `POST /api/v1/imports` (multipart: `kind`, `file`) | Upload; file goes through the upload pipeline (size, type, malware scan) |
| `POST /api/v1/imports/{id}/validate` | Dry run: per-row `ok/warning/error` with codes; nothing is written to business tables |
| `GET /api/v1/imports/{id}` · `GET /api/v1/imports/{id}/rows?status=` | Report and row detail |
| `POST /api/v1/imports/{id}/commit` | `Idempotency-Key` + `If-Match` (batch version). Only `validated` batches; creates records with `migrated` date assertions (→ unverified) |
| `POST /api/v1/imports/{id}/rollback` | Soft-delete the batch's records if none were edited since; otherwise 409 `import.rollback_blocked` with the blocking records |

Imports require `import.run` and step-up; they are audited per batch and per created record.

### 3.9 Audit query

| Method & path | Purpose |
|---|---|
| `GET /api/v1/audit/events?matter=&actor=&action=&from=&to=&cursor=` | Business events — managing partner (`audit.read_business`), filtered by the caller's matter access and walls; **viewing is itself audited** |
| `GET /api/v1/audit/security-events?…` | Authentication and admin events — system administrator (`audit.read_security`); matter IDs shown, no matter content (doc 09 change) |
| `GET /api/v1/matters/{id}/audit` | Matter-level trail for the responsible partner |

There is no write, update or delete endpoint for audit (scenario `audit_records_cannot_be_modified`).

### 3.10 Operational (existing, unchanged)

`GET /healthz`, `GET /readyz`, `GET /version` — unauthenticated, no business data.

---

## 4. Mobile offline queue contract (M1 scope)

Doc 12: court-day mode tolerates going offline for **outcomes and notes only**.

1. The phone stores the pending action (method, path, body, `Idempotency-Key`, `client_recorded_at`) encrypted in IndexedDB and shows it as "Waiting to send".
2. On reconnect it replays in order. 2xx or a replayed 2xx → done. 409/412/422 → shown to the user to resolve; **never** retried automatically. 401 → re-unlock, then retry; queued items survive session expiry (doc 12).
3. The server treats `client_recorded_at` as information only; ordering and audit use server time.

---

## 5. Open questions

| # | Question | Marker |
|---|---|---|
| A1 | Error envelope object vs the scaffold's bare string (§1) | **[DECISION REQUIRED]** — recommend object form now |
| A2 | Client-generated IDs for the offline queue (§2.5) | **[DECISION REQUIRED]** with ADR #17 |
| A3 | Duplicate-case-number warning across an ethical wall (§2.3) | **[NEEDS LAWYER CONFIRMATION]** |
| A4 | Idempotency-key retention period | **[ASSUMPTION]** 7 days |
| A5 | Cookie `SameSite=Lax` vs `Strict` | **[DECISION REQUIRED]** ADR #20 |
| A6 | Milestone vocabulary: "Release 1 / Foundation" (scaffold) vs "M1" (planning pack) | **[DECISION REQUIRED]** |
| A7 | Whether a JSON:API/OpenAPI-first workflow (generated server stubs) is used | ADR #17 |
