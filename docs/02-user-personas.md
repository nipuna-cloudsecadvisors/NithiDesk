# 02 — User Personas

## 1. Principles

- **Titles are not permissions.** A user has a *job title* (configurable, bilingual), one or more *role templates* (permission bundles) and *matter memberships* (which matters they can touch). See doc 09.
- **[NEEDS LAWYER CONFIRMATION]** Sri Lankan firms vary: chambers around a senior counsel, formal partnerships, sole practitioners with a clerk, corporate legal departments. The personas below are generic; the pilot firm will map its real people onto them.
- **[MAKE CONFIGURABLE]** Which persona approves what (conflict clearance, engagement, client updates, closure) is a firm-level setting.

## 2. Persona summary

| # | Persona | Core question they ask NithiDesk | Primary device |
|---|---|---|---|
| P1 | Managing Partner / Head of Chambers | "Is anything at risk across the firm?" | Desktop + phone |
| P2 | Partner / Senior Counsel | "Are my matters on track, and what needs my approval?" | Desktop + phone |
| P3 | Senior Lawyer (Senior Associate) | "What am I running this week and who covers what?" | Laptop + phone |
| P4 | Junior Lawyer | "Where am I appearing today, and what must I ask for?" | **Phone** + laptop |
| P5 | Legal Assistant / Paralegal | "What documents and data do I need to prepare?" | Desktop |
| P6 | Court Clerk | "Which dates are unconfirmed, and what did the record say?" | **Phone** + shared desktop |
| P7 | Administrative / Front-desk Staff | "Who is this caller, and who handles them?" | Desktop |
| P8 | Finance Staff (Phase 2+) | "What is billable or unpaid?" | Desktop |
| P9 | System Administrator | "Are users, access and backups correct?" | Desktop |
| P10 | External Counsel | "What do I need for the hearing I've been briefed on?" | Phone + laptop |
| P11 | Client (Phase 3) | "What is happening in my case and when is the next date?" | Phone |
| P12 | Read-only / Auditor (optional) | "Show me the record for review." | Desktop |

## 3. Persona detail

### P1 — Managing Partner / Head of Chambers

| Aspect | Detail |
|---|---|
| Responsibilities | Firm-wide oversight; final authority on engagement and conflicts; ethical walls; workload balance; risk; client relationships for key clients |
| Needs to see | All matters **except** those behind an ethical wall that excludes them (walls apply to everyone, including the top — **[NEEDS LAWYER CONFIRMATION]** whether a firm head may ever be walled); firm-wide diary; unverified dates; overdue tasks; matter aging; workload by lawyer; pending conflict reviews; audit summaries |
| Should not see | Matters they are walled from; raw passwords/MFA secrets; AI prompt logs of others unless investigating (audited access only) |
| Daily actions | Review firm dashboard; approve conflict decisions/engagements; reassign appearances; review high-risk matters |
| Mobile | Read-mostly: today's firm-wide hearings, approvals queue, matter lookup; approve/decline conflicts on phone with reason |
| Notifications | Conflict review escalations; missed/unrecorded outcomes for high-priority matters; hearing in next 48h still unverified (firm-level digest); security alerts (summary) |
| Approvals | Conflict clearance (final); engagement acceptance for flagged matters; ethical wall creation/removal; matter closure for high-risk matters; data export approvals |

### P2 — Partner / Senior Counsel

| Aspect | Detail |
|---|---|
| Responsibilities | Responsible partner for a portfolio; supervises lawyers; appears in key hearings; signs off client advice |
| Needs to see | Their matters and their team's matters; diary for their team; preparation status; documents; draft client updates awaiting approval |
| Should not see | Matters where they have no membership (unless firm policy grants partners firm-wide read — **[MAKE CONFIGURABLE]**); walled matters; finance detail unless granted |
| Daily actions | Check today/tomorrow; review briefs; approve client updates; review conflict results for new enquiries they sponsor; assign juniors |
| Mobile | Full court-day mode; approvals; outcome recording |
| Notifications | Hearings tomorrow (own appearances + team summary); client update awaiting approval; conflict review assigned; outcome not recorded by end of day for own matters |
| Approvals | Conflict review (first-level or final, configurable); engagement; client updates; closure; document marking "filed version" |

### P3 — Senior Lawyer

