# 20 — Lawyer Discovery Questions

Covers PART 36. Use these in structured sessions with the practising Attorney-at-Law (and ideally a senior clerk and a junior lawyer).

**Legend:** 🔴 must answer before coding (Gate G0) · 🟠 before the related milestone · 🟢 can be answered during the pilot.
**Recording answers:** for each question record the answer, and whether the item becomes an **ASSUMPTION confirmed**, a **fixed rule**, or a **MAKE CONFIGURABLE** setting.

---

## ⭐ Top 10 questions (ask first)

| # | Question | Why it matters |
|---|---|---|
| 1 | 🔴 Walk us through exactly how a hearing date is recorded today — from the moment the judge gives the next date to when it is in the diary. Who writes it, where, and how is it checked? | Defines the verification model, roles and the biggest risk (R1) |
| 2 | 🔴 What counts as an authoritative confirmation of a date (journal entry, case record, court list, registry)? How do clerks check it, and how often do dates turn out to be wrong? | Verification rules & sources |
| 3 | 🔴 When a case is appealed (or goes to revision, leave to appeal, writ), do you treat it as the same file/matter or a new one? How do you keep the old case number? | Matter vs proceeding rule |
| 4 | 🔴 Who appears in court vs who holds the proxy? How often do juniors cover for others, or do you brief external counsel? Do external counsel need system access, or just a brief pack? | Access model, appearance-scoped access, external counsel scope |
| 5 | 🔴 How do you check conflicts today? Who decides? What records do you keep? Which relationships matter (family, directors, former clients)? | Conflict workflow & data |
| 6 | 🔴 Which courts, case types and practice areas make up most of your work? Roughly how many active matters and hearings per week? | Reference data, volumes, MVP priorities |
| 7 | 🔴 How do lawyers and clerks use phones at court today? Is mobile data reliable inside the courts you attend? Android or iPhone? | Court-day mode, offline scope |
| 8 | 🔴 Which matters must be restricted from some staff? Have you ever needed to wall someone off? Should clerks see all cases? | Confidentiality levels, walls, clerk scope |
| 9 | 🔴 Which staff prefer Sinhala, English, or both? Which legal terms should stay in English? Do you have Tamil-language matters? | Localization scope & glossary |
| 10 | 🔴 Where must the data be hosted (Sri Lanka only? cloud acceptable?) and what are your obligations under the PDPA and professional rules for client data, retention and destruction? | Hosting, security, retention architecture |

---

## 1. Firm structure

1. 🔴 Is the firm a partnership, chambers around a senior counsel, an association of independent lawyers, or other? What titles do people use?
2. 🔴 How many lawyers (by seniority), clerks, assistants, admin and finance staff?
3. 🔴 Who is "responsible" for a matter — a partner, the senior counsel, the lawyer who brought the client?
4. 🟠 Do lawyers have their own clients/matters that others should not see by default?
5. 🟠 Do you have branch offices or lawyers based outstation?
6. 🟢 Do you work with other firms (as instructing attorney or as counsel for other firms)?
7. 🔴 Is this system for your firm only, or do you envisage offering it to other firms? (affects multi-tenancy) — **Decision for owner**

## 2. Client onboarding

1. 🔴 How does a new client typically come to you? What do you record at the first meeting?
2. 🟠 Do you record anything before deciding to act (conflict check first, or facts first)?
3. 🟠 What identity documents do you collect for individuals and companies? Why?
4. 🟠 Do you issue engagement/retainer letters? Who signs?
5. 🟠 Who can decide to accept or decline a client? Are some matters escalated (government, high-profile, large value)?
6. 🟢 Do you keep records of people you declined? For how long?
7. 🟠 Do you need date of birth, gender, occupation, emergency contacts for any purpose (pleadings, remand cases)?

## 3. Court procedures

