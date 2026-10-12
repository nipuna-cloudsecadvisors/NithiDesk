# 17 — MVP Scope

Covers PART 31 (MVP), PART 28 (reporting), PART 34 (workflow configuration).

---

## 1. MVP thesis

> **The MVP replaces the physical court diary and the "where is that file?" problem for one firm — safely, bilingually, and from a phone.**

If the MVP does only one thing exceptionally well, it must be: **every upcoming hearing is in NithiDesk, with its source and verification state, the right lawyer knows about it with full context, and the outcome and next date are captured the same day.**

Everything in MVP must serve daily use by lawyers and clerks. Anything that is only occasionally useful, or that depends on data the firm doesn't yet have in the system, waits.

**[ASSUMPTION]** Single firm, single tenant deployment (tenant-aware data model). One pilot firm of ~3–25 lawyers.

---

## 2. Classification

### 2.1 MVP (Release 1 — usable every day)

| Area | MVP capabilities |
|---|---|
| **Foundation** | Users, MFA (TOTP), sessions & device list, role templates, matter-level access, confidentiality levels, ethical walls (basic: exclude users from matter), audit log (all events in doc 10 §9 relevant to MVP features), bilingual UI (Sinhala/English + dual-label), backups with tested restore |
| **Registry & clients** | Entity registry (individual/company/government/organisation), bilingual names + variants, NIC/passport/reg no. (masked, encrypted), contacts, addresses, relationships (basic types), duplicate warnings, merge (privileged) |
| **Intake & conflicts** | Enquiry, conflict search (identifiers, phone, normalised names, trigram, cross-script name keys + seeded surname dictionary, 1-hop relationships), match classification, human decision with reason, engagement decision |
| **Matters** | Matter creation from enquiry, multi-client, team & roles with history, lifecycle status + phase + derived flags, related matters, physical file location, closure checklist & approval |
| **Courts & proceedings** | Court master data (seeded), proceeding types (configurable lists, not full templates), case-number history, proceeding relationships (appeal/revision/transfer/related), party roles per proceeding with ordinal and side, representation (opposing attorney) |
| **Hearings & diary** | Hearing records, date assertions with source, verification state, listing status, conflict state, preparation status + simple checklist templates, outcome recording → next hearing, outcome-not-recorded queue, court-list manual check-off, day/week views (by lawyer/court), schedule-clash detection (same lawyer same session), court calendar (holidays/Poya days) warnings |
| **Today & weekly** | Personal Today view, daily brief (in-app + email), weekly planner with highlights |
| **Court-day mobile** | Mobile Today, case card, quick outcome with read-back confirmation, notes, photo-to-PDF upload, quick task, request client contact, **offline-tolerant outcome/note queue** |
| **Documents** | Upload (web + camera), categories, matter ownership + links to proceeding/hearing/task, versioning, filed-version pin, confidentiality, previews, signed URLs, malware scan, hash, originals register (simple), soft-delete |
| **Tasks** | Tasks linked to matter/proceeding/hearing/document, owner, due date, hard-deadline flag, priority, statuses, reviewer, reminders; tasks from outcomes |
| **Communications** | Log calls/meetings/WhatsApp references/letters/emails (manual), client instructions, client update drafts with approval → mark sent (sent via firm's own channels) |
| **Search** | Global grouped search: case numbers (incl. historical), matter no., names (EN/SI + cross-script), NIC (authorised), phone, document titles/metadata; permission-filtered |
| **Timeline** | Auto-generated matter timeline (key events), filter by proceeding |
| **Notifications** | In-app + email; priority model; escalation for unverified dates & unrecorded outcomes; digests |
| **Reports (basic)** | Upcoming hearings, unverified dates, outcomes not recorded, open matters by lawyer/court/practice area, overdue tasks |
| **Migration** | CSV import (clients, matters, proceedings, parties, hearings) with dry-run & error report; import batches |
| **Retention basics** | Retention class per matter category (recorded), legal hold flag blocking deletion |

### 2.2 Phase 2 (strengthen daily use; 3–6 months after MVP go-live)

- **OCR** (Sinhala + English, self-hosted, benchmarked) and full-text content search.
- Encrypted **offline court pack** with documents.
- Court-list import (CSV/PDF) with matching; email-in to matter.
- Web push; **SMS** notifications; iCal feed.
- Full **workflow templates** per proceeding type (stages, purpose catalogue, checklists, document checklists, deadline-rule suggestions).
- Document requests to clients; secure expiring share links.
- Appearance-scoped access automation; break-glass workflow; external counsel accounts (if confirmed needed).
- **Lightweight fee tracking** (fee arrangement, appearance-fee log, fee notes/receipts) — **[DECISION REQUIRED]**.
- Bilingual PDF exports (briefs, chronology, reports).
- Expanded reports (matter aging, workload, document activity, hearings by month).
- Retention/disposal workflow; data-subject request handling.
- Legacy Sinhala font conversion tooling at scale; archive import (class D).
- Passkeys/WebAuthn; OIDC SSO.

### 2.3 Phase 3 (differentiate)

- **AI gateway** + hearing brief narrative, order summarisation, information extraction, client-update drafting, permission-aware semantic search, conflict-match assistance.
- **Client portal** (status, verified dates, shared documents, requests, messages).
- WhatsApp Business API notifications (client-facing).
- Drafting workflow (draft → review → approved → filed) and precedents library.
- First **court connector** adapters (if official sources exist).

### 2.4 Future

- E-filing integration; filing/payment status.
- Full accounting/trust accounting integration.
- Multi-firm SaaS operations (tenant onboarding, billing).
- Tamil UI and Tamil OCR (earlier if pilot needs it).
- Native mobile apps (only if PWA limits bite — e.g. iOS storage/push).
- Office/Google Docs live editing integration.
- Analytics/benchmarks across firms (opt-in, anonymised).

---

## 3. What should NOT be in MVP (and why)

| Excluded | Reason |
|---|---|
| AI features | Needs trusted data and OCR first; adds risk & cost; firm opt-in decisions pending |
| Client portal | Internal data must be reliable first; large security surface; low demand evidence |
| OCR | Sinhala accuracy unknown — benchmark first; MVP still searches metadata/titles |
| Offline document packs | Security complexity; MVP covers the critical write-path offline (outcome queue) |
| Billing / accounting | Not the core pain; scope creep risk |
| SMS / WhatsApp sending | Cost, provider contracts, template approvals; email + in-app suffice to start |
| Government connectors / e-filing | Availability unknown; manual workflows first |
| Full configurable workflow engine | Lists + simple templates are enough; a generic engine is a trap |
| Document editing in browser | Download/edit/upload versioning is adequate |
| Native apps | PWA is sufficient for MVP |
| Multi-firm SaaS features | Single firm first (but tenant-aware model) |
| Advanced analytics | Basic operational reports first |

---

## 4. MVP acceptance criteria (pilot)

1. All active matters and **all future hearings** migrated; 100% of hearings in the next 14 days verified before cut-over.
2. Lawyers record ≥ 90% of outcomes on the hearing day, median ≤ 2 minutes on mobile.
3. Zero hearings missing from NithiDesk vs. the paper diary for 2 consecutive weeks (parallel run).
4. Sinhala-preferring staff complete onboarding, hearing, outcome and task workflows in the Sinhala UI without help.
5. Authorisation test suite passes (incl. ethical-wall bypass attempts); external penetration test with no high/critical findings open.
6. Restore from backup demonstrated.

---

## 5. MVP size estimate (rough)

**[ASSUMPTION]** A small team (2–3 engineers + part-time designer + part-time QA, with Claude Code assistance) → MVP in roughly **5–7 months** including foundation, migration tooling and a 4–8 week parallel run. Highly sensitive to: number of courts/proceeding types configured up front, Sinhala search quality bar, and pilot-firm availability for reviews.

---

## 6. Reporting (PART 28)

| Report | Audience | Phase |
|---|---|---|
| Upcoming hearings (day/week/month; by lawyer/court) | All lawyers, clerks, partners | MVP |
| Unverified dates (by window, court, clerk) | Clerks, partners | MVP |
| Outcomes not recorded | Partners, lead lawyers | MVP |
| Open matters (by lawyer, court, practice area, phase) | Partners | MVP |
| Overdue tasks (by owner, hard deadlines highlighted) | Partners, lawyers | MVP |
| Cases by court / by case type | Managing partner | MVP (simple) |
| Workload per lawyer (hearings/week, open matters, open tasks) | Managing partner | Phase 2 |
| Matter aging (time since open, since last activity, per stage) | Partners | Phase 2 |
| Closed matters (period, outcome) | Managing partner | Phase 2 |
| Hearings by month (volume, adjournment rate) | Managing partner | Phase 2 |
| Document activity (uploads, downloads by user — security-sensitive) | Managing partner, admin | Phase 2 |
| Client activity (communications, updates sent) | Partners | Phase 2 |
| Conflict checks (volume, decisions, turnaround) | Managing partner | Phase 2 |
| Restricted-matter access review | Partners | Phase 2 |
| Fee/appearance reports | Finance, partners | Phase 2 (if billing) |
| Verification performance (avg lead time, % verified by T-2) | Managing partner, clerks | Phase 2 |

All reports permission-filtered; exports audited.

---

## 7. Workflow configuration map (PART 34)

| Area | Varies by | Configurable item | MVP form |
|---|---|---|---|
| Courts | Court | Court list, divisions, courtrooms, sitting patterns, calendar | Admin screens / seed data |
| Case numbering | Court, case type | Patterns, display masks, normalisation | Patterns (warn only) |
| Proceeding types | Court level, practice area | Types, allowed courts, party roles, stages, purposes, outcome types | Lists per type |
| Relationships | Court hierarchy | Proceeding relationship types & allowed pairs | List |
| Party roles | Proceeding type | Role catalogue, compound appellate roles | List |
| Hearing purposes / outcomes | Proceeding type | Catalogues | Lists |
| Preparation checklists | Purpose × type | Templates, mandatory items | Simple templates |
| Document categories | Firm | Two-level taxonomy, default confidentiality | List |
| Matter numbering & naming | Firm | Pattern, title convention | Setting |
| Matter categories / practice areas | Firm | Lists | Lists |
| Statuses | Firm | Phase labels; lifecycle fixed | Phase list |
| Approvals | Firm | Who approves conflicts, engagement, client updates, closure; one- or two-level | Settings |
| Verification policy | Firm, court, priority | Who may verify; window; whether lawyer-in-court counts as verified | Settings |
| Access defaults | Firm | Partner firm-wide read; clerk scope; restricted masking | Settings |
| Notifications | Firm, user | Priorities, escalation timings, digest times, quiet/court hours | Settings |
| Retention | Matter category, document category | Retention classes | Settings |
| Deadline rules | Proceeding type | Suggested deadlines (never binding) | Phase 2 |
| Languages | Firm, user | Default language, dual-label, enabled languages | Settings |
| AI | Firm, matter level | Enabled features, provider | Phase 3 |

**Rule:** configuration is declarative data with versioning and audit; no scripting; changes to templates don't silently alter existing records.
