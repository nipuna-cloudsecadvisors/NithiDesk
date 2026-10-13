# NithiDesk — Planning Summary (Management Level)

> **Status:** Planning draft v0.2 (gap audit 2026-10-13) for review with a practising Sri Lankan Attorney-at-Law. **No screens or database migrations have been created.** An early Go scaffold (`cmd/`, `internal/`) exists; it is treated as a Phase 0 scaffold, not feature work (Gate G0). Detailed documents are in [`docs/`](docs/README.md).
> Markers used throughout: **[NEEDS LAWYER CONFIRMATION]** · **[MAKE CONFIGURABLE]** · **[ASSUMPTION]** · **[DECISION REQUIRED]**.

---

## 1. What NithiDesk is

NithiDesk is the **daily operating platform for a Sri Lankan law practice**. It connects every client, matter, court proceeding, hearing date, document, task and client update in one place, so that:

- no hearing date is missed or recorded wrongly,
- every lawyer (especially a junior covering at short notice) knows from a phone **what the case is, what happened last time, what is needed today and what happens next**,
- case history stays intact as matters move from the Magistrate's or District Court to the High Court, Court of Appeal and Supreme Court,
- confidential client information is seen only by the people who should see it,
- Sinhala- and English-speaking staff can work naturally, with Tamil possible later.

It is **not** a generic CRM, document store, task manager, accounting system or AI lawyer. Detail: [01-product-vision](docs/01-product-vision.md).

## 2. Who will use it

| Group | Main use |
|---|---|
| Managing partner / head of chambers | Firm-wide oversight, conflict and engagement approvals, risk |
| Partners / senior counsel | Portfolio supervision, approvals, key appearances |
| Senior & junior lawyers | Hearing preparation, **court-day mobile use**, recording outcomes |
| Court clerks | **Guardians of the diary** — verifying dates against case records and court lists, registry work |
| Legal assistants / admin staff | Intake, documents, client contact |
| Finance (Phase 2) | Fee tracking |
| System administrator | Users, configuration, backups — **no default access to client files** |
| External counsel (if needed) | Time-limited access to briefed matters |
| Clients (Phase 3) | Approved status, verified dates, shared documents |

Detail: [02-user-personas](docs/02-user-personas.md).

## 3. The main business problem

Today the firm's critical information is spread across a **physical court diary**, paper files, Excel sheets, Word documents, shared drives, email, WhatsApp and individual memory. The consequences are: hearing dates that live in one book or one person's head; next dates transcribed later from memory; case history fragmented when a matter is appealed; time lost hunting for documents before court; juniors walking into court without context; ad-hoc client updates; no disciplined conflict checks; no access control over sensitive matters.

The single most damaging failure is a **missed or wrongly recorded hearing**. The MVP is designed around preventing it.

## 4. The proposed workflow

```
Enquiry → Conflict check (human decision) → Engagement → Client onboarding → MATTER
   MATTER → Proceeding(s) [court, case number history, parties with roles]
          → Hearing [date + SOURCE + VERIFICATION] → Preparation → Court day (mobile)
          → Outcome → Next hearing (created from the order) → Tasks → Client update (approved)
          → Appeal / revision = NEW linked proceeding under the SAME matter
          → Settlement / judgment → Closure (checklist + partner approval) → Archive (retention, legal hold)
```

Key improvements to the original concept chain:
1. **One registry of people and organisations**; "client", "opposing party", "witness", "director" are *roles*. This is what makes conflict checking and search reliable.
2. **Matter ≠ proceeding.** A matter can hold many linked proceedings (appeal, leave to appeal, revision, transfer, execution) with every case number kept.
3. **Documents belong to the matter** and are linked to proceedings, hearings and tasks, rather than nested under hearings.
4. **Preparation, appearance and outcome are phases of one hearing record**; the outcome creates the next hearing.
5. **Hearing status split into two dimensions**: *listing status* (Scheduled, Held, Not reached, Court not sitting, Vacated, Cancelled, Outcome recorded) and *date verification* (Unverified, Verified, Conflict, plus a derived "verification overdue"). "Changed" and "postponed" become recorded events rather than statuses.
6. **Matter status split into lifecycle + phase + automatic attention flags**, instead of one overloaded status list.
7. **Not every matter is litigation.** Advisory and conveyancing matters have no proceedings.

Diagrams: [system map](docs/diagrams/system-map.md) · [client journey](docs/diagrams/client-journey.md) · [matter lifecycle](docs/diagrams/matter-lifecycle.md) · [information relationships](docs/diagrams/information-relationships.md).