1. 🔴 List the courts you regularly appear in (with locations). Which are most frequent?
2. 🔴 What case types do you handle in each court? Example case numbers for each (to build patterns).
3. 🟠 What are the typical stages of a District Court civil case in your practice? A Magistrate's Court criminal case? An appeal?
4. 🟠 What hearing purposes do you use ("calling", "mention", "answer", "trial", "support", "argument", "judgment")?
5. 🟠 How are appeals, leave to appeal, revision and writ applications linked to the original case in your files?
6. 🟠 How are transfers handled? Does the case number change? Remitted retrials?
7. 🟠 What appeal and filing time limits do you track? How do you calculate them (holidays, vacations)?
8. 🟢 Do you need judges' names recorded? Is that sensitive?
9. 🟠 How do court vacations and holidays affect your diary?
10. 🟢 Do you handle tribunals (Labour Tribunal, Quazi courts), mediation boards or arbitration?

## 4. Case files

1. 🔴 What does a physical case file contain? How is it organised?
2. 🟠 Do you keep one file per case number or per client problem?
3. 🟠 Where are originals kept (deeds, certified copies)? How do you track them?
4. 🟢 How do you name/number your files today?
5. 🟠 What makes a file "closed"? What do you do at closure (return originals, final letter, fees)?

## 5. Hearings

