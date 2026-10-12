# 01 — Product Vision

## 1. One-sentence vision

**NithiDesk (නීති Desk) is the daily operating platform for a Sri Lankan law practice: it connects every client, matter, court proceeding, hearing date, document, task and client update, so that no date is missed, no file is lost, and every lawyer knows — from a phone outside a courtroom — what the case is, what happened last time, what is needed today and what happens next.**

## 2. The business problem

Sri Lankan legal practice today typically runs on a combination of:

- a **physical court diary** (often kept by a senior clerk or the senior lawyer) — the single point of failure for hearing dates;
- **paper case files / briefs** that physically travel to court;
- **Excel sheets** for case lists, **Word** for pleadings, **shared folders / Google Drive / OneDrive** for scanned documents;
- **WhatsApp and phone calls** for client and team communication, with little of it recorded;
- **personal memory** of who is handling what and what the court said last time.

Consequences (to be validated with the lawyer — **[NEEDS LAWYER CONFIRMATION]** on severity):

| Problem | Consequence |
|---|---|
| Hearing dates live in one diary or one person's head | Missed or wrongly attended hearings, ex-parte orders, laches, professional-negligence exposure |
| Next date recorded verbally after court and transcribed later | Transcription errors; nobody knows whether a date was *confirmed* from the case record |
| Matter = file folder = one case number | When a case is appealed, the history fragments across files; prior case numbers get lost |
| Documents scattered | Time wasted before court; originals mislaid; wrong version filed |
| Juniors/clerks cover appearances at short notice | They lack context: last order, today's purpose, what to ask for |
| Client updates ad hoc | Clients chase the firm by phone; no record of what was told to whom |
| No conflict-check discipline | Risk of acting against a former client or a related party |
| No access control | Every staff member can see every file, including sensitive matters |

## 3. What NithiDesk is — and is not

**NithiDesk is:**

- A **matter-centric** practice platform built around Sri Lankan courts and procedures.
- A **verified court diary**: every hearing date carries its *source* and *verification state*.
- A **court-day companion** on mobile: open today's case, see the essentials, record the outcome and next date in under a minute.
- A **connected record**: Client → Matter → Proceeding(s) → Hearings → Documents → Tasks → Outcomes → Client updates, with full history preserved across appeals.
- **Bilingual by design** (Sinhala + English first-class, Tamil-ready).
- **Confidential by default**: matter-level access, ethical walls, audit trail.

**NithiDesk is not:**

- A generic CRM, a generic task manager or a generic document store.
- An e-filing system (it may connect to one later — see doc 15).
- An AI lawyer. AI may suggest; lawyers decide (doc 14).
- An accounting system. Billing is limited and later-phase (**[DECISION REQUIRED]** — see doc 17).

## 4. Product principles

1. **The matter is the anchor; the proceeding is the court's view.** One client problem (Matter) may spawn many proceedings across courts. Never collapse them.
2. **Every date has a source and a confidence.** A hearing date the system "heard about" is not the same as a date confirmed from the case record. The UI must never show them identically.
3. **History is never overwritten.** Case numbers, dates, documents and outcomes are versioned or superseded, not replaced.
4. **Court-first UX.** Design for a lawyer standing in a corridor with poor signal and one free hand.
5. **People are entities, roles are relationships.** A person may be a client in one matter, an opposing party in another and a witness in a third. Store the person once; attach roles. This is what makes conflict checking work.
6. **Sinhala is not a translation layer.** Names, notes, search and documents work natively in Sinhala, English, and mixed text.
7. **Least privilege by default.** Access is granted per matter; ethical walls override roles.
8. **Configure, don't hardcode, Sri Lankan variability.** Courts, case-number formats, party roles, stages, checklists and statuses are reference data.
9. **Integration-ready, integration-independent.** Works fully with manual entry and imports; government connectors plug in later.
10. **Human decides, system records.** Conflict decisions, engagement decisions, date verification and closure are explicit human acts, recorded with who/when/why.

## 5. Challenge to the proposed central chain

The brief proposed a linear chain:

```
Client → Matter → Court Proceeding → Hearing → Documents/Evidence → Tasks → Preparation
→ Appearance → Outcome → Next Hearing → Client Update → Appeal → Closure
```

This is a good *narrative* but the wrong *data model*. Problems:

| Issue in linear chain | Improvement |
|---|---|
| Client sits only at the top | A matter may have **multiple clients** (co-plaintiffs, husband and wife, company + director). Client is a *role* on a Matter, not a parent. |
| Documents hang under hearings | Documents belong primarily to the **Matter**, and are *linked* to proceedings, hearings, tasks, parties and evidence. One affidavit may be used at three hearings. |
| Tasks hang under documents | Tasks attach to **any** of: matter, proceeding, hearing, document, client. |
| Preparation / Appearance / Outcome look like separate entities | They are **phases of one Hearing record** (prepare → attend → record outcome). |
| Next hearing is a separate step | The outcome of hearing N **creates** hearing N+1 (linked), with its date source = "court order at hearing N". |
| Appeal sits near the end | An appeal is a **new Proceeding linked to the original**, under the same Matter, and may start while other proceedings continue (e.g. interlocutory appeal, parallel revision). |
| Not every matter is litigation | Advisory, conveyancing, notarial, documentation, demand-letter and settlement matters have **zero proceedings**. **[NEEDS LAWYER CONFIRMATION]** on the firm's non-litigation share. |
| Parties missing | Parties (people/organisations) and their **role per proceeding** are central to conflicts, court lists and search. |

