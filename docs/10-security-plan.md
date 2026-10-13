# 10 — Security Plan

Covers PART 22 (security) and PART 23 (auditability). Authorisation model: doc 09.

> NithiDesk holds privileged, confidential legal information about identifiable people. A breach is a professional-conduct, contractual and (under the Personal Data Protection Act, No. 9 of 2022) potentially regulatory event. **[NEEDS LAWYER CONFIRMATION]** on applicable PDPA obligations, timelines and any Bar Association / Supreme Court rules on electronic records.

---

## 1. Security objectives

1. Only authorised people see a matter or document (least privilege, walls).
2. Every access to sensitive data is attributable (audit).
3. Records cannot be silently altered or deleted (integrity).
4. The firm can always get its data back (availability, backups).
5. A lost phone or a departed employee does not become a data leak.
6. AI and integrations never widen access.

Standards to align with: **OWASP ASVS (Level 2 target, selected Level 3 controls for documents and authentication)**, OWASP API Security Top 10, OWASP Top 10, zero-trust principles.

---

## 2. Threat model (summary)

| Threat actor | Example | Primary controls |
|---|---|---|
| Curious insider | Staff browsing a celebrity's divorce file | Matter ACL, confidentiality levels, view auditing, anomaly alerts |
| Conflicted insider | Lawyer formerly acting for opponent | Ethical walls |
| Departing employee | Bulk download before leaving | Download permissions, export controls, rate alerts, immediate disable |
| External attacker | Credential stuffing, phishing | MFA, rate limiting, breached-password checks, session controls |
| Malicious file | Weaponised PDF/DOCX from opposing party | Quarantine, malware scan, MIME validation, sandboxed preview rendering |
| Lost/stolen device | Phone left at court | Short sessions, PWA cache encryption & expiry, remote session revocation |
| Compromised admin / IT contractor | Admin reads matter files | Admin has no matter access; break-glass dual control; audit |
| Supplier / cloud | Hosting or AI provider misuse | Encryption, contractual controls, data residency decision, AI data minimisation |
| Opposing party social engineering | Calls clerk asking for next date | Process training; client-visibility flags; no public lookups |

The detailed STRIDE threat model (Phase 0) is in [security/threat-model.md](security/threat-model.md).

---

## 3. Authentication

| Control | Plan |
|---|---|
| Identity | Local accounts in MVP with an OIDC-ready design (Microsoft Entra ID / Google Workspace later, **[DECISION REQUIRED]** depends on what the firm uses for email) |
| Passwords | Minimum length ≥ 12, breached-password check, no forced periodic rotation (NIST 800-63B), argon2id hashing |
| **MFA** | **Mandatory for all internal users** from day one. TOTP authenticator app in MVP; passkeys/WebAuthn next. SMS OTP only as fallback (**[ASSUMPTION]** acceptable locally, weaker) |
| Step-up auth | Re-authenticate for: bulk export, wall changes, permission changes, break-glass, revealing full NIC, downloading highly restricted documents |
| Account lockout | Progressive delay + temporary lock; alert on spikes |
| Recovery | Admin-assisted MFA reset with identity check, audited, notifies user |
| External counsel | MFA mandatory; accounts time-boxed |
| Clients (Phase 3) | Separate authentication realm; passwordless email/SMS link + device binding; no access to internal app |

## 4. Sessions

| Control | Plan |
|---|---|
| Session type | Server-side sessions (opaque ID in Secure, HttpOnly, SameSite cookie) — easier revocation than long-lived JWTs |
| Idle timeout | Desktop 30 min; mobile 15 min with quick re-unlock (**[MAKE CONFIGURABLE]**). MVP: re-unlock with a short server-checked PIN tied to the session (rate-limited; after 5 failures, full login). Phase 2: biometric through passkeys/WebAuthn, in line with doc 17 §2.2 |
| Absolute lifetime | 12 hours (one working day) |
| Device visibility | Users see their active sessions/devices; can revoke |
| Admin revocation | Disable user → all sessions killed immediately |
| CSRF | SameSite + anti-CSRF tokens on state-changing requests |
| Concurrent sessions | Allowed, visible; alert on unusual geography (**[ASSUMPTION]** most use is within Sri Lanka) |

## 5. Authorisation

See doc 09. Engineering rules: central policy, deny by default, matter filter in every query, ethical-wall check in policy core, automated authorisation test suite blocking merges.

## 6. Data protection