## 5. Main system modules

| Module | Purpose |
|---|---|
| Identity & access | Login, MFA, sessions, roles, matter-level access, ethical walls |
| Audit & governance | Append-only audit trail, legal hold, retention, data-subject requests |
| Localisation & Sinhala text services | Bilingual UI, Sinhala normalisation, cross-script name matching, fonts, PDF |
| Configuration & court reference data | Courts, case types, case-number patterns, stages, roles, purposes, checklists |
| Person & organisation registry | One record per real-world entity, with relationships |
| Intake & conflict checking | Enquiries, conflict search, human decisions, engagement |
| Matters | The client's legal problem, team, lifecycle, related matters |
| Proceedings & parties | Court cases with case-number history, relationships, party roles |
| Hearings & court diary | Dates with provenance and verification, preparation, outcomes, court-list checks |
| Today / weekly planner / court-day mode | Role-specific daily work, mobile court use |
| Documents & evidence | Upload pipeline, versions, categories, evidence custody, originals register |
| Tasks & checklists | Follow-ups, deadlines, reviewers |
| Client communications | Calls, meetings, WhatsApp references, instructions, approved updates |
| Search | Grouped, permission-aware, Sinhala/English/cross-script |
| Notifications | Priority model, escalations, digests |
| Reports & dashboards | Role-specific |
| Import & migration | CSV dry-run imports, batches |
| *Later:* OCR, AI gateway, client portal, court connectors, fee tracking | |

## 6. MVP scope

> **The MVP replaces the physical court diary and the "where is that file?" problem for one firm, safely, bilingually and from a phone.**

MVP includes: secure foundation (MFA, matter-level access, ethical walls, audit, backups); Sinhala/English UI with dual-label mode; person registry and clients; intake with conflict search and human decision; matters; court master data; proceedings with case-number history, relationships and party roles; **hearings with source + verification, outcome → next hearing, court-list check-off, outcome-not-recorded queue**; Today, weekly planner and daily brief; **mobile court-day mode** with quick outcome capture that tolerates going offline; documents (secure upload, malware scanning, versions, previews); tasks; communication log with approved client updates; grouped global search; matter timeline; in-app and email notifications with escalation; basic reports; CSV migration with dry-run.

Detail and acceptance criteria: [17-mvp-scope](docs/17-mvp-scope.md).

## 7. What should NOT be in the MVP

AI features · client portal · OCR (benchmark Sinhala first) · offline document packs (MVP covers only the offline outcome queue) · billing/accounting · SMS/WhatsApp sending · government connectors and e-filing · a generic workflow engine · in-browser document editing · native apps · multi-firm SaaS features · advanced analytics.

## 8. Major risks

| Risk | Key controls |
|---|---|
| Wrong or missed hearing date | Date provenance, read-back confirmation, clerk verification queue, court-list check, escalation for unrecorded outcomes, careful migration with 100% verification of near dates |
| Low adoption (staff keep using paper or WhatsApp) | Court-first mobile UX, Sinhala usability, pilot champions, parallel run |
| Unauthorised access / ethical-wall bypass | Central server-side authorisation on every read path (incl. search, notifications, reports, AI), audit of every view and download, alerts |
| Incorrect conflict result | Multiple matching signals incl. cross-script and relationships; mandatory human decision; historical party data migrated |
| Sinhala text handling (zero-width joiners, legacy fonts, PDF shaping, OCR) | Normalisation layer, legacy-font conversion, HTML→PDF rendering, early technical spikes |
| Mobile device loss | Short sessions, minimal encrypted offline data that expires, remote revocation |
| Backup failure | Point-in-time recovery, immutable offsite copy, quarterly restore drills |
| Scope creep | MVP thesis, explicit exclusions, decision gates |

Full register (30 risks): [19-product-risk-register](docs/19-product-risk-register.md).

## 9. Decisions needed from the lawyer

**Must be answered before coding (Gate G0):**
1. How hearing dates are recorded and verified today, and what counts as authoritative confirmation.
2. Whether a date written down in court by the appearing lawyer counts as "verified".
3. The matter vs proceeding rule (when an appeal or revision stays in the same matter).
4. Who appears vs who holds the proxy; how often juniors cover; whether external counsel need logins.
5. Conflict-check practice, decision-makers and relevant relationships.
6. Courts, case types and volumes; example case numbers per court.
7. Mobile use and connectivity in court; device types.
8. Confidentiality expectations: restricted matters, walls, whether clerks see all cases.
9. Language preferences, the legal glossary, and whether Tamil is needed.
10. Hosting and data-residency constraints; PDPA and professional obligations; retention periods.
11. Whether billing is needed in the first release.
12. Whether Sinhala documents use Unicode or legacy fonts.

