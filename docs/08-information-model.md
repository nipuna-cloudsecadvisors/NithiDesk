# 08 — Information Model (Conceptual)

Covers PART 30. **Conceptual only — not physical tables, not migrations.** Entity names are domain concepts; the physical schema will be designed after lawyer review.

Diagram: [diagrams/information-relationships.md](diagrams/information-relationships.md).

---

## 1. Bounded contexts (module boundaries)

| Context | Owns | Key relationships out |
|---|---|---|
| **Identity & Access** | User, Role template, Session, MFA factor, Matter membership, Ethical wall, Access grant | Matter, Audit |
| **Registry** | Entity (Person / Organisation / Government body), Entity relationship, Identifier, Contact point, Address, Name variant | Client, Party, Conflict |
| **Intake** | Enquiry, Conflict check, Conflict match, Conflict decision, Engagement decision | Entity, Matter |
| **Matters** | Matter, Matter-client link, Matter team member, Matter link, Matter status event | Proceeding, Document, Task, Communication |
| **Courts (reference)** | Court level, Court, Division, Courtroom, Court calendar, Proceeding type template, Stage catalogue, Party-role catalogue, Hearing purpose, Outcome type, Relationship type | Proceeding, Hearing |
| **Proceedings** | Proceeding, Case-number record, Proceeding relationship, Party role, Stage event, Representation | Hearing, Evidence |
| **Hearings** | Hearing, Date assertion, Verification, Preparation checklist, Outcome, Court-list import batch | Task, Document, Notification |
| **Documents** | Document, Document version, Document link, Evidence item, Custody event, Original (physical) record, OCR result | Matter, Proceeding, Hearing |
| **Work** | Task, Task dependency, Checklist template, Reminder | Anything (polymorphic link) |
| **Communications** | Communication (call, meeting, letter, email, WhatsApp ref, update, instruction), Client update draft/approval | Matter, Entity |
| **Notifications** | Notification, Notification preference, Delivery attempt | User |
| **Governance** | Audit event, Legal hold, Retention class, Disposal record, Data-subject request, Configuration | Everything (by reference) |
| **AI (later)** | AI request, AI output, Citation, Human decision on suggestion | Document, Matter |
| **Integration (later)** | Connector, External reference, Import batch, Sync record | Court, Proceeding, Hearing |

---

## 2. Core relationships (cardinalities)

| Relationship | Cardinality | Notes |
|---|---|---|
| Entity — Entity relationship — Entity | M:N (typed, dated) | spouse, director-of, shareholder-of… |
| Entity — Client relationship | 1 : 0..1 | Client = entity with firm relationship (status, onboarding info) |
| Enquiry — Entity | M:N | prospective clients, other parties |
| Enquiry — Conflict check | 1 : 1..N | re-runs allowed; each frozen |
| Conflict check — Conflict match | 1 : N | match → Entity |
| Conflict check — Conflict decision | 1 : 1..N | appended; latest effective |
| Enquiry — Matter | 1 : 0..1 | accepted enquiry creates matter |
| Matter — Client (via Matter-client link) | M:N | ≥1 client per matter; role per link |
| Matter — Team member (User) | M:N | role + effective dates |
| Matter — Matter | M:N | typed related-matter links |
| Matter — Proceeding | 1 : 0..N | proceeding belongs to exactly one matter |
| Proceeding — Court | N : 1 (current) | history via case-number records |
| Proceeding — Case-number record | 1 : 0..N | history; current derived |
| Proceeding — Proceeding | M:N (typed) | appeal_of, revision_of, transferred_from… |
| Proceeding — Party role — Entity | M:N | role, ordinal, side, status per proceeding |
| Party role — Representation — Entity (lawyer) | M:N | attorney on record / counsel, dated |
| Proceeding — Hearing | 1 : 0..N | |
| Hearing — Hearing (previous/next) | 1 : 0..1 each way | chain; also "replaces" for vacated |
| Hearing — Date assertion | 1 : 1..N | append-only |
| Hearing — Verification | 1 : 0..N | append-only; current derived |
| Hearing — Outcome | 1 : 0..1 (+ corrections) | |
| Hearing — User (appearing) | M:N | |
| Matter — Document | 1 : N | ownership |
| Document — Version | 1 : 1..N | |
| Document — (Proceeding / Hearing / Task / Evidence / Party / Communication) | M:N via Document link | |
| Proceeding — Evidence item | 1 : N | |
| Evidence item — Document | M:N | |
| Evidence item — Custody event | 1 : N | append-only |
| Task — anchor (Matter, or firm) | N : 1 | plus optional M:N links |
| Task — Task | M:N (depends on) | |
| Communication — Matter | N : 1 (or entity-only for pre-matter) | |
| Communication — Entity (participants) | M:N | |
| Notification — User | N : 1 | refers to a source record |
| Audit event — (any record) | N : 1 (by reference, no FK enforcement needed) | append-only |
| Legal hold — (Client / Matter / Proceeding / Document) | M:N | |
| Ethical wall — Matter, Users | M:N both sides | |

