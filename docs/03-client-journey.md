# 03 — Client Journey

From first contact to archive. Diagram: [diagrams/client-journey.md](diagrams/client-journey.md).

## 1. Journey overview

| Phase | Stages |
|---|---|
| A. Intake | 1 Initial enquiry · 2 Client information capture · 3 Conflict check · 4 Conflict review · 5 Engagement decision |
| B. Onboarding | 6 Client onboarding · 7 Matter creation · 8 Lawyer assignment · 9 Document collection |
| C. Litigation loop | 10 Court proceeding creation · 11 Hearing preparation · 12 Court appearance · 13 Outcome recording · 14 Client update · 15 Next action |
| D. Resolution | 16 Settlement · 17 Appeal / related proceeding · 18 Closure · 19 Archiving |

**Key change from a naive flow:** the conflict check happens **before** detailed client data is captured and **before** confidential facts are recorded. Capturing a full story from someone you may not be able to act for creates its own confidentiality problem. **[NEEDS LAWYER CONFIRMATION]** how the firm handles this today.

Not every journey reaches Phase C — advisory and documentation matters skip proceedings entirely.

## 2. Stage detail

Each stage lists: **Who · Information required · Information created · Documents · Approvals · Notifications · Next.**

### Stage 1 — Initial enquiry

| | |
|---|---|
| Who | Front desk, legal assistant, or any lawyer (walk-in, phone, referral, WhatsApp, email) |
| Required | Name of enquirer (English and/or Sinhala), contact phone, how they reached the firm (referral source), *brief* nature of issue (category only, not full facts), names of **other parties** involved (needed for conflict check), urgency (e.g. "case listed next week") |
| Created | **Enquiry** record (status: New); Person/Organisation records in *prospect* state; enquiry reference |
| Documents | None required. If the enquirer hands over papers (e.g. summons, plaint copy), record receipt but **[ASSUMPTION]** don't upload into a matter until conflict clearance — hold in a restricted intake area |
| Approvals | None |
| Notifications | Assigned intake lawyer (if routed); urgent flag → partner on duty |
| Next | Stage 2 (light) and Stage 3 immediately |

**Urgency path:** If the enquirer has a hearing date imminent (e.g. summons returnable in 3 days), the enquiry carries an "urgent date" that is visible to the reviewing partner. **[NEEDS LAWYER CONFIRMATION]** whether the firm ever appears before conflict clearance completes (e.g. to seek a postponement).

### Stage 2 — Client information capture (minimum)

| | |
|---|---|
| Who | Intake staff |
| Required | Client type (individual / company / government / other organisation); identity fields enough to de-duplicate and conflict-check: names in both scripts where available, NIC or passport (individuals) or registration number (companies), phone, preferred language |
| Created | Prospect entity details; duplicate-check results ("possible existing person: 2 matches") |
| Documents | None yet (identity copies collected at onboarding) |
| Approvals | None |
| Notifications | None |
| Next | Stage 3 |

See doc 04 §2 for the full client data model and the purpose of each field.

### Stage 3 — Conflict check

| | |
|---|---|
| Who | Intake staff or lawyer runs the search (system-assisted) |
| Required | All names: prospective client(s), opposing parties, related persons (spouses, directors, guarantors, co-owners), companies, known witnesses where relevant; identifiers (NIC, company reg. no.); matter subject (e.g. land at a given address — optional) |
| Created | **Conflict check** record: search terms (frozen snapshot), each automatic match with score and reason (e.g. "NIC exact", "Sinhala name fuzzy 0.91", "director of client X"), searcher, timestamp |
| Documents | None |
| Approvals | None at this stage — search only |
| Notifications | Assigned reviewer: "Conflict review required" |
| Next | Stage 4 |

### Stage 4 — Conflict review

