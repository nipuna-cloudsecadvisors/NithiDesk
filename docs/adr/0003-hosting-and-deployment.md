# 0003. Hosting and deployment

- **Status:** Proposed
- **Date:** 2026-10-13
- **Issue:** #19
- **Deciders:** product owner, with the firm
- **Blocked on:** owner decision **#12** (hosting location and data residency); lawyer question **#11** (Q10: hosting, PDPA, retention) **[NEEDS LAWYER CONFIRMATION]**; PDPA cross-border transfer conditions in force at go-live **[NEEDS LAWYER CONFIRMATION]**
- **Related:** [doc 10 §6, §6.1, §14](../10-security-plan.md), [ADR 0002](0002-tenancy-model.md), [ADR 0005](0005-file-storage.md)

## Context

NithiDesk holds privileged, confidential legal information about Sri Lankan clients. Where it is hosted affects:
- the PDPA's cross-border transfer conditions,
- client trust,
- latency from courts on mobile networks,
- cost, and backup and disaster-recovery options.

No hyperscale cloud provider is assumed to run a region inside Sri Lanka (**[ASSUMPTION]**, verify), so "regional cloud" means e.g. Mumbai or Singapore. Backup targets in doc 10 §14 are RPO ≤ 15 min for the database, ≤ 24 h for files, and RTO ≤ 8 h.

## Decision drivers

- Legal permissibility under PDPA and professional obligations.
- Operational maturity: managed Postgres with point-in-time recovery, encrypted object storage, immutable backups.
- Portability: no lock-in that makes a later move impossible.
- Cost and operations burden for a small team.
- Latency and availability from Sri Lankan mobile networks.

## Considered options

1. **Regional hyperscale cloud (e.g. Mumbai or Singapore), managed PostgreSQL + S3-compatible storage, containers.**
2. **Sri Lankan data centre / local cloud provider** (VMs or managed services, if available), self-managed Postgres + MinIO.
3. **On-premises at the firm** (server in the office).
4. **Hybrid:** primary in Sri Lanka, encrypted offsite backups in a regional cloud (or the reverse).

### Option 1: regional cloud
- Good: mature managed Postgres with PITR, object versioning and immutability; good tooling.
- Bad: data leaves Sri Lanka, so it depends on PDPA transfer conditions and client acceptance; possible USD cost exposure.

### Option 2: Sri Lankan data centre
- Good: data stays in-country; client trust; low latency.
- Bad: fewer managed services, so we would run Postgres, backups and MinIO ourselves; DR maturity varies by provider.

### Option 3: on-premises
- Good: the firm physically controls the data.
- Bad: power, connectivity, physical security, backups and patching fall on the firm; poor remote and court access; highest risk of data loss. Not recommended.

### Option 4: hybrid
- Good: in-country primary with stronger offsite DR.
- Bad: backups abroad are still a cross-border transfer; more moving parts.

## Decision

**Provisional:** stay portable now and choose the location once #12 and the PDPA question are answered.
- **Packaging:** OCI containers for `api` and `worker`, stateless, configured only through environment variables (as `internal/config` already does).
- **Dependencies:** standard PostgreSQL (no provider-specific extensions beyond `pg_trgm`, `unaccent` and ICU collations; `pgvector` later), and any S3-compatible object store.
- **Environments:** local (Docker Compose with Postgres + MinIO), staging, production. Infrastructure as code. Secrets come from a secret manager, never Git.
- **Default recommendation if the PDPA allows it:** option 1 for operational safety, encrypted with keys the operator controls. If in-country hosting is required, option 2 with a documented self-managed backup/PITR runbook and an immutable offsite copy. The offsite copy's location must satisfy the same rule.
- On-premises (option 3) is **not** recommended.

## Consequences

- Positive: code does not change whichever location is chosen.
- Negative: until #12 is decided, staging can be built but not production.
- Follow-up: the M1 platform epic delivers containers, Compose, CI, backups and a restore drill on the chosen target.

## Confirmation

- Restore drill meets RPO/RTO on the chosen hosting before go-live (Gate G3).
- A data-flow diagram showing every location personal data reaches (including backups, logs, email/SMS providers) is reviewed against PDPA by the lawyer.

## Open questions

- #12: in-country vs regional, and which provider? **[DECISION REQUIRED]**
- Do PDPA transfer conditions or Bar/professional rules restrict storing privileged material abroad? **[NEEDS LAWYER CONFIRMATION]**
- Who operates production (the owner's team, or a managed service provider)? **[DECISION REQUIRED]**