| Aspect | Detail |
|---|---|
| Responsibilities | Lead lawyer on matters; drafting; appearances; delegating to juniors and clerks |
| Needs to see | Matters where they are lead/supporting; team diary; tasks; documents; preparation checklists |
| Should not see | Non-member matters; walled matters; firm financials; audit logs |
| Daily actions | Prepare for hearings; assign tasks; record outcomes; draft client updates; upload/version pleadings |
| Mobile | Full court-day mode |
| Notifications | Own hearings (7 days, 1 day, morning brief); tasks due/overdue; date changes on own matters; documents uploaded to own matters (digest) |
| Approvals | Review junior's drafts (task reviewer); approve hearing preparation complete; may be delegated client-update approval (**[MAKE CONFIGURABLE]**) |

### P4 — Junior Lawyer

| Aspect | Detail |
|---|---|
| Responsibilities | Appearances (often several courts per morning), covering for seniors, drafting, recording what happened |
| Needs to see | Matters they are assigned to (permanently or **per hearing** — see "appearance-scoped access", doc 09); last order; today's purpose; key documents; instructions from senior |
| Should not see | Matters not assigned; strategy notes marked "partner-only"; billing; conflict files |
| Daily actions | Open Today → walk through each case → record outcome + next date → create follow-up tasks; upload photos of minutes/documents |
| Mobile | **Primary user of court-day mode.** Must work one-handed, low bandwidth, Sinhala or English |
| Notifications | Morning brief; hearing reassigned to them; senior's instructions added to a hearing; outcome reminder if not recorded by a configured time |
| Approvals | None by default; submits outcomes which may require senior confirmation (**[MAKE CONFIGURABLE]**) |

### P5 — Legal Assistant / Paralegal

| Aspect | Detail |
|---|---|
| Responsibilities | Data entry; document collection and filing; preparing court bundles; client document requests |
| Needs to see | Matters assigned; document folders; tasks; client contact details needed for their work |
| Should not see | Restricted/privileged strategy notes; walled matters; conflict decision rationale (may see "cleared/not cleared"); finance |
| Daily actions | Upload & categorise documents; complete document checklists; create clients/prospects; run conflict searches (but not decide) |
| Mobile | Light: task list, document upload |
| Notifications | Tasks assigned/due; document requested; missing document for upcoming hearing |
| Approvals | None |

### P6 — Court Clerk