| | |
|---|---|
| Who | Reviewing lawyer (first level) → partner/managing partner (final) — **[MAKE CONFIGURABLE]** single or two-level |
| Required | Reviewer examines each match: *Not the same person* / *Same person — no conflict* / *Potential conflict* / *Actual conflict*; reason text per match; overall decision |
| Created | **Conflict decision**: Cleared / Cleared with conditions (e.g. ethical wall, consent obtained) / Not cleared; decision-maker, reason, timestamp; any waivers/consents recorded with document reference |
| Documents | Written consent/waiver letters if relied on |
| Approvals | **Mandatory human decision.** System never auto-clears; even "zero matches" requires a click-through confirmation by an authorised person (**[MAKE CONFIGURABLE]**: firms may allow "no match" auto-proceed with senior confirmation) |
| Notifications | Intake owner: decision made; if "cleared with ethical wall" → admin/partner to configure wall |
| Next | Stage 5 (or close enquiry as "Declined — conflict") |

### Stage 5 — Engagement decision

| | |
|---|---|
| Who | Partner / responsible lawyer |
| Required | Decision: Accept / Decline / Refer elsewhere; proposed responsible partner and lead lawyer; fee basis (if tracked); scope of engagement |
| Created | Engagement decision record; declined enquiries retained (minimal data) for future conflict checks — **[NEEDS LAWYER CONFIRMATION]** retention period for declined prospects under PDPA |
| Documents | Engagement/retainer letter (generated or uploaded); proxy to be signed later if litigation |
| Approvals | Partner approval (configurable thresholds, e.g. any matter against government entity requires managing partner) |
| Notifications | Intake owner and proposed team; declined → optional decline letter task |
| Next | Stage 6 |

### Stage 6 — Client onboarding

| | |
|---|---|
| Who | Legal assistant / admin |
| Required | Complete client profile (addresses, contacts, representatives, preferred language & channel, communication consent), identity verification evidence, authorised representatives for companies (board resolution) |
| Created | **Client** status = Active; contacts; representative links; consent record |
| Documents | NIC/passport copy (restricted category), company registration certificate / Form 1/20/40 equivalents (**[NEEDS LAWYER CONFIRMATION]**), board resolution, signed engagement letter |
| Approvals | None (lead lawyer confirms profile complete — optional) |
| Notifications | Lead lawyer: client onboarded |
| Next | Stage 7 |

### Stage 7 — Matter creation

| | |
|---|---|
| Who | Lawyer or assistant (from the accepted enquiry — data carried forward) |
| Required | Matter title (bilingual optional), client(s), category & practice area, responsible partner, lead lawyer, confidentiality level, open date, initial description, opposing parties (carried from conflict check) |
| Created | **Matter** (status: Open), matter number, membership/ACL, timeline event "Matter opened"; conflict check linked permanently |
| Documents | Intake documents released from restricted intake into the matter |
| Approvals | None if engagement approved; restricted/high-confidentiality matters require partner confirmation of access list |
| Notifications | All assigned team members: "New matter assigned" |
| Next | Stages 8–9; Stage 10 if litigation |

### Stage 8 — Lawyer assignment

| | |
|---|---|
| Who | Responsible partner |
| Required | Team roles: responsible partner, lead lawyer, supporting lawyers, clerk, assistant; proxy holder (registered attorney) if litigation; external counsel if briefed |
| Created | Matter memberships with role and effective dates (history kept when people change) |
| Documents | Proxy (when filed) |
| Approvals | Partner |
| Notifications | Each assignee |
| Next | Ongoing; reassignments recorded on timeline |

### Stage 9 — Document collection

| | |
|---|---|
| Who | Assistant / lawyer, client supplies |
| Required | Document checklist for the matter type (**[MAKE CONFIGURABLE]** templates, e.g. land case: deeds, plan, extracts from land registry, survey plan) |
| Created | Document requests (outstanding/received), uploaded documents with categories, originals register ("original deed held in safe, box 4") |
| Documents | Per checklist |
| Approvals | None |
| Notifications | Client contact task for missing items; "missing document" flag if required before a hearing |
| Next | Ongoing throughout the matter |

