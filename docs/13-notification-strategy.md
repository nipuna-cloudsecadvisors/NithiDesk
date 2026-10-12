# 13 — Notification Strategy

Covers PART 17.

---

## 1. Principles

1. **A notification must demand an action or prevent a mistake.** Otherwise it belongs in a digest or a dashboard, not an alert.
2. **Escalate, don't repeat.** If a problem persists, notify the *next* responsible person rather than spamming the same person.
3. **Digest by default; interrupt only for critical items.**
4. **Respect court hours** (silent push during configurable court hours; urgent items still badge).
5. **Never put confidential detail in channels outside the app** (email subject, SMS, lock-screen push). Use "You have 1 hearing date change — open NithiDesk" rather than client names, by default (**[MAKE CONFIGURABLE]**).
6. **Matter access applies** — no notification about a matter you cannot see.

---

## 2. Priority model

| Priority | Meaning | Channels | Timing | Examples |
|---|---|---|---|---|
| **P1 — Critical** | Imminent risk of missed hearing/deadline or security incident | In-app banner + push + email (+ SMS Phase 2) | Immediate, bypasses quiet hours; escalates if unacknowledged | Date conflict on a hearing ≤ 2 days away; hearing tomorrow with no appearing lawyer; hard deadline due tomorrow and not done; outcome not recorded by end of hearing day (to lead, then partner next morning); ethical-wall breach attempt; break-glass used |
| **P2 — High** | Needs action today | In-app + push | Immediate during working hours | Hearing date changed; verification overdue; conflict review assigned; client update awaiting approval; hearing reassigned to you |
| **P3 — Normal** | Needs action soon | In-app; included in daily digest | Batched | New matter assigned; task assigned; document requested; preparation not ready at T-3 |
| **P4 — Info** | Awareness | Daily/weekly digest only | Batched | Document uploaded to your matter; task completed by colleague; new party added |

---

## 3. Notification catalogue (defaults — [MAKE CONFIGURABLE])

| Event | Recipient(s) | Priority | Notes |
|---|---|---|---|
| Morning brief (today's hearings) | Each appearing user | Digest (P3) at 6:00 | Contains everything for the day |
| Hearing tomorrow | Appearing lawyer, clerk | P3 (in evening digest) | Upgraded to P2 if unverified or not ready |
| Hearing in 7 days | Lead lawyer | P4 (weekly digest) | Upgraded if unverified / missing documents |
| Hearing date changed | Appearing lawyer, lead lawyer, clerk | P2 | P1 if ≤ 2 days away |
| Date conflict (sources disagree) | Clerk, lead lawyer | P1 if ≤ 3 days, else P2 | |
| Date not verified (in window) | Clerk → lead lawyer (escalation after 1 day) → partner (T-1) | P2 → P1 | Escalation chain |
| Outcome not recorded | Appearing lawyer (evening) → lead lawyer → partner (next morning) | P2 → P1 | Critical control |
| Hearing without appearing lawyer (≤ 3 days) | Lead lawyer, partner | P1 | |
| Schedule clash (same lawyer, two courts) | Lawyer, lead lawyer | P2 | |
| Task assigned | Assignee | P3 | |
| Task due today | Owner | Digest | |
| Task overdue | Owner (day 1) → reviewer/lead (day 2) | P3 → P2 | Hard deadlines: P1 to owner + lead on due date morning |
| Missing document for upcoming hearing (T-3) | Lead lawyer, assistant | P2 | |
| Client update required (after outcome) | Lead lawyer | P3; P2 after 2 days | |
| Client update awaiting approval | Approver | P2 | |
| New matter assigned | Team members | P3 | |
| Conflict review required | Reviewer | P2 | Urgent enquiry → P1 |
| Conflict re-check triggered (new party) | Responsible partner | P2 | |
| New document uploaded | Matter team | P4 | Court orders/judgments → P3 to lead |
| Access granted / wall applied to you | User | P3 | |
| Security: new device login, MFA reset | User | P2 (email) | |
| Admin: backup failed, scanner down, queue stuck | Admin | P1 | |

---

## 4. Anti-noise rules

- **Deduplicate**: one notification per item per state change; updates edit the existing notification rather than create new ones.
- **Bundling**: 10 document uploads → "10 documents added to ND-2026-0142".
- **Caps**: max push notifications per user per hour (excluding P1) — e.g. 5; overflow goes to digest.
- **Snooze/acknowledge**: P1/P2 can be acknowledged (stops escalation) with optional note.
- **User preferences**: users can lower P3/P4 channels; cannot disable P1 or escalation (**[MAKE CONFIGURABLE]** at firm level).
- **Quiet hours**: personal quiet hours (e.g. 21:00–06:00) except P1.
- **Measure**: track notification open/act rates in pilot; remove notifications nobody acts on.

---

## 5. Channels & providers

| Channel | Phase | Notes |
|---|---|---|
| In-app (bell + inbox + badges) | MVP | Source of truth |
| Web push (PWA) | MVP-late | Android reliable; iOS push for installed PWAs only (iOS 16.4+) — **[ASSUMPTION]** acceptable |
| Email | MVP (digests + P1/P2) | Transactional email provider; SPF/DKIM/DMARC; no confidential details by default |
| SMS | Phase 2 | Local gateways (Sri Lankan mobile operators / aggregators); Sinhala SMS cost (UCS-2, doc 11 §10) |
| WhatsApp Business API | Phase 3 | Only via an approved Business Solution Provider; template approval; client communications primarily |
| Calendar feed (iCal) | Phase 2 | Read-only personal feed, masked titles |

Providers sit behind a **notification channel interface**; the domain emits events, the notification service decides recipients, priority, channel and timing.

---

## 6. Client-facing notifications (later)

Clients receive only: approved updates, verified next-date reminders (client-visible flag), document requests. Every client notification is logged as a communication. **[NEEDS LAWYER CONFIRMATION]** whether the firm wants automated reminders to clients at all (some lawyers prefer personal contact).