**Owner decisions (business):** single firm vs future multi-firm SaaS; hosting provider and location; budget and team; SMS/WhatsApp cost appetite; AI opt-in policy (later).

Full question bank: [20-lawyer-discovery-questions](docs/20-lawyer-discovery-questions.md).

## 10. Proposed development phases

| Phase | Content | Indicative duration |
|---|---|---|
| **0 — Discovery & design** | Lawyer sessions, observed court day and clerk day, ADRs, threat model, logical data model, API outline, Sinhala technical spikes, court reference data | 4–6 weeks |
| **MVP — M1** | Foundation + **Court Diary Core** | ~8 weeks |
| **MVP — M2** | Registry, intake, conflicts, full matters/proceedings/parties, walls | ~6 weeks |
| **MVP — M3** | Documents, tasks, communications, timeline | ~6 weeks |
| **MVP — M4** | Court-day polish, search, notifications, reports, closure | ~5 weeks |
| Migration and parallel run → **go-live** | Verification sprint; 4–8 weeks running alongside the paper diary | ~6–10 weeks (overlapping) |
| **Phase 2** | OCR, offline court packs, court-list import, SMS, full workflow templates, fee tracking, PDF exports, retention/disposal, SSO/passkeys | ~4 months |
| **Phase 3** | AI gateway and features, client portal, WhatsApp, drafting workflow, first court connectors | ~5 months |
| **Future** | E-filing, accounting integration, multi-firm SaaS, Tamil UI, native apps | — |

Roadmap and dependency diagram: [18-product-roadmap](docs/18-product-roadmap.md). **[ASSUMPTION]** 2–3 engineers + part-time design and QA; MVP is about 5–7 months.

## 11. Recommended architecture direction (not final; to be confirmed through ADRs in Phase 0)

| Concern | Direction | Why |
|---|---|---|
| Shape | **Modular monolith** in Go (one API binary + one worker binary), modules split along the boundaries in doc 08 | Small team, strong consistency needs; microservices would be premature |
| Backend | Go; lightweight router (chi-style); SQL-first data access (pgx + generated query code, e.g. sqlc); explicit transactions; no heavy ORM | Matches stated direction; safe, parameterised SQL; testable |
| Database | PostgreSQL: relational core, full-text search, **trigram** matching, ICU collations for Sinhala; pgvector later for semantic search | One database covers MVP search needs; avoid OpenSearch until scale requires it |
| Files | S3-compatible private object storage (MinIO-compatible locally; cloud or on-prem in production), random keys, short-lived signed URLs | Standard, portable |
| Background jobs | Queue abstraction; **prefer a Postgres-backed queue in MVP** (fewer moving parts) with Redis as an option if needed | Simplicity |
| Malware scanning | ClamAV-class scanner in the upload pipeline | Required for legal documents from untrusted parties |
| PDF generation | **HTML templates rendered by a headless browser engine** (correct Sinhala shaping) | Many Go PDF libraries render Sinhala incorrectly |
| OCR (Phase 2) | Provider interface; Tesseract (Sinhala + English) self-hosted first; cloud OCR opt-in only | Confidentiality; benchmark-driven |
| Frontend | **React + TypeScript SPA built with Vite, as an installable PWA** (recommended over Next.js: authenticated app, no SEO need, simpler offline/service-worker model); ICU-based i18n catalogues; self-hosted Noto Sinhala fonts; Tailwind-style design tokens | Mobile court mode and offline queue |
| Auth | Server-side sessions, TOTP MFA from day one, passkeys next; OIDC-ready for Microsoft Entra ID / Google later | Revocation, security |
| Authorisation | Central policy: role capability + matter ACL + record restriction − ethical wall, applied inside every query | Prevents leaks via search, reports and AI |
| Tenancy | **Tenant-aware data model (firm ID on every record), single-tenant deployment first** | Cheap now, expensive to add later |
| Integrations | Adapters for notifications, OCR, AI, court connectors behind interfaces | Swap providers without touching core code |
| Hosting | **[DECISION REQUIRED]** Sri Lanka-hosted vs regional cloud; architecture stays portable (containers, standard Postgres, S3 API) | PDPA, latency, trust |
| Observability | Structured logs without confidential content, metrics, health/readiness endpoints | Operations |

