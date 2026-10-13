# 04 — Legal Matter Workflow

Covers: client information (PART 3), conflict checking (PART 4), matters (PART 5), parties (PART 8), tasks (PART 16), client communication (PART 18), timeline (PART 15), client portal analysis (PART 19).

Lifecycle diagram: [diagrams/matter-lifecycle.md](diagrams/matter-lifecycle.md).

---

## 1. The foundational idea: one registry of people and organisations

Every real-world person or organisation is stored **once** in a **Person/Organisation registry** ("Entity"). Everything else refers to it:

- a **client** is an entity with a client relationship to the firm;
- a **party** is an entity playing a role in a proceeding (plaintiff, accused, respondent…);
- a **witness**, **director**, **spouse**, **guarantor**, **opposing attorney** — all entities with relationships.

Why: conflict checking, de-duplication, search and history all depend on recognising "this is the same Nimal Perera" across matters. A design where client records and party records are separate tables of free-text names makes conflict checking unreliable.

**Risk:** over-eager merging of two different people with the same name (very common for Sinhala surnames such as Perera, Fernando, Silva). Merges must be **human-confirmed** and **reversible** (doc 19).

---

## 2. Client information (PART 3)

Rule: **every field has a stated purpose.** Fields without a purpose are not collected (PDPA purpose limitation — **[NEEDS LAWYER CONFIRMATION]** on PDPA obligations applicable to law firms).

### 2.1 Common to all entity types

| Field | Purpose | Notes |
|---|---|---|
| Internal entity ID | System identity | Never shown as a legal identifier |
| Display name (English script) | Search, documents in English, court papers in appellate courts | |
| Display name (Sinhala script) | Search, Sinhala correspondence, original-court documents | Unicode; normalised for search (doc 11) |
| Tamil name | Future | Field exists in model, hidden in UI until enabled |
| Also-known-as / alternate spellings | Conflict check, search | Multiple; e.g. "Fernando / Fernandopulle / ප්‍රනාන්දු" |
| Entity type | Drives which fields apply | Individual / Company / Government entity / Other organisation |
| Phone numbers (typed: mobile/landline/WhatsApp) | Contact, search by phone | Normalised to E.164 (+94…) for search; displayed local format |
| Email | Contact | |
| Addresses (typed: residential/service/registered office/postal) | Correspondence, court service addresses, conflict signal (same address) | Bilingual free text; district/province from list |
| Preferred language (UI/communication) | Client updates, letters | Sinhala / English / Tamil |
| Preferred channel | Client updates | Phone, WhatsApp, email, letter, SMS |
| Communication consent | PDPA, channel use | Recorded with date & method |
| Sensitivity flags | Access control | e.g. "public figure", "vulnerable person", "minor" |
| Source / introduced by | Business development, conflict signal | Optional |
| Notes (internal) | Context | Restricted to matter team |
| Representatives | Who may instruct or receive information for this client (attorney under power of attorney, guardian, company officer) | Stored as relationships (§2.6) with authority evidence |
| Previous matters | Context, conflict checking | **Derived** from matter-client links, never typed in. Shown only for matters the viewer may access; others appear as a count |
| Conflict history | Audit of past conflict checks involving this entity | **Derived**; visible only to conflict approvers (§3.4) |

### 2.2 Individuals

| Field | Purpose | Notes |
|---|---|---|
| Name parts (initials, surname, full names) in both scripts | Court papers commonly use "initials + surname"; conflict matching | Sri Lankan naming: initials such as "W.A.D." + surname, or full names "Warnakulasuriya Arachchige Don …". **[NEEDS LAWYER CONFIRMATION]** how the firm writes names in pleadings |
| NIC number | Strongest de-duplication & conflict key | Validate both old (9 digits + V/X) and new (12 digit) formats; store normalised; **masked** in UI except for authorised roles |
| Passport number + country | Non-residents, foreigners | Masked |
| Date of birth | Only if needed (e.g. derived from NIC, age for minors) | **[ASSUMPTION]** not collected by default |
| Gender | **Not collected by default** — no clear business purpose | Configurable if firm needs it for pleadings |
| Occupation | Pleadings often state it | Optional |
| Emergency contact | Brief requested it; purpose weak | **[DECISION REQUIRED]** — recommend omit unless firm confirms use (e.g. remanded clients) |
| Guardian / next friend | Minors, persons of unsound mind | Relationship link |