**Recommended conceptual spine:**

```
                  Person / Organisation registry (one record per real-world entity)
                         │  plays roles (client, opposing party, witness, director…)
                         ▼
Client(s) ──engages──► MATTER ◄── responsible team (partner, lead, juniors, clerk)
                         │
          ┌──────────────┼────────────────────────┬──────────────┐
          ▼              ▼                        ▼              ▼
   PROCEEDING(s)    Documents/Evidence        Tasks        Communications
   (court, case no.,   (linked many-to-many     (linked to    (client updates,
    parties+roles,      to proceedings,          anything)     calls, meetings)
    stage, links to     hearings, tasks)
    parent/appeal)
          │
          ▼
     HEARINGS  (date assertions + verification → preparation → appearance → outcome → next hearing)
```

See [diagrams/system-map.md](diagrams/system-map.md) and [08-information-model.md](08-information-model.md).

## 6. Target users (summary — detail in doc 02)

| Group | Primary value |
|---|---|
| Managing partner / senior counsel | Oversight of every matter, every date, workload and risk |
| Partners / senior lawyers | Their matters, their team, approvals (conflicts, engagement, client updates) |
| Junior lawyers | Today's appearances with full context; recording outcomes |
| Court clerks | Diary control, verifying dates from case records, court lists, certified copies |
| Legal assistants / admin | Onboarding, documents, data entry, client contact |
| Finance (later) | Fee notes, payments |
| External counsel | Time-limited access to the matters they are briefed on |
| Clients (later) | Approved status, approved dates, shared documents |

**[ASSUMPTION]** The first firm is a small-to-medium practice (roughly 3–25 lawyers plus clerks and staff), not a single sole practitioner and not a 200-lawyer corporate firm. The design must still work for a sole practitioner with one clerk, because that is a large share of the Sri Lankan market.

**[NEEDS LAWYER CONFIRMATION]** Many Sri Lankan practices are structured as chambers around a senior counsel or as associations of independent Attorneys-at-Law rather than formal partnerships. The word "Partner" may not fit. Roles are therefore modelled as configurable titles mapped to permission templates (doc 09).

## 7. Sri Lankan context that shapes the product

These features of Sri Lankan practice change the design compared with a US/EU product. All are **[NEEDS LAWYER CONFIRMATION]** for detail:

1. **Registered attorney (proxy) vs counsel.** The Attorney-at-Law who files the proxy (attorney on record / instructing attorney) may differ from counsel who appears. A matter must capture *who holds the proxy* and *who appears* for each hearing. External counsel (including President's Counsel) may be briefed per hearing or per proceeding.
2. **The case record and journal entries.** Courts maintain a case record with dated journal entries; "checking the record" is how clerks confirm what happened and the next date. This is the gold-standard date source.
3. **Calling / mention dates vs trial dates.** Many hearings are short "call" dates for steps (filing answer, replication, list of witnesses) rather than substantive hearings. The hearing *purpose* taxonomy must reflect this.
4. **Frequent postponements.** Dates move often; the system must make re-dating fast and keep history.
5. **Court language.** Original courts largely work in Sinhala (Tamil in the North and East); appellate courts largely in English. Documents in one matter are routinely mixed-language.
6. **Case-number formats vary by court and case type** (e.g. Magistrate's Court "B" reports, District Court land/money/divorce/testamentary prefixes, provincial HC civil appeal numbers, CA writ/revision numbers, SC appeal / special leave / fundamental-rights numbers). Formats must be configurable patterns, not code. (Examples in doc 05 are illustrative only.)
7. **Court vacations, public holidays and Poya days** affect sittings and deadline computation.
8. **WhatsApp is the dominant communication channel** — the system should record *that* a WhatsApp exchange happened, not try to replace WhatsApp in MVP.
9. **Certified copies** of orders and proceedings are obtained from the registry and are important documents with their own lifecycle (applied → received).
10. **Personal Data Protection Act, No. 9 of 2022** and the professional confidentiality obligations of Attorneys-at-Law govern data handling. Exact obligations and commencement dates **[NEEDS LAWYER CONFIRMATION]**.
11. **Government digitisation is emerging but uneven.** Plan for manual entry first and connectors later (doc 15).

## 8. Success criteria

The product succeeds when, for the pilot firm:

| Outcome | Measure (pilot target, **[ASSUMPTION]**) |
|---|---|
| No missed hearings attributable to diary failure | 0 missed hearings in 6 months |
| Court diary is trusted | ≥ 95% of hearings in next 14 days are *Verified* at least 2 working days before |
| Lawyers find a case fast | Any authorised matter found in < 10 seconds by name, Sinhala name, case number, NIC or phone |
| Outcome recorded promptly | ≥ 90% of hearing outcomes recorded same day; median < 2 minutes on mobile |
| Juniors walk in prepared | Daily brief available for every appearance by 7:00 a.m. |
| Confidentiality | Zero unauthorised access incidents; 100% document views audited |
| Adoption | The physical diary is retired (or becomes the backup) within 3 months |
| Bilingual | Sinhala-preferring staff can complete every MVP workflow without English |

## 9. Scope of this planning phase

Delivered: 20 planning documents, 4 diagram documents, and a management summary. Explicitly **not** delivered: code, screens, database migrations, irreversible architecture choices.