**Decisions to make before coding, because they are hard to reverse:** single entity registry; matter ≠ proceeding graph; case-number history; hearing date assertions with derived verification; local-date type for court dates; tenant ID on all records; central authorisation design; bilingual name fields plus normalised search shadows.

## 12. Estimated complexity of each module

| Module | Complexity | Main difficulty |
|---|---|---|
| Identity, MFA, sessions | Medium | Getting MFA, recovery and session revocation right |
| Authorisation (RBAC + matter ACL + walls) | **High** | Correctness on *every* read path; testing |
| Audit & governance | Medium | Volume; tamper evidence; retention |
| Localisation & Sinhala text services | **High** | Normalisation, cross-script matching, legacy fonts, PDF shaping |
| Court reference data & configuration | Medium | Data gathering; versioned templates without a workflow engine |
| Person/organisation registry | Medium–High | Dedup/merge/unmerge; relationships |
| Intake & conflict checking | **High** | Matching quality across scripts; controlled disclosure |
| Matters | Medium | Lifecycle, team history, closure rules |
| Proceedings & parties | Medium–High | Relationship graph; case-number history; role transformations on appeal |
| Hearings & court diary | **High** | Provenance and verification model; clash detection; outcome→next chain; court-list matching |
| Today / weekly / daily brief | Medium | Performance and clear presentation |
| Court-day mobile mode + offline outcome queue | **High** | Usability under pressure; sync conflicts |
| Documents & evidence pipeline | Medium–High | Secure upload pipeline; previews; versioning |
| Tasks & checklists | Low–Medium | Templates; escalation |
| Client communications | Low–Medium | Approval flow |
| Search (grouped, permission-aware) | Medium–High | Sinhala relevance; permission filtering at speed |
| Notifications | Medium | Priority, escalation, anti-noise |
| Reports & dashboards | Low–Medium | Permission-filtered aggregates |
| Import & migration tooling | Medium | Validation, dry-run, legacy fonts |
| *Phase 2+:* OCR | Medium–High | Sinhala accuracy |
| *Phase 2+:* Offline court pack | **High** | Encryption, expiry, device loss |
| *Phase 3:* AI gateway | **High** | Permission-safe retrieval, citations, evaluation |
| *Phase 3:* Client portal | Medium–High | Separate auth realm, security surface |
| *Phase 3+:* Court connectors | Unknown | Depends on government availability |

## 13. Dependencies between modules

```
Foundation (auth · access · audit · i18n · config)
   ├─► Court reference data ──────────────┐
   └─► Person/org registry ─► Intake & conflicts ─► Matters ─► Proceedings & parties ─► Hearings & diary ─► Today / weekly / court-day
                                                     │              │                     │
                                                     ├─► Documents ◄┴─────────────────────┤
                                                     ├─► Tasks ◄──────────────────────────┤
                                                     └─► Communications                   └─► Notifications
   Documents + Registry + Proceedings ─► Search ─► (Phase 2) OCR ─► (Phase 3) AI
   Hearings ─► (Phase 3+) Court connectors · Communications + Documents ─► (Phase 3) Client portal
```

Authorisation and localisation sit under *everything*. They are built first and cannot be added later. Diagram: [18-product-roadmap §3](docs/18-product-roadmap.md).

## 14. Recommended first development milestone

### **M1: "Foundation + Court Diary Core"**

**Goal:** a secure, bilingual court diary that the clerk and lawyers could rely on by itself, even before the rest of the product exists.

**Scope:**
- Repository, CI with quality gates (tests, lint, authorisation tests, secret scanning), environments, automated backups plus a restore test.
- Login with MFA, sessions and device list, role templates, basic matter-level access, audit framework.
- Localisation framework (Sinhala/English, dual-label), Sinhala fonts, CI check for hardcoded strings.
- Court reference data for the courts the firm attends.
- Thin matter and proceeding records (title, client name, team, court, case-number history, main parties).
- Hearings: date assertions with source, verification state, listing status, conflict state, outcome → next hearing.
- Today view (desktop and mobile), weekly view, clerk verification queue, outcome-not-recorded queue.
- CSV import (dry-run) of the firm's current diary and active cases.
- Daily brief in-app plus a **bilingual printable PDF**, which retires the Sinhala PDF risk early.

**Why first:** it attacks the highest-impact risk (missed or wrong dates), delivers visible daily value, lets the firm load real data early, and forces the hardest foundations (authorisation, audit, Sinhala handling, local-date semantics) to be solved up front.

**Pre-conditions:** Gate G0 answers from the lawyer, approved ADRs, and the Phase 0 Sinhala spikes completed.
