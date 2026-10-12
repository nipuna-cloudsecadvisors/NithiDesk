# NithiDesk — Planning Documentation Index

> Status: **PLANNING DRAFT v0.1** — for review with a practising Sri Lankan Attorney-at-Law before any development begins.
> No application code, screens or database migrations exist or should be created from these documents until review is complete.

## Marker conventions used in every document

| Marker | Meaning |
|---|---|
| **[NEEDS LAWYER CONFIRMATION]** | We are not certain how Sri Lankan practice works here. Must be confirmed before it is built. |
| **[MAKE CONFIGURABLE]** | Firms, courts or practice areas may differ. Build as configuration / reference data, never hardcode. |
| **[ASSUMPTION]** | A working assumption so planning can continue. Valid until contradicted. |
| **[DECISION REQUIRED]** | A product or business decision the owner/lawyer must make before the related build phase. |

## Reading order (dependency order)

| # | Document | Depends on |
|---|---|---|
| 01 | [Product vision](01-product-vision.md) | — |
| — | [System map diagram](diagrams/system-map.md) | 01 |
| 02 | [User personas](02-user-personas.md) | 01 |
| 03 | [Client journey](03-client-journey.md) | 02 |
| — | [Client journey diagram](diagrams/client-journey.md) | 03 |
| 04 | [Legal matter workflow](04-legal-matter-workflow.md) (clients, conflicts, matters, parties, tasks, communication, timeline) | 03 |
| — | [Matter lifecycle diagram](diagrams/matter-lifecycle.md) | 04 |
| 05 | [Court & proceeding model](05-court-proceeding-model.md) | 04 |
| 06 | [Hearing workflow](06-hearing-workflow.md) (diary, daily/weekly views, verification) | 05 |
| 07 | [Document workflow](07-document-workflow.md) (documents, evidence, search) | 04, 05, 06 |
| 08 | [Information model](08-information-model.md) | 04–07 |
| — | [Information relationships diagram](diagrams/information-relationships.md) | 08 |
| 09 | [Role & permission model](09-role-permission-model.md) | 02, 08 |
| 10 | [Security plan](10-security-plan.md) (incl. audit) | 09 |
| 11 | [Localization plan](11-localization-plan.md) | 04, 07 |
| 12 | [Mobile & court-day mode](12-mobile-court-mode.md) (incl. offline, navigation, dashboards) | 06, 07, 10 |
| 13 | [Notification strategy](13-notification-strategy.md) | 06, 09 |
| 14 | [AI opportunities & guardrails](14-ai-opportunities-and-guardrails.md) | 07, 10 |
| 15 | [Government integration strategy](15-government-integration-strategy.md) | 05, 06 |
| 16 | [Data migration plan](16-data-migration-plan.md) | 08, 11 |
| 17 | [MVP scope](17-mvp-scope.md) (incl. reports, configuration) | all above |
| 18 | [Product roadmap](18-product-roadmap.md) | 17 |
| 19 | [Product risk register](19-product-risk-register.md) | all above |
| 20 | [Lawyer discovery questions](20-lawyer-discovery-questions.md) | all above |
| — | [Planning summary](../NITHIDESK_PLANNING_SUMMARY.md) | all above |

## Where each PART of the brief is answered

| Brief part | Location |
|---|---|
| 1 Users | 02 |
| 2 Client journey | 03, diagrams/client-journey |
| 3 Client information | 04 §2 |
| 4 Conflict checking | 04 §3 |
| 5 Matter management | 04 §4 |
| 6 Court structure | 05 §2–3 |
| 7 Proceedings | 05 §4–6 |
| 8 Parties | 04 §5, 05 §7 |
| 9 Hearing management | 06 §2–5 |
| 10 Lawyer daily work | 06 §6 |
| 11 Weekly planning | 06 §7 |
| 12 Court-day mode | 12 §3 |
| 13 Document management | 07 §2–6 |
| 14 Document search | 07 §8 |
| 15 Case timeline | 04 §8 |
| 16 Task management | 04 §6 |
| 17 Notifications | 13 |
| 18 Client communication | 04 §7 |
| 19 Client portal | 04 §9 |
| 20 Global search | 07 §9 |
| 21 Sinhala & English | 11 |
| 22 Security | 10 |
| 23 Auditability | 10 §9 |
| 24 Mobile | 12 §2 |
| 25 Offline | 12 §5 |
| 26 AI | 14 |
| 27 Government connections | 15 |
| 28 Reporting | 17 §6 |
| 29 Migration | 16 |
| 30 Information model | 08, diagrams/information-relationships |
| 31 MVP | 17, 18 |
| 32 UX navigation | 12 §6 |
| 33 Dashboards | 12 §7 |
| 34 Workflow configuration | 17 §7 |
| 35 Risk analysis | 19 |
| 36 Planning questions | 20 |
| 37 Required output | this folder |
| 38 Final report | ../NITHIDESK_PLANNING_SUMMARY.md |

## Source inputs

- Owner's planning brief (38 parts) and the earlier build-oriented brief (63 sections, used as reference for technical direction only — **not** as authorisation to build).
- "CourtDesk LK — System Map" concept deck (4 pages): six-module concept (onboarding, case setup, documents, court diary, court-day support, follow-up) and the principle "every item stays connected to the same matter".