| Area | Control |
|---|---|
| In transit | TLS 1.2+ (prefer 1.3), HSTS |
| At rest — database | Disk/volume encryption; **field-level encryption** for NIC/passport numbers and other high-sensitivity identifiers (with a blind index/hash for exact search) |
| At rest — files | Object storage server-side encryption; per-environment keys; consider envelope encryption for Highly restricted documents |
| Keys & secrets | Secret manager or encrypted environment config; never in Git; rotation procedure |
| Data residency | **[DECISION REQUIRED]** Sri Lanka-hosted vs regional cloud (e.g. Singapore/Mumbai). Affects latency, PDPA cross-border transfer rules, client trust. Architecture must not assume either |
| Data minimisation | Purpose-tagged fields (doc 04 §2); masked display |
| Logs | No document content, no full NIC, no passwords/tokens in application logs |
| Document restrictions | Per-document levels (Standard · Restricted · Privileged-internal · Highly restricted) are enforced by the same policy layer as matter access (doc 09 §4). Previews, thumbnails, search snippets, OCR text and AI context obey the document's level, not just the matter's |

### 6.1 Personal Data Protection Act, No. 9 of 2022 (PDPA)

The firm will usually be the **controller** for client and party data in NithiDesk. The software provider, if it hosts the system, is a **processor**. Points the design must allow for, with the legal reading **[NEEDS LAWYER CONFIRMATION]** for each:

| PDPA theme | Design consequence |
|---|---|
| Lawful basis, purpose limitation, data minimisation | Every personal-data field has a documented purpose (doc 04 §2). Optional sensitive fields are off unless the firm enables them |
| Special categories (e.g. health, criminal-offence data, children's data) | Treated as sensitive. Criminal and family matters default to **Restricted** confidentiality (doc 09 §3) **[MAKE CONFIGURABLE]** |
| Data-subject rights (access, rectification, erasure, objection, review of automated decisions) | A data-subject request workflow sits in the Governance context. Erasure is weighed against legal-professional retention duties and legal holds, so the outcome is a **recorded human decision**, not an automatic delete. No automated decisions with legal effect (the AI guardrails in doc 14 already forbid them) |
| Data Protection Officer | The firm designates a DPO. NithiDesk gives the DPO reporting access (request register, breach register, processing records) without matter content |
| Personal-data breach notification | The incident process (§16) records detection time, affected data subjects and matters (derived from the audit log), and the notification decision and date |
| Cross-border transfer | Hosting outside Sri Lanka depends on the Act's transfer conditions and any adequacy decisions. This feeds the data-residency decision (§6) **[DECISION REQUIRED]** |
| Processor obligations | A written data-processing agreement between the firm and any hosting, OCR, AI or SMS provider |

**[ASSUMPTION]** The Act's provisions are being brought into force in stages. Phase 0 must confirm which obligations are in force at go-live.

## 7. File upload security

Pipeline in doc 07 §6: quarantine → size/extension allow-list → magic-byte MIME check → malware scan → hash → private storage with random keys → previews rendered in an isolated worker (document renderers are a common exploit vector) → signed short-lived URLs. Filenames sanitised (path traversal, Unicode tricks, very long names). Macro-enabled Office files blocked or flagged.

## 8. Export restrictions

| Export type | Control |
|---|---|
| Single document download | Permissioned per role; audited; optional watermark (user + timestamp) for Restricted/Highly restricted |
| Bulk download (matter zip) | Partner approval or step-up; audited; rate-limited |
| Reports to Excel/PDF | Only data the user can see; audited |
| Full firm export (data portability / exit) | Managing partner + admin dual approval; encrypted archive |
| Print | Allowed for briefs; watermark option |
| Copy/paste / screenshots | Cannot be technically prevented; rely on audit, policy and training |

## 9. Audit logging (PART 23)

### 9.1 Principles

- **Append-only** from the application's perspective (no update/delete API; database role without UPDATE/DELETE on audit store; periodic hash-chaining or export to write-once storage for tamper evidence).
- Each event: timestamp (UTC), actor (user/system/connector), session/device, IP, action, target type + ID, matter ID, outcome (success/denied), before/after for changes (excluding secrets/large content), reason (where required).
- Searchable by managing partner (business events) and administrator (security events). Viewing the audit log is itself audited.
- Retention: **[NEEDS LAWYER CONFIRMATION]**; proposed ≥ 7 years for business events tied to matter retention, ≥ 2 years for security telemetry.

### 9.2 What is audited

| Category | Events | Notes |
|---|---|---|
| Authentication | Login success, login failure, MFA success/failure, logout, session revoked, password/MFA reset, lockout | Failure spikes alert |
| Access (read) | Matter opened, client profile opened, **document viewed**, **document downloaded**, full NIC revealed, search performed (query + result count), report run | Read auditing is essential in legal practice; volume manageable |
| Records (write) | Client/entity created/edited/merged/unmerged; matter created/edited; proceeding created; case number added/changed; party added/removed; document uploaded/new version/soft-deleted/purged; evidence/custody events | |
| **Court dates** | Hearing created, date assertion added, date changed, **verified**, verification revoked, conflict raised/resolved, outcome recorded/corrected, hearing cancelled/vacated | Highest business sensitivity |
| Conflicts & engagement | Conflict search run, match classified, decision made, engagement decision | Immutable |
| Permissions | Role changes, matter membership changes, appearance grants, ethical wall create/change/remove, confidentiality changes, break-glass | Alert on walls & break-glass |
| Data movement | Exports, bulk downloads, external shares (secure links), offline court-pack created/wiped | |
| AI | AI request (who, feature, matter, documents included), output viewed, suggestion accepted/rejected | Doc 14 |
| Lifecycle | Matter closure request/approval, reopen, archive, legal hold applied/released, disposal | |
| Admin & config | Reference-data changes, template changes, integration credentials changed, backup/restore events | |

### 9.3 Detective use

- Alerts: denied access to walled matter; unusual volume of document views/downloads by one user; access outside working hours to Highly restricted matters (**[MAKE CONFIGURABLE]**); break-glass use; admin role changes.
- Monthly access review report for partners: who accessed their Restricted matters.
- Audit integrity check: a scheduled job verifies the hash chain and alerts on gaps. If audit writes fail, sensitive actions such as downloads and exports are **refused** rather than performed unaudited (fail closed).
- Security audit entries shown to the system administrator identify matters by ID only (doc 09 §6), so the audit log does not leak matter titles or client names.

### 9.4 Coverage check against PART 23

| PART 23 action | Covered by §9.2 row |
|---|---|
| Login · failed login | Authentication |
| Client access · matter access | Access (read): client profile opened, matter opened |
| Document viewing · download · upload · deletion | Access (read); Records (write): uploaded / soft-deleted / purged |
| Court-date changes · court-date verification | Court dates |
| Permission changes | Permissions |
| Conflict decisions | Conflicts & engagement |
| Data exports | Data movement |
| AI queries | AI |
| Matter closure | Lifecycle |

## 10. Mobile access

- Same authentication; mobile sessions shorter; PIN re-unlock in MVP, biometric via WebAuthn in Phase 2 (§4).
- No documents stored on device except explicit offline court packs (doc 12 §5).
- Remote revocation wipes offline packs on next app open (and they self-expire regardless).
- Optional: restrict downloads on mobile to preview-only for Restricted documents.

## 11. Offline access

See doc 12 §5. Summary: only selected court-pack data, encrypted, expiring (e.g. end of day + grace), wiped on logout/revocation; never the firm database.

## 12. AI access

See doc 14. Summary: AI acts **as the requesting user** (same permissions), only on data passed by the gateway after permission filtering; no training on firm data; provider contracts with zero retention where available; every request audited; AI disabled by default per firm and per matter confidentiality level (Highly restricted matters excluded unless explicitly enabled).

## 13. Administrative privileges

- Admins manage users, configuration and infrastructure — **no default access to matter content**.
- Permission changes affecting matter access require partner approval (dual control, configurable for small firms).
- Production database/storage access for engineers: just-in-time, logged, with documented purpose; no routine direct DB access.
- Separate admin accounts from everyday user accounts for people with both roles.

## 14. Backup & recovery security

| Control | Plan |
|---|---|
| Scope | PostgreSQL (logical + PITR/WAL), object storage (versioned), configuration, encryption keys (escrowed separately) |
| Frequency | Continuous WAL / daily snapshot; object storage replication; weekly full |
| Encryption | Backups encrypted with keys separate from production |
| Access | Backup storage reachable only by the backup service and named operators, with MFA. Restore and backup-download operations are audited, need dual approval and alert the managing partner. Engineers never use backups as test data |
| Isolation | At least one copy offsite/immutable (ransomware protection) |
| Targets | RPO ≤ 15 min (DB), ≤ 24 h (files) — **[ASSUMPTION]**; RTO ≤ 8 hours |
| Restore testing | Quarterly restore drill to an isolated environment, documented |
| Legal holds | Backups honour holds; disposal of data from backups occurs by backup expiry (documented) |

## 15. Application security engineering (for later phases)

Parameterised queries only (SQL-first with generated code); strict input validation; output encoding; CSP; dependency and container scanning; secret scanning; SAST in CI; authorisation tests as merge gates; security headers; rate limiting per user/IP; safe error messages (no SQL errors, stack traces, paths, storage details); structured logs without confidential content; annual penetration test before go-live and yearly after.

## 16. Incident response (outline)

Detection → triage (admin + managing partner + DPO) → containment (revoke sessions, disable accounts, rotate keys) → assessment (which matters/clients affected, via audit log) → notification decision (clients; the Data Protection Authority under the PDPA, within the Act's timeline — **[NEEDS LAWYER CONFIRMATION]**) → recovery → post-incident review. Security incident register maintained in Governance context.