1. 🔴 (Top 10 #1–2) Date recording and verification.
2. 🔴 Does a date announced in court and written down by the appearing lawyer count as confirmed, or must the clerk check the record?
3. 🟠 How do you get court lists (cause lists)? Paper notice board, website, registry, other? For which courts?
4. 🟠 How many hearings does a junior attend in a typical morning? In how many courts?
5. 🟠 How do you prepare for a hearing? Is there a checklist (client contacted, documents, witnesses, fees)?
6. 🟠 What do you write down after a hearing? Who needs to know?
7. 🟠 How often are dates changed before the hearing day, and how do you find out?
8. 🟢 Do hearings have specific times or just morning/afternoon lists?
9. 🟢 Have you ever missed a hearing or attended on the wrong date? What caused it? (helps prioritise controls)

## 6. Documents

1. 🟠 What document types do you handle most? Which are most often needed in court?
2. 🟠 What proportion are scanned paper vs born-digital? Typed in Sinhala, English or both?
3. 🔴 Do your Sinhala Word documents use Unicode or legacy fonts (e.g. FM Abhaya, DL-Manel)? Please share a sample.
4. 🟠 Are drafts reviewed by seniors before filing? How are versions tracked?
5. 🟠 How are evidence items marked and tracked (P1, D1…)? Who holds physical evidence?
6. 🟠 Are some documents never to be seen by clerks/assistants?
7. 🟠 How long must files be retained after closure? Any destruction practice?
8. 🟢 Are you permitted to photograph journal entries / court records? Under what conditions?

## 7. Security

1. 🔴 (Top 10 #8) Restricted matters and walls.
2. 🔴 (Top 10 #10) Hosting, PDPA, professional obligations.
3. 🟠 Who should be able to download documents? Print? Export lists?
4. 🟠 Do staff use personal phones/laptops for work?
5. 🟠 Who provides IT support (internal, contractor)? Should they see client data? (Recommended: no.)
6. 🟢 Would you accept MFA via an authenticator app for all users? Any staff without smartphones?
7. 🟠 What happens today when a staff member leaves?

## 8. Current software

1. 🔴 What software is used now (Excel, Word, Google Workspace/Microsoft 365, any case management system, accounting)?
2. 🟠 Which email system does the firm use (for possible single sign-on and email integration)?
3. 🟢 Any existing digital diary or calendar sync that people rely on?

## 9. Mobile usage

1. 🔴 (Top 10 #7) Phones at court, connectivity, devices.
2. 🟠 Would lawyers be comfortable recording outcomes on their phone in court corridors? Any etiquette restrictions on phone use in court buildings?
3. 🟢 Would a downloadable "court pack" of key documents for offline use be valuable?

## 10. Notifications

1. 🟠 What reminders would genuinely help (night before, morning of, a week before)? Which would be annoying?
2. 🟠 Email, SMS, WhatsApp, or in-app only? Who should be escalated to when a date is unconfirmed or an outcome isn't recorded?
3. 🟢 Should notifications ever contain client names (lock screens, email subjects)?

## 11. Billing

1. 🔴 Is billing/fee tracking needed in the first release? If yes, what minimum: fee arrangement, per-appearance fees, fee notes, receipts, disbursements (court fees, stamps)?
2. 🟢 What accounting software, if any, is used?

## 12. Client communication

1. 🟠 How do you update clients after a hearing today (call, WhatsApp, SMS, letter)? Who does it?
2. 🟠 Which communications must be recorded for the firm's protection (instructions, advice, settlement authority)?
3. 🟠 Should client updates be approved by a senior before sending?
4. 🟢 Would clients use a portal? Would the firm want automated date reminders to clients?

## 13. Reports

1. 🟠 What does the managing partner need to see weekly/monthly?
2. 🟢 Do you track lawyer workload or matter aging today?
3. 🟢 Any reports required by regulators, insurers or clients?

## 14. Sinhala requirements

1. 🔴 (Top 10 #9) Language preferences and terminology.
2. 🟠 How are client names written in pleadings (initials + surname, full names, both scripts)?
3. 🟠 Which Sinhala terms should we use for: client, matter, proceeding, hearing, journal entry, verified, outcome?
4. 🟠 Are letters to clients and documents for court generated in Sinhala? Which templates?
5. 🟢 How do staff type Sinhala today (keyboard layouts, phone apps)?

## 15. Data migration

1. 🔴 Where is the current diary kept? Can we photograph/transcribe it?
2. 🔴 Where are the active case lists (Excel)? Can we see a copy (or anonymised sample)?
3. 🟠 How many years of closed files exist, and in what form?
4. 🟠 Are there closed-matter party lists useful for conflict checking?
5. 🟠 Who can be allocated to verify migrated dates, and for how long?
6. 🟠 How long can the paper diary run in parallel?

---

## Items recorded as ASSUMPTION pending answers

| Assumption | Doc |
|---|---|
| Small-to-medium firm (3–25 lawyers), single tenant | 01, 17 |
| Most listings have no specific time (session only) | 06 |
| Lawyer-in-court dates are *not* verified until clerk checks the record | 06 |
| Conflict check precedes detailed fact capture | 03 |
| Most lawyers use Android phones; connectivity in courts is unreliable | 12 |
| External counsel may not need logins (brief pack may suffice) | 02 |
| No Tamil UI needed in MVP | 11 |
| Billing not required in MVP | 17 |
| WhatsApp recorded by reference only | 04 |
| Retention periods TBD by lawyer | 07 |
| Advisory/conveyancing matters exist and have no proceedings | 04 |

## Items marked NEEDS LAWYER CONFIRMATION (tracked to questions)

| Item | Question(s) | Doc |
|---|---|---|
| What counts as an authoritative date confirmation | Top 10 #1–2; §5.2 | 06 |
| Matter vs proceeding rule for appeals, revision, writs | Top 10 #3; §3.5 | 05 |
| Court list availability per court | §5.3 | 06, 15 |
| Legacy Sinhala fonts in use | §6.3 | 11, 16 |
| Current state of Sri Lankan court digital services | §8.1 (plus owner research) | 15 |
| Whether clients want automated reminders | §12.4 | 13 |
| Judge analytics / judges' names sensitivity | §3.8 | 14 |
| Acceptable downtime and fallback | §7 (add to session) | 19 |

## Configurable decisions (default proposed, firm may change)

| Decision | Proposed default | Doc |
|---|---|---|
| Whether a date written down in court by the appearing lawyer counts as verified | No — clerk verifies | 06, 17 §7 |
| Who approves conflicts, engagement, client updates, closure | Responsible partner (one level) | 17 §7 |
| Clerk scope | "Diary scope" (hearing metadata, not substance) firm-wide; masked for Restricted, date/court/number only for Highly restricted; firm may limit to assigned courts | 09 §2.1 |
| Notification escalation timings, digest times, quiet/court hours | As doc 13 §3 | 13 |
| Parallel-run length | 6 weeks | 16 §4.2 |
| Client names in emails/push | Off | 13 §1 |
| Matter numbering pattern | ND-YYYY-NNNN | 17 §7 |
| Retention class per matter category | Lawyer to set | 07 |
