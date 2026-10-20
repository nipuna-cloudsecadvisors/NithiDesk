# 0002. Tenancy model

- **Status:** Proposed
- **Date:** 2026-10-13
- **Issue:** #18
- **Deciders:** product owner
- **Blocked on:** owner decision **#13** (single firm vs future multi-firm SaaS) **[DECISION REQUIRED]**
- **Related:** [summary §11](../../NITHIDESK_PLANNING_SUMMARY.md), [doc 08](../08-information-model.md), [doc 09](../09-role-permission-model.md), [doc 10 §6.1 PDPA](../10-security-plan.md)

## Context

The first customer is one firm. The owner may later offer NithiDesk to other firms (#13). Adding a tenant key to every table later is expensive and risky. Leaving it out now also risks cross-firm leakage if multi-firm is bolted on later. Under the PDPA each firm is a separate **controller**; the provider would be a **processor** for all of them.

## Decision drivers

- Zero chance of one firm seeing another firm's data.
- Low cost now, while there is only one firm.
- Keep the multi-firm option open without rework.
- Per-firm backup, restore, export and deletion (needed for PDPA, and for a firm leaving).

## Considered options

1. **Shared database and schema, `firm_id` on every row, enforced by the application *and* PostgreSQL Row-Level Security (RLS).**
2. **Schema per firm** in one database.
3. **Database (or whole deployment) per firm.**
4. **No tenant key** (pure single-tenant).

### Option 1: shared schema + `firm_id` + RLS
- Good: cheapest to run at scale; one migration path. RLS provides a database-level backstop if an application query forgets the filter.
- Bad: per-firm restore or export is harder; one noisy firm affects others; RLS needs care with connection pooling (set `app.firm_id` per transaction).

### Option 2: schema per firm
- Good: easier per-firm export.
- Bad: migrations run N times; cross-schema mistakes are still possible; pooling is complicated.

### Option 3: database/deployment per firm
- Good: strongest isolation; per-firm hosting location (helps data residency); trivial per-firm restore and deletion.
- Bad: more operations per firm; fleet upgrades need automation.

### Option 4: no tenant key
- Good: simplest.
- Bad: rules out multi-firm later without a rewrite. Rejected by the planning summary.

## Decision

**Hybrid of 1 and 3: tenant-aware data model, single-tenant deployment.**
- Every table carries `firm_id` (non-null, part of unique keys where relevant), and every query filters on it through the authorisation layer.
- RLS policies on `firm_id` are enabled from M1 as a backstop, with `SET LOCAL app.firm_id` per transaction.
- **Deploy one firm per database/deployment for now** (option 3 operationally).
- If #13 decides on multi-firm SaaS, smaller firms can later share a deployment (option 1) without schema changes. A firm that needs isolation or in-country hosting keeps its own deployment.

## Consequences

- Positive: little cost now; no rewrite later; isolation is strong while there is one firm.
- Negative: `firm_id` must be present on every insert, and RLS adds a little complexity. Tests must cover "wrong firm" reads.
- Follow-up: the authorisation suite (`internal/authz`) gains cross-firm scenarios; the migration lint requires `firm_id` on new tables.

## Confirmation

- A CI check fails any migration that creates a business table without `firm_id`.
- An integration test with two firms in one database proves no cross-firm read on every read path, including search, counts and reports.

## Open questions

- #13: single firm or SaaS? This changes only the deployment model, not the schema. **[DECISION REQUIRED]**
- Should reference data (courts, case types) be global, shared across firms, or copied per firm? The proposal is global reference data with firm-level overrides (**[MAKE CONFIGURABLE]**).