| Aspect | Detail |
|---|---|
| Responsibilities | **Guardian of the diary.** Checks case records/journal at registries; confirms next dates; collects court lists; files papers at registry; applies for and collects certified copies; pays court fees/stamps (**[NEEDS LAWYER CONFIRMATION]** exact duties) |
| Needs to see | Firm-wide (or assigned-courts) diary; case numbers, courts, parties; which dates are unverified; registry tasks; certified-copy applications |
| Should not see | Legal strategy notes; privileged advice; most document *contents* (may see titles and filing status — **[MAKE CONFIGURABLE]**); finance beyond court fees |
| Daily actions | Work the "Verification queue"; mark dates verified with source (journal entry, court list); record registry visits; upload photo of court list/notice; flag mismatches |
| Mobile | **High.** Verification queue, court-list check-off, photo upload, quick re-date |
| Notifications | Unverified hearings within N days for their courts; new hearing entered by a lawyer that needs verification; certified copy due for collection |
| Approvals | Verification of dates (they *attest*, they don't *decide* outcomes); conflicts between two claimed dates escalate to the lead lawyer |

**Design note:** in many Sri Lankan practices the clerk is the most reliable source of truth on dates. NithiDesk should make the clerk's verification *visible and credited*, not bypass it.

### P7 — Administrative / Front-desk Staff

| Aspect | Detail |
|---|---|
| Responsibilities | Takes enquiries; books meetings; captures prospect details; routes calls; basic client communication logs |
| Needs to see | Client/prospect contact info; which lawyer handles a client; appointment calendar; *existence* of a matter (title may be masked for restricted matters) |
| Should not see | Matter documents, notes, conflict details, strategy |
| Daily actions | Create enquiry; search client by phone/name; log call; schedule meeting |
| Mobile | Minimal |
| Notifications | Enquiry assigned back for more info; meeting reminders |
| Approvals | None |

### P8 — Finance Staff (Phase 2+)

| Aspect | Detail |
|---|---|
| Responsibilities | Fee notes / invoices; receipts; disbursements (court fees, stamp duty, travel); outstanding balances |
| Needs to see | Billing arrangement, fee entries, payments per matter; client billing contacts |
| Should not see | Documents, notes, strategy, evidence |
| Daily actions | Raise fee notes; record payments; chase arrears |
| Mobile | None in MVP |
| Notifications | Matter opened with fee arrangement; appearance recorded (if per-appearance billing); payment overdue |
| Approvals | Write-offs (or partner approves) |

**[DECISION REQUIRED]** Whether billing is in scope at all for early phases (doc 17 recommends Phase 2 "lightweight fee tracking", not accounting).

### P9 — System Administrator

| Aspect | Detail |
|---|---|
| Responsibilities | User lifecycle; MFA resets; role templates; reference data (courts, case types); backups; integrations |
| Needs to see | Users, roles, sessions, configuration, system health, audit logs (security events) |
| Should not see | **Matter content by default.** Admin ≠ superuser over client data. Break-glass access is separate, dual-approved and audited (doc 10) |
| Daily actions | Onboard/offboard users; reset MFA; review security alerts |
| Mobile | Emergency: disable user / revoke sessions |
| Notifications | Failed-login spikes; backup failure; storage/scan failures; break-glass invoked |
| Approvals | Executes permission changes approved by a partner (configurable dual control) |

**[ASSUMPTION]** In a small firm the "administrator" may be an outside IT contractor. That makes separating admin from matter content mandatory.

### P10 — External Counsel

| Aspect | Detail |
|---|---|
| Responsibilities | Appears or advises on specific proceedings/hearings when briefed |
| Needs to see | **Only** the briefed matter/proceeding, selected documents (the brief), relevant hearings |
| Should not see | Other matters; client's other affairs; internal firm discussions; fees |
| Daily actions | Read brief; view hearing details; optionally record outcome / add notes visible to the firm |
| Mobile | Court-day read access |
| Notifications | Hearing reminders for briefed hearings; brief updated |
| Approvals | None |
| Access controls | Time-boxed access; MFA mandatory; download restrictions/watermarking; auto-expiry after proceeding/hearing |

**[NEEDS LAWYER CONFIRMATION]** Do external counsel (e.g. a President's Counsel briefed by the firm) actually use firm systems, or do they receive a paper/PDF brief? If the latter, the feature becomes "generate a brief pack" rather than "external login" — much simpler for MVP.

### P11 — Client (Phase 3)

| Aspect | Detail |
|---|---|
| Responsibilities | Gives instructions; supplies documents and information; attends court when required; pays fees |
| Needs to see | Status in plain language; next date (only once **verified and approved for client visibility**); documents explicitly shared; requests for documents |
| Should not see | Internal notes, strategy, conflict information, other parties' confidential material, unverified dates, internal discussion, restricted documents, billing unless shared |
| Daily actions | Occasional: check next date; upload requested documents; read updates; reply to secure messages |
| Mobile | Primary channel (likely after WhatsApp/SMS link) |
| Notifications | Next date confirmed; document requested; update published |
| Approvals | None inside NithiDesk. Instructions the client gives (e.g. "appeal", "settle") are recorded by the firm as *Instruction* communications (doc 04 §7). Clients never approve anything in the system |

**Before Phase 3:** clients have no login. They receive approved updates through the firm's existing channels (doc 04 §9).

### P12 — Read-only / Auditor (optional)

| Aspect | Detail |
|---|---|
| Responsibilities | Internal quality review, compliance check or external audit, for a defined period and scope |
| Needs to see | The matters or record types in the grant (matter-scoped or firm-wide read) |
| Should not see | Anything outside the grant. Sees no data before the grant starts or after it expires |
| Daily actions | Read and annotate review findings (outside the matter record) |
| Mobile | None |
| Notifications | Access granted / expiring |
| Approvals | None. Granting this role requires managing-partner approval (**[MAKE CONFIGURABLE]**) |
| Access controls | Time-boxed; all access audited; no downloads or exports by default |

## 4. Cross-persona needs matrix

| Capability | P1 | P2 | P3 | P4 | P5 | P6 | P7 | P8 | P9 | P10 | P11 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Today / daily brief | firm | team | own+team | own | — | courts | — | — | — | briefed | — |
| Court-day mode | ● | ● | ● | ●● | — | ● | — | — | — | ● | — |
| Verify hearing dates | ● | ● | ● | ○ | — | ●● | — | — | — | — | — |
| Record outcome | ● | ● | ● | ●● | — | ○ | — | — | — | ○ | — |
| Conflict search | ● | ● | ● | ○ | ● | — | ○ | — | — | — | — |
| Conflict decision | ●● | ● | — | — | — | — | — | — | — | — | — |
| Upload documents | ● | ● | ● | ● | ●● | ● (photos) | — | — | — | ○ | ○ (P3) |
| Client update approval | ● | ●● | ○ | — | — | — | — | — | — | — | — |
| Reports | firm | portfolio | own | own | — | diary | — | finance | system | — | — |
| Admin config | — | — | — | — | — | — | — | — | ●● | — | — |

●● primary · ● yes · ○ optional/configurable · — no