### Stage 10 — Court proceeding creation

| | |
|---|---|
| Who | Lawyer / clerk |
| Required | Court (from master list), proceeding type, case number (or "to be assigned" for newly filed), filing date, our side/role, parties with roles for *this* proceeding, opposing counsel/attorney on record, current stage |
| Created | **Proceeding**; party-role links; timeline event; case-number history entry |
| Documents | Plaint/petition/charge sheet/B report, summons, proxy, filed copies with registry stamp |
| Approvals | None; **duplicate case-number check** warns if the same court+number exists in another matter |
| Notifications | Team; clerk if no first hearing date yet |
| Next | First hearing created (Stage 11) |

### Stage 11 — Hearing preparation

| | |
|---|---|
| Who | Lead/junior lawyer, assistant, clerk |
| Required | Hearing date (with source + verification), purpose, appearing lawyer, preparation checklist (template by purpose/case type), required documents, witnesses to attend |
| Created | Checklist progress; preparation status (Not started / In progress / Ready / Blocked); brief note ("ask for date to file answer; object to amendment") |
| Documents | Brief bundle; previous order; drafts for filing |
| Approvals | Senior may mark "Ready" (**[MAKE CONFIGURABLE]**) |
| Notifications | 7-day and 1-day reminders; "not ready" escalation at T-2 days; unverified date alert to clerk |
| Next | Stage 12 |

### Stage 12 — Court appearance

| | |
|---|---|
| Who | Appearing lawyer (maybe junior, maybe external counsel) |
| Required | Access to: case number, court/courtroom, parties, last order, today's purpose, key documents, notes |
| Created | Listing status updated to `Held`, `Not reached` or `Court not sitting` (doc 06 §4). Appearing lawyer confirmed, plus any substitute who actually appeared |
| Documents | Photos of journal/minute where permitted (**[NEEDS LAWYER CONFIRMATION]** rules on photographing court records) |
| Approvals | None |
| Notifications | None during court (silent mode) |
| Next | Stage 13 |

### Stage 13 — Outcome recording