### 2.3 Companies

| Field | Purpose |
|---|---|
| Registered name (and former names) | Conflict, pleadings |
| Registration number (e.g. PV/PB/PQ numbers — **[NEEDS LAWYER CONFIRMATION]** formats) | De-duplication |
| Registered office address | Service |
| Directors, shareholders (significant), company secretary | Conflict check — stored as **relationships** to Person entities, with as-of date |
| Parent / subsidiary / group companies | Conflict check |
| Authorised representatives & authority evidence (board resolution) | Who may instruct the firm |

### 2.4 Government entities

| Field | Purpose |
|---|---|
| Official name (Sinhala/English/Tamil) | Pleadings |
| Type (ministry, department, statutory board, local authority, police station, AG's Department) | Search, conflict |
| Parent body | Conflict |
| Office-holder designations (e.g. "Officer-in-Charge, X Police Station") — role vs person | Many cases name an *office*, not a person — model as office entity with current holder optional |

### 2.5 Other organisations

Trusts, societies, partnerships, temples/religious bodies, NGOs, unincorporated associations: name, registration (if any), trustees/office bearers as relationships, representative.

### 2.6 Relationships (entity ↔ entity)

Typed, directional, dated: spouse, parent/child, sibling, director-of, shareholder-of, employee-of, guarantor-for, trustee-of, represented-by, successor-in-title, heir-of, attorney (power of attorney holder). **[MAKE CONFIGURABLE]** relationship types.

### 2.7 Data the system should *not* hold on the client record

- Full narrative facts of the case (belongs in the Matter, under matter access control).
- Bank details (unless finance module needs it later).
- Religion, ethnicity, health — only when relevant to the matter (e.g. Kandyan/Muslim/Thesawalamai personal law questions) and then in the **matter**, restricted. **[NEEDS LAWYER CONFIRMATION]**.

### 2.8 Duplicate prevention

On creation: exact match on NIC/passport/registration number → hard warning; fuzzy match on name (both scripts) + phone/address → soft warning showing candidates. Merge is a privileged, audited, reversible action.

---

## 3. Conflict checking (PART 4)

### 3.1 What a conflict check searches

| Source | Examples |
|---|---|
| Current & former clients | All entities with client relationship (active or ended) |
| Opposing parties (current & past) | Every entity that was an adverse party in any proceeding |
| Other parties | Co-defendants, interveners, added parties |
| Related entities | Directors, shareholders, group companies, spouses, family members, guarantors |
| Witnesses & experts | Lower weight, but shown (**[MAKE CONFIGURABLE]**) |
| Declined enquiries & prospects | People who consulted but were not engaged (they may have shared confidential info) |
| Existing proceedings | Court + case number (current and previous numbers): the prospect's case may already be on the firm's books, possibly for the other side |
| Matter subjects | Optional: property address / land name / vehicle number (land disputes often recur) |
| Opposing counsel / attorneys | Not a conflict per se, but informative |

### 3.2 How matching works (assistive, not decisive)

1. **Identifier match** — NIC, passport, company registration: exact after normalisation. Highest confidence.
2. **Phone / email match** — exact after normalisation.
3. **Name match**, applied to every name variant:
   - Unicode normalisation of Sinhala (NFC + removal/standardisation of ZWJ/ZWNJ — e.g. "ප්‍රනාන්දු" written with and without zero-width joiner must match) — doc 11.
   - Case/punctuation/initial normalisation ("W.A.D. Perera" ≈ "WAD Perera" ≈ "Perera W A D").
   - Trigram similarity for misspellings.
   - **Cross-script matching**: Sinhala ↔ Latin transliteration keys (e.g. "පෙරේරා" ↔ "Perera"; "ප්‍රනාන්දු" ↔ "Fernando"). Rule-based transliteration + a curated dictionary of common Sri Lankan surnames and given names (**[ASSUMPTION]**: the dictionary is the most valuable part; seeded and grown by the firm).
   - Token-order independence ("Silva Kamal" ≈ "Kamal Silva").
4. **Relationship expansion** — one hop by default (e.g. prospective opponent is director of an existing client), configurable depth.
5. **Address similarity** — optional, low weight.

Each hit carries a **score** and an **explanation** ("Sinhala surname + initials match, NIC differs → probably different person").

Common-name problem: "Perera" alone would match thousands. Results are ranked; surname-only hits are collapsed and shown only when combined with another signal. **[MAKE CONFIGURABLE]** thresholds.

### 3.3 Review & decision workflow

```
Search run  →  Matches listed with reasons
           →  Reviewer classifies each match: Not same entity | Same entity – no conflict | Potential conflict | Actual conflict
           →  Overall recommendation (reviewer)
           →  Final decision (authorised approver): Cleared | Cleared with conditions | Not cleared
           →  Conditions recorded: ethical wall members, written consent obtained (document link), scope limitation
           →  Locked record (immutable; corrections via new appended decision)
```

- **The system never makes the decision.** AI or fuzzy matching may *suggest* relationships (doc 14).
- The search snapshot (terms + results as they were) is preserved even if data changes later.
- **Re-checks** are triggered automatically when: a party is added to a proceeding, a client is added to a matter, or a related entity relationship is added — the result goes to the responsible partner.
- **[MAKE CONFIGURABLE]** who may approve; whether two-level review is required; whether "no matches" requires approval.
- **[NEEDS LAWYER CONFIRMATION]** the professional-conduct standard the firm applies (e.g. acting against former clients in related matters; consent-based waivers).

### 3.4 Confidentiality of conflict data

Conflict searches expose cross-matter information ("we act for X in a matter against Y"). Therefore:
- Searchers see **that** a match exists and its type, but not matter details they are not authorised for ("Possible match in a restricted matter — refer to partner").
- Conflict decisions and rationales are visible only to approvers and the responsible partner.
- Every conflict search is audited.

---

## 4. Matter management (PART 5)

### 4.1 Definition

> **A Matter is the firm's engagement to handle one client problem (or a closely related set), under one responsible partner, with one access-control boundary.** It may contain zero, one or many court proceedings and survives across courts and appeals.

**[NEEDS LAWYER CONFIRMATION]** Rules for *when to open a separate matter* vs a new proceeding under the same matter. Proposed rule: same facts & same client objective → same matter; new dispute/new client objective → new matter, linked as "related matter".

### 4.2 Matter fields

| Field | Notes |
|---|---|
| Matter number | Human-readable, firm-configurable pattern, e.g. `ND-2026-0142` (**[MAKE CONFIGURABLE]**); immutable once issued |
| Title (EN/SI) | Convention e.g. "Perera v. Silva — land at Kotte" (**[MAKE CONFIGURABLE]** naming convention) |
| Short description | Restricted to team |
| Client(s) | One or more; each with role (primary client, co-client, instructing party, guarantor, paying party) |
| Category | Litigation / Advisory / Conveyancing & notarial / Documentation / Arbitration / ADR / Other (**[MAKE CONFIGURABLE]**) |
| Practice area | Land, civil money recovery, commercial, family/divorce, testamentary, criminal, labour, fundamental rights, writs, intellectual property, banking, etc. (**[MAKE CONFIGURABLE]**) |
| Responsible partner | Exactly one at a time; history kept |
| Lead lawyer | One at a time; history kept |
| Supporting lawyers, clerk(s), assistant(s) | Many; with effective dates |
| Proxy holder / attorney on record | Per proceeding, not per matter (may differ between courts) |
| Lifecycle status | See 4.3 |
| Phase (derived/labelled) | See 4.3 |
| Priority | Normal / High / Urgent — drives notification intensity |
| Confidentiality level | Standard / Restricted / Highly restricted (see doc 09) |
| Risk level | Optional (**[MAKE CONFIGURABLE]**) — e.g. high-value, reputational, limitation risk |
| Open date / close date | Close date set by closure workflow only |
| Billing arrangement | Phase 2: fixed fee / per appearance / retainer / pro bono / not tracked |
| Preferred client language | Defaults from primary client |
| Related matters | Typed links: related dispute, same property, same client group, successor matter |
| Conflict check reference | Mandatory link (except migrated matters, flagged "conflict check not on system") |
| Physical file location | Many firms will keep paper files for years; record where the file is |

### 4.3 Matter statuses — challenged and recommended

A typical status list for this kind of product (and the one in the earlier build brief) is: Prospective, Open, Active, Waiting, Court Pending, Appeal, Settlement, Closed, Archived.

**Problem:** this list mixes three different things:
1. *Lifecycle* (is the engagement open?): Prospective, Open, Closed, Archived.
2. *Litigation state* (what is the court doing?): Court Pending, Appeal — these belong to **proceedings**, and a matter may have one proceeding in appeal while another is pending.
3. *Activity* (is anything happening?): Active, Waiting.

Single-select status forcing a choice among these will be wrong half the time and will not be maintained.

**Recommendation — three separate dimensions:**

| Dimension | Values | Set by |
|---|---|---|
| **Lifecycle status** (single, controlled transitions) | `Open` ⇄ `On hold`; `Open` → `Closure requested` → `Closed` → `Archived` → (`Disposed`); `Closure requested` → `Open` if rejected; `Closed` → `Open` on reopen (diagrams/matter-lifecycle §2) | Users via workflow; closure and reopen need partner approval |
| **Phase** (label, configurable) | Pre-litigation · Litigation · Appeal · Settlement / negotiation · Enforcement / execution · Advisory · Post-judgment | User-chosen, with suggestion derived from proceedings |
| **Attention flags** (derived, automatic) | Has hearing in next 7 days · Unverified upcoming date · Overdue tasks · Missing required documents · No activity in N days · Awaiting client instructions | System |

"Prospective" is **not a matter status**: prospects are **Enquiries** (separate entity). A matter is created only after engagement. This keeps unaccepted, possibly-conflicted information out of matter space.

### 4.4 Matter operations

| Operation | Rules |
|---|---|
| Open | From accepted enquiry (normal) or direct with mandatory conflict reference (configurable bypass for partners, audited) |
| Change team | History retained; outgoing member loses access (configurable grace for handover) |
| Change confidentiality | Partner only; audited; access list re-evaluated |
| Link related matter | Bidirectional typed link; does **not** grant access across matters |
| Put on hold | Reason required; reminders suppressed except hard deadlines |
| Request closure | Checklist (doc 03 Stage 18) |
| Reopen | Partner approval; reason; new timeline event |
| Archive / dispose | Retention engine (doc 07 §7) |

---

## 5. Parties (PART 8)

### 5.1 Model

```
Entity (person/organisation)  ──plays──►  PartyRole in a Proceeding
   e.g. Nimal Perera                       role: "1st Defendant", side: Opposing, represented by: Attorney X
```

| Attribute | Meaning |
|---|---|
| Entity | Registry record |
| Proceeding | Party roles are per proceeding (a Plaintiff in DC becomes Respondent/Appellant in appeal) |
| Role type | From a **configurable role catalogue per proceeding type** |
| Ordinal / designation | "1st Defendant", "2nd Respondent", "Substituted Plaintiff" — ordering matters in Sri Lankan pleadings |
| Side | Our client / Aligned / Opposing / Neutral / Court-appointed (e.g. curator) |
| Represented by | Attorney/counsel entity, firm, or "Attorney-General" for State |
| Status | Active / Deceased / Substituted / Added / Discharged / Withdrawn — with dates |
| Substitution chain | Substituted party links to the original (deceased party → heirs) |
| Capacity | e.g. "as Executor of the estate of …", "as Officer-in-Charge" |

### 5.2 Role catalogue (configurable, examples)

| Proceeding family | Role examples |
|---|---|
| Civil original (DC) | Plaintiff, Defendant, Added-Defendant, Intervenient Petitioner, Substituted Plaintiff/Defendant |
| Partition | Plaintiff, Defendants (often many — numbered), Surveyor/Commissioner (**[NEEDS LAWYER CONFIRMATION]**) |
| Testamentary | Petitioner, Respondent, Executor, Administrator, Heirs, Intervenient |
| Criminal (MC/HC) | Complainant, Accused, Suspect (B report stage), Victim, Aggrieved party, Prosecutor (police / AG) |
| Appeals (HCCA/CA/SC) | Appellant, Respondent, Plaintiff-Appellant, Defendant-Respondent, Petitioner, Respondent-Respondent… (compound roles carry history) |
| Writ / FR | Petitioner, Respondents (often officials + AG) |
| Labour / tribunals | Applicant, Respondent, Employer, Workman; on appeal to the Provincial High Court, Applicant-Appellant / Respondent-Respondent etc. |
| Primary Court (e.g. land-dispute proceedings) | Informant, First Party, Second Party, Intervenient (**[NEEDS LAWYER CONFIRMATION]**) |
| Commercial High Court | As civil original (Plaintiff, Defendant) (**[NEEDS LAWYER CONFIRMATION]**) |
| Any | Witness, Expert, Interpreter, Curator, Receiver, Commissioner |

Compound appellate roles (e.g. "Defendant-Appellant-Petitioner") are generated from history + current role rather than typed freehand (**[NEEDS LAWYER CONFIRMATION]** convention).

### 5.3 Opposing lawyers

Opposing attorneys on record and counsel are entities with a "legal practitioner" type (optional enrolment details), linked to the party they represent per proceeding. Useful for conflict signals, communications and search.

---

## 6. Task management (PART 16)

### 6.1 Task model

| Field | Notes |
|---|---|
| Title / description | Bilingual text, free |
| Type | General · Filing · Registry visit · Certified copy · Client contact · Document request · Drafting · Review · Payment · Witness contact (**[MAKE CONFIGURABLE]**) |
| Links | Exactly one **anchor** (matter, mandatory unless "firm task") plus optional links to proceeding, hearing, document, client, party |
| Owner (assignee) | One accountable person |
| Watchers | Optional |
| Reviewer | Optional; task cannot complete until reviewer approves (e.g. junior drafts, senior reviews) |
| Created by / source | Manual · From hearing outcome · From checklist template · From document request · From AI suggestion (accepted by human) |
| Due date (+ optional time) | Local date (Asia/Colombo) |
| Hard deadline flag | Distinguishes legal deadlines (appeal period, filing date ordered by court) from soft internal dates — drives escalation |
| Priority | Low / Normal / High / Critical |
| Status | `To do` → `In progress` → `Waiting` (reason: client / court / other party / internal) → `In review` → `Done`; or `Cancelled` (reason required) |
| Reminder(s) | Relative to due date; defaults by priority |
| Dependencies | "Blocked by" other tasks; finish-to-start only (keep simple) |
| Completion note & evidence | Optional document link ("filed — registry stamp uploaded") |

### 6.2 Task sources that matter most

1. **Hearing outcome** → "file answer by DD/MM", "obtain certified copy", "inform client".
2. **Preparation checklist** (template per hearing purpose / case type) → items can be converted to tasks with owners.
3. **Document requests** → task to chase client.
4. **Deadlines** (appeal periods etc.) → computed suggestion, lawyer-confirmed, *hard deadline*.

### 6.3 Rules

- Tasks on a closed matter must be completed/cancelled/transferred before closure.
- A hard-deadline task cannot be cancelled without a reason and the lead lawyer's confirmation.
- Overdue hard deadlines escalate to responsible partner (doc 13).

---

## 7. Client communication (PART 18)

### 7.1 What is recorded

| Type | Recorded as | Official matter record? |
|---|---|---|
| Client status update (sent by firm) | Full content, channel, recipients, approver, sent time | **Yes** |
| Formal letter | Letter document + communication entry | **Yes** |
| Email (sent/received) | Phase 1: manual log + attach .eml/PDF. Phase 2+: email integration (forward-to-matter address) | **Yes** when filed to matter |
| Meeting | Date, attendees, summary, follow-ups | **Yes** (summary); detailed notes may be "privileged internal" |
| Telephone call | Date/time, who, summary, follow-ups | **Yes** if logged; logging encouraged for instructions and advice |
| WhatsApp | **Reference** entry: date, participants, summary; optional screenshot/export upload | Yes when logged; **[DECISION REQUIRED]** — NithiDesk does not read WhatsApp in MVP |
| SMS reminders sent by system | Automatic log | Yes (system communication) |
| Client instructions | Special communication type "Instruction" — e.g. "client instructs to appeal", "client agrees to settle at X" | **Yes — high importance**; shown on timeline; can be required by workflows (appeal, settlement) |
| Internal team discussion | Comments on matter/hearing/task | **No** — internal, not client communication; never shared |

### 7.2 Principles

- **"Instruction" and "Advice given"** are distinguished because they are the communications that protect the firm in a dispute with the client.
- Outbound client updates in MVP are **drafted in NithiDesk, sent through the firm's existing channel** (copy to WhatsApp/email), then marked "sent". Direct sending via email is Phase 1-late/Phase 2; SMS/WhatsApp Business API later (doc 13).
- AI-drafted updates always require human approval (doc 14).
- Communications are matter-scoped and follow matter access.

---

## 8. Case timeline (PART 15)

A chronological, filterable view of the matter across all proceedings.

### 8.1 Event sources

| Event | Automatic? | Notes |
|---|---|---|
| Enquiry received / conflict cleared | Auto | Visible only to authorised roles |
| Client onboarded / matter opened | Auto | |
| Team changes | Auto | |
| Proceeding opened / case number assigned / changed | Auto | Case-number history visible |
| Filing (plaint, answer, petition, motion) | Semi-auto | When a document of a "filing" category is marked *filed* with a filing date |
| Hearing held + outcome | Auto | One event per hearing with outcome summary |
| Hearing date changed / postponed | Auto | Collapsed by default (noise) |
| Order / judgment | Auto | When order/judgment document uploaded and linked, or outcome type is "judgment delivered" |
| Document uploaded | Auto but **hidden by default** | Too noisy; show only "key documents" flagged |
| Client instruction / advice / update | Auto (from communications) | |
| Meeting | Auto (from communications) | |
| Task completed | Only for *hard deadline* or *flagged* tasks | Avoid noise |
| Appeal / related proceeding filed | Auto | Visually branches the timeline |
| Settlement | Auto (stage change) | |
| Closure / reopen / archive | Auto | |
| Manual milestone | Manual | "Site inspection done", "Mediation session" |

### 8.2 Design

- **Swimlanes per proceeding** (DC / HCCA / SC) on desktop; single list with proceeding badges on mobile.
- Filters: proceeding, event type, "key events only" (default), date range.
- Each event links to its source record. Events derived from records are **not editable** directly — edit the source.
- Exportable as a chronology (PDF, bilingual) for briefs — Phase 2.
- **AI timeline assistance** (Phase 3): propose events extracted from documents, accepted by a lawyer (doc 14).

---

## 9. Client portal — MVP or later? (PART 19)

**Recommendation: NOT in MVP. Phase 3.**

| Factor | Analysis |
|---|---|
| Value | Real but secondary: reduces "what is my next date?" calls |
| Pre-requisite | The firm's internal data must be trustworthy first (verified dates, curated shared documents). Exposing an unreliable diary to clients is reputationally dangerous |
| Security | Opens the system to the internet for hundreds of external users; doubles the attack surface; needs separate authentication, onboarding, support |
| Habit | Sri Lankan clients predominantly expect WhatsApp/phone contact. **[NEEDS LAWYER CONFIRMATION]** demand |
| Cheaper alternative (Phase 2) | **Approved client updates** sent via WhatsApp/SMS/email from NithiDesk templates; optional **secure one-time document links** (expiring) for sharing specific documents |

When built, the portal shows only: matter status in plain language, *verified and client-visible* upcoming dates, documents explicitly shared, document requests, secure messages. Never: internal notes, strategy, conflict information, internal discussion, restricted documents, unverified dates. "Client-visible" is an explicit flag per item, default **off**.