---

## 3. Key modelling decisions (with rationale)

| # | Decision | Rationale | Reversible? |
|---|---|---|---|
| D1 | Single Entity registry for clients, parties, witnesses, lawyers, companies | Conflict checking, dedup, search | Hard to reverse later → **decide now** |
| D2 | Matter ≠ Proceeding; proceeding graph | Core domain principle | Hard to reverse → **decide now** |
| D3 | Case number is a history, not a field | Transfers, corrections, search | Hard to reverse |
| D4 | Hearing date as append-only assertions + derived verification | Provenance, no silent overwrite | Medium |
| D5 | Listing status and verification state are separate | See doc 06 §4 | Medium |
| D6 | Documents owned by matter, linked many-to-many | Re-use across hearings/proceedings | Medium |
| D7 | Party roles per proceeding | Roles change between courts | Hard to reverse |
| D8 | Bilingual text stored as parallel fields for *names/titles* (EN, SI, TA) and as *single free text* for notes (any script) | Names need both scripts; notes are written once in whatever language | Medium |
| D9 | Reference data (courts, types, stages, roles, purposes) as versioned configuration | Sri Lankan variability | Easy |
| D10 | Every record carries `firm_id` (tenant) even if single-firm initially | Keeps multi-firm SaaS possible without rework | Hard to add later → **decide now** (cheap to include) |
| D11 | Soft-delete + audit; purge only via retention workflow | Legal records, legal hold | Medium |
| D12 | Local dates for hearings/deadlines; UTC timestamps for events | Court dates must not shift | Hard to reverse |

---

## 4. Lifecycle ownership

| Record | Created by | Can be edited? | Can be deleted? |
|---|---|---|---|
| Entity | Intake/any authorised | Yes (audited); merge/unmerge privileged | Only via retention; never if referenced by a held record |
| Conflict check/decision | Intake / approver | **No** (append new) | No |
| Matter | From enquiry | Yes, controlled fields | No (archive/dispose) |
| Proceeding | Lawyer/clerk | Yes (case numbers append) | Soft-delete if created in error (reason, partner) |
| Hearing | Lawyer/clerk/import/outcome | Fields yes; dates via assertions only | Cancel (reason) — not delete |
| Outcome | Appearing lawyer | Correction event | No |
| Document version | Uploader | **No** | Soft-delete; purge via retention |
| Evidence/custody | Lawyer | Append | No |
| Task | Anyone with matter access | Yes | Cancel |
| Communication | Lawyer/staff | Edit within short window, then correction | No |
| Audit event | System | **Never** | Only by retention policy for audit logs |

---

## 5. Derived / computed views (not stored as truth)

- Matter attention flags (unverified upcoming, overdue tasks, missing docs, inactivity).
- Current case number, current court, current stage.
- Current hearing date and verification state.
- Timeline (projection of events).
- Daily brief and weekly planner.
- Workload metrics.

These may be cached/materialised for speed but are always rebuildable from source records.

---

## 6. Open model questions (→ doc 20)

- Can a single proceeding belong to two matters (e.g. firm acts for two clients with separate engagements in one case)? **Proposed: no** — use two matters each with its own proceeding record cross-linked by `same_case_as`. **[NEEDS LAWYER CONFIRMATION]**
- Do we model judges? (Useful for reports, sensitive.) **[DECISION REQUIRED]**
- Do we need "matter subject" entities (land parcels, vehicles, properties) for conflict and search? **Proposed: yes, light** (name, identifiers, address).
