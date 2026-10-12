# 19 — Product Risk Register

Covers PART 35. Likelihood (L) and Impact (I): 1 (low) – 5 (high). Score = L × I, before controls.

## 1. Product & domain risks

| ID | Risk | L | I | Score | Preventive controls | Detective controls | Owner |
|---|---|---|---|---|---|---|---|
| R1 | **Wrong hearing date recorded** (mis-tap, transcription, wrong case) | 4 | 5 | 20 | Date provenance; read-back confirmation ("Tuesday 24 November"); holiday/weekend/clash warnings; dates default Unverified; verified dates can't be silently overwritten | Clerk verification queue; court-list check-off ("on list / not on list"); conflict state when sources disagree; verification-overdue escalation | Clerk / lead lawyer |
| R2 | **Missed hearing** (date never entered, nobody assigned, lawyer unaware) | 3 | 5 | 15 | Outcome form forces next date or explicit "no next date" reason; hearing must have appearing lawyer by T-3; morning brief; migration verification | "Outcome not recorded" queue & escalation; court list shows unexpected listing; proceedings with no future hearing and not Dormant/Concluded flagged | Lead lawyer / partner |
| R3 | **Wrong case number** | 3 | 4 | 12 | Pattern validation per court/type (warn); duplicate (court+number) check; copy from documents; case-number history | Court-list mismatch; search for similar numbers; periodic data-quality report | Lawyer |
| R4 | **Unauthorised document access** | 3 | 5 | 15 | Matter ACL; confidentiality levels; ethical walls; server-side authorisation in every query; signed short-lived URLs; download permissions | Full view/download audit; anomaly alerts (volume, walled access attempts); monthly restricted-matter access review | Managing partner / admin |
| R5 | **Incorrect conflict result** (missed match or false clearance) | 3 | 5 | 15 | Multi-signal matching incl. cross-script & relationships; mandatory human decision; conflict data from closed matters migrated; incremental re-checks on new parties | Sampled audit of decisions; re-check when parties added; report of matters opened without conflict check (migrated) | Managing partner |
| R6 | **Missing document at hearing** | 4 | 3 | 12 | Hearing document checklist; "required but missing" placeholders; preparation status; originals register | T-3 missing-document alert; Today badges | Lead lawyer / assistant |
| R7 | **Duplicate client/entity** | 4 | 3 | 12 | Identifier exact-match block; fuzzy warnings at creation; dictionary-based cross-script matching | Duplicate-candidate report; merge queue | Admin / assistant |
| R8 | **Wrong merge of two different people** (common names) | 3 | 4 | 12 | Merge only by privileged users with side-by-side comparison; identifiers must not conflict | Unmerge capability; merge audit | Admin |
| R9 | **Duplicate case/proceeding** | 3 | 3 | 9 | Court+normalised number uniqueness warning | Data-quality report | Clerk |
| R10 | **Lost or altered evidence** | 2 | 5 | 10 | Immutable evidence records; custody log; hashes on digital evidence; no hard delete; legal hold | Custody report; hash verification job | Lead lawyer |
| R11 | **AI hallucination** relied upon | 3 | 4 | 12 | AI off in MVP; suggestions only; citations required; "not found" behaviour; AI cannot write verified data; labelled output | Human accept/reject audit; evaluation sets; user feedback flag | Product owner |
| R12 | **Incorrect Sinhala OCR** leading to missed search results or wrong extraction | 4 | 3 | 12 | Benchmark before adoption; confidence stored; original always available; OCR never authoritative; metadata search independent of OCR | Low-confidence page flags; spot checks | Product owner |
| R13 | **Mobile device loss** | 3 | 4 | 12 | Short sessions; WebAuthn/PIN re-unlock; minimal offline data, encrypted, expiring; no downloads to device storage for restricted docs | Device/session list; remote revocation; offline pack audit | User / admin |
| R14 | **Failed backup / unrecoverable data** | 2 | 5 | 10 | Automated backups (DB PITR + object versioning), offsite immutable copy, encrypted, keys escrowed | Backup success monitoring (P1 alert); quarterly restore drills | Admin |
| R15 | **Low adoption** — lawyers keep using paper/WhatsApp | 4 | 4 | 16 | Court-day UX first; fast outcome capture; Sinhala UI; pilot champions; parallel run; training | Usage metrics (outcomes recorded same-day, logins); feedback sessions | Managing partner / product owner |
| R16 | **Data-entry burden** too high for clerks | 4 | 3 | 12 | Minimal mandatory fields; carry-forward (outcome → next hearing); CSV import; later court-list import & extraction | Time-to-complete metrics; clerk feedback | Product owner |
| R17 | **Over-generic design** that ignores Sri Lankan practice (US-style statuses, roles) | 3 | 4 | 12 | Lawyer review gates; configurable catalogues; observed court-day walkthroughs | Pilot feedback; doc 20 tracking | Product owner |
| R18 | **Scope creep** (billing, portal, AI pulled into MVP) | 4 | 3 | 12 | MVP thesis; explicit exclusions; decision gates | Milestone reviews | Owner |
| R19 | **Sinhala rendering/search defects** (ZWJ variants, legacy fonts, PDF shaping) | 4 | 3 | 12 | Normalisation layer; legacy conversion; HTML→PDF rendering; early spikes | Bilingual test suite; Sinhala staff acceptance tests | Engineering |
| R20 | **Regulatory non-compliance** (PDPA, professional rules, data residency) | 3 | 4 | 12 | Purpose limitation; retention & legal hold; DSR workflow; hosting decision documented; legal review | Annual compliance review; processor register | Managing partner |
| R21 | **Insider bulk exfiltration** (departing staff) | 2 | 5 | 10 | Download permissions; bulk export approvals; watermarking; immediate offboarding | Download volume alerts; offboarding checklist audit | Admin |
| R22 | **Supplier lock-in / outage** (cloud, email, AI) | 2 | 3 | 6 | Provider interfaces; standard formats; export capability | Uptime monitoring | Engineering |
| R23 | **Government integration never materialises or changes** | 4 | 2 | 8 | Product fully functional without connectors; adapter architecture | Periodic landscape review | Product owner |
| R24 | **Malware via uploaded documents** | 3 | 4 | 12 | Quarantine, scanning, MIME checks, isolated preview rendering, macro blocking | Scan-failure alerts; security audit events | Admin |
| R25 | **Wrong client-facing update** (wrong date/case sent to client) | 3 | 4 | 12 | Only verified dates in templates by default; approval workflow; preview with recipient + matter shown | Communication log; client correction process | Lead lawyer |
| R26 | **Timezone/date-shift bug** moving court dates by a day | 2 | 5 | 10 | Local-date type for court events; tests around midnight; no UTC conversion for date-only values | Automated tests; reconciliation reports | Engineering |
| R27 | **Ethical wall bypass** via search, notification, report or AI | 2 | 5 | 10 | Wall check in central policy used by all read paths; tests for each path | Denied-access alerts; periodic wall audit | Engineering / MP |

## 2. Top 5 risks to manage first

1. **R1/R2 — wrong or missed hearings** → verification model, court-list check, outcome-not-recorded escalation, careful migration (M1 focus).
2. **R15 — adoption** → court-day UX and Sinhala-first usability validated with real juniors and clerks.
3. **R4/R27 — unauthorised access** → central authorisation from day one, tested on every path.
4. **R5 — conflict errors** → strong matching + mandatory human decision + historical party data.
5. **R19 — Sinhala text handling** → spikes in Phase 0.

## 3. Business continuity fallback

If NithiDesk is unavailable on a court morning: a **daily printable diary** (PDF, emailed to partners and clerk each evening for the next two days) and the offline-tolerant mobile view of today's cases provide a fallback. **[NEEDS LAWYER CONFIRMATION]** acceptable downtime and fallback expectations.