| | |
|---|---|
| Who | Appearing lawyer (target: same day, on phone) |
| Required | What happened (outcome type from list + free text), order/direction, **next date and purpose** (if given), who must do what by when, whether client must be informed |
| Created | Hearing outcome; listing status = `Outcome recorded`; next hearing (date source = "Court order at hearing on DD/MM", authority = Primary, verification state = `Unverified` until the clerk confirms it from the record. Whether the appearing lawyer's own entry may count as verified is **[MAKE CONFIGURABLE]**, see doc 06); follow-up tasks; timeline events |
| Documents | Certified copy request task if order matters |
| Approvals | Optional senior confirmation for juniors' entries |
| Notifications | Lead lawyer (if different); clerk (verify next date); client-update task created |
| Next | Stage 14, 15 |

### Stage 14 — Client update

| | |
|---|---|
| Who | Lawyer drafts; partner approves (configurable) |
| Required | Plain-language summary in client's preferred language; next date (only if verified, or clearly labelled provisional); actions needed from client |
| Created | Communication record (channel, content, sent by, timestamp); for WhatsApp/phone: a logged summary |
| Documents | Letter (if formal) |
| Approvals | Configurable: none / lead lawyer / partner; **AI-drafted text always requires human approval** |
| Notifications | Approver |
| Next | Stage 15 |

### Stage 15 — Next action

| | |
|---|---|
| Who | Lead lawyer assigns; owners carry out the tasks (junior, clerk, assistant) |
| Required | Directions from the outcome: what must be done, by whom and by when. Hard deadlines are flagged |
| Created | Tasks from the outcome (file a document by a date, obtain a certified copy, contact a witness, prepare written submissions, pay fees), linked to the matter, proceeding and next hearing; preparation checklist for the next hearing generated from its purpose |
| Documents | Drafts, filed copies with the registry stamp, certified copies as they arrive |
| Approvals | Reviewer sign-off on tasks that have a reviewer (doc 04 §6) |
| Notifications | Task assigned; due and overdue reminders; overdue hard deadline escalates to the responsible partner (doc 13) |
| Next | Back to Stage 11 for the next hearing, or Stage 16, 17 or 18 when the proceeding ends |

### Stage 16 — Settlement

| | |
|---|---|
| Who | Lead lawyer / partner |
| Required | Settlement terms summary (restricted), settlement type (in court / out of court), whether proceeding is withdrawn, terms of decree / consent order |
| Created | Proceeding stage = Settled; tasks (e.g. execute deed, file motion to withdraw, monitor instalments) |
| Documents | Settlement agreement, terms of settlement filed, consent decree |
| Approvals | Client instruction recorded; partner sign-off |
| Notifications | Team; finance (if fees tied) |
| Next | Possibly enforcement/execution proceeding (new proceeding), or closure |

### Stage 17 — Appeal / related proceeding

| | |
|---|---|
| Who | Partner decides (client instructions); lawyer creates |
| Required | Appealable decision (link to judgment), appeal deadline (computed from configurable rules, **must be verified by lawyer** — **[NEEDS LAWYER CONFIRMATION]** time limits per court/type), forum, new case number once assigned |
| Created | **New proceeding** linked to the original with relation type (appeal, leave to appeal, revision, writ, etc.); original stays intact with final status; deadline task with high priority |
| Documents | Petition of appeal, notice of appeal, leave application, certified copies of judgment and proceedings |
| Approvals | Partner + client instruction recorded |
| Notifications | High-priority deadline reminders (escalating) |
| Next | Litigation loop for new proceeding |

The firm may also be **respondent** to the other side's appeal — the same structure applies.

### Stage 18 — Closure

| | |
|---|---|
| Who | Lead lawyer requests; partner approves |
| Required | Closure checklist: all proceedings concluded or withdrawn; no future hearings; no open tasks (or transferred); originals returned to client (receipt); final client letter sent; fees settled (if tracked); closure reason |
| Created | Matter status = Closed; closure date; retention period starts |
| Documents | Closing letter; originals return receipt |
| Approvals | Partner (system blocks closure if open hearings/tasks unless override with reason) |
| Notifications | Team; finance |
| Next | Archive after configurable period |

### Stage 19 — Archiving

| | |
|---|---|
| Who | System (scheduled) + admin |
| Required | Retention rule for the matter category (**[NEEDS LAWYER CONFIRMATION]** — typical professional retention periods), legal hold check |
| Created | Archived state (read-only, searchable by authorised users); disposal date |
| Documents | Physical file location register (box/storage) |
| Approvals | **Disposal** requires partner approval and no legal hold; disposal certificate recorded |
| Notifications | Admin: matters reaching disposal eligibility (quarterly) |
| Next | Retained for future conflict checks (parties + matter summary) even after documents are disposed — **[NEEDS LAWYER CONFIRMATION]** |

## 3. Things that commonly break the clean journey

| Real-world event | Handling |
|---|---|
| Client arrives with a case already running (firm takes over from another lawyer) | Onboarding includes "existing proceeding" path: create proceeding with current case number, history import, *change of proxy* step |
| Client is the respondent served with a summons | Proceeding created from the summons; first date source = "summons" |
| Criminal matter where client is a suspect before charges (Magistrate's Court B report stage) | Proceeding type "B report / investigation"; later becomes a charge sheet / indictment proceeding in another court — linked (**[NEEDS LAWYER CONFIRMATION]**) |
| Multiple clients in one matter with divergent interests | Each client is a matter-client link; conflict re-check triggered when adding a client to an existing matter |
| A new opposing party joins mid-case | Adding a party triggers an **incremental conflict check** |
| Client stops responding / instructions withdrawn | Matter status "On hold — instructions awaited"; tasks for withdrawal of proxy if needed |
| Lawyer leaves the firm | Bulk reassignment wizard; history preserved |
