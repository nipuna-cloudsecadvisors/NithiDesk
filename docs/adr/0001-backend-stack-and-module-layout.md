# 0001. Backend stack and module layout

- **Status:** Proposed
- **Date:** 2026-10-13
- **Issue:** #17
- **Deciders:** product owner (approves); lead engineer (proposes)
- **Related:** [summary §11](../../NITHIDESK_PLANNING_SUMMARY.md), [doc 08 §1 bounded contexts](../08-information-model.md), [doc 10](../10-security-plan.md), [doc 18](../18-product-roadmap.md)

## Context

The owner's direction is Go and PostgreSQL. The product needs strong consistency (a hearing date and its verification must never disagree), a central authorisation policy applied inside every query (doc 09, doc 10 §5), and a small team. Doc 08 §1 defines 14 bounded contexts: Identity & Access, Registry, Intake, Matters, Courts, Proceedings, Hearings, Documents, Work, Communications, Notifications, Governance, and later AI and Integration.

**What exists already (untracked scaffold, not yet in Git):**
- `cmd/api`: HTTP server with graceful shutdown, `/healthz`, `/readyz` (checks DB and object storage), `/version`.
- `cmd/worker`: a Postgres-backed queue loop. It imports `internal/worker`, which is **missing** from the working copy, so the scaffold does not currently build.
- `internal/config`: environment-only configuration with redaction. `internal/observability`: `log/slog` JSON logging with an identifiers-only policy. `internal/httpapi`: standard-library `net/http` `ServeMux` with method patterns, plus middleware for request ID, logging, recover and security headers. `internal/platform/db`: `pgxpool`, no ORM. `internal/platform/objectstore`: `minio-go` against the S3 API. `internal/authz`: a list of required authorisation bypass scenarios.
- `go.mod`: module `github.com/nipuna-cloudsecadvisors/NithiDesk`, `go 1.27`, with dependencies `pgx/v5` and `minio-go/v7`.

The scaffold references "Release 1 / Foundation 1" and a spec "CLO-2", which are not in this repository. **[DECISION REQUIRED]** Either bring that spec into `docs/` or remove the references.

## Decision drivers

- Correctness and consistency over raw scale (one firm, tens of users first).
- Authorisation enforced in one place and testable.
- Small team: few moving parts, simple deployment.
- Safe, parameterised SQL; explicit transactions.
- Module boundaries that could later be split if ever needed.

## Considered options

1. **Modular monolith: one Go module, two binaries (api, worker), packages per bounded context.**
2. **Microservices per bounded context.**
3. **A Go web framework or ORM-centric stack (e.g. Gin/Echo + GORM, or an all-in-one framework).**

### Option 1: modular monolith
- Good: one deployable, one database transaction across contexts where needed (e.g. outcome → next hearing → task → notification); simplest operations.
- Good: matches the existing scaffold.
- Bad: needs discipline so contexts don't reach into each other's tables; this is enforced by package rules and lint.

### Option 2: microservices
- Good: independent scaling and deployment.
- Bad: distributed transactions for the hearing chain; authorisation duplicated across services; far too much operational load for 2–3 engineers. Premature.

### Option 3: framework/ORM stack
- Good: quick CRUD.
- Bad: ORMs hide queries, which makes it hard to guarantee the matter filter is in every query. Go's standard-library router has supported method and path patterns since Go 1.22, so a third-party router adds little.

## Decision

**Option 1.** Specifically:

- **Layout:**
  ```
  cmd/api, cmd/worker
  internal/<context>/        one package per doc 08 context (identity, registry, intake, matters,
                             courts, proceedings, hearings, documents, work, communications,
                             notifications, governance)
  internal/authz/            central policy; every context calls it, none bypass it
  internal/platform/         db, objectstore, queue, clock, i18n, pdf: technical adapters only
  internal/httpapi/          transport only; no business logic
  ```
- **Rules:**
  - A context exposes a Go interface. Another context never imports its storage package or queries its tables directly.
  - Cross-context reads go through the owning context's service.
- **HTTP:** keep the standard-library `ServeMux` that the scaffold uses. The planning summary suggested chi; it is not needed unless routing needs grow.
- **Data access:** `pgx/v5` with **sqlc**-generated query code (**[ASSUMPTION]**, to be confirmed in the first M1 slice); hand-written SQL migrations with a versioned migration tool, chosen in M1. No ORM.
- **Background jobs:** a Postgres-backed queue (`SELECT … FOR UPDATE SKIP LOCKED`) in the worker binary. Redis is not used in MVP.
- **Time:** court dates are stored as `date`, not timestamps (local-date semantics). Times are `timestamptz`, rendered in `Asia/Colombo`.
- **Go version:** the scaffold declares `go 1.27`. Go publishes a major release roughly every six months (1.26 in February 2026, 1.27 expected around August 2026), so 1.27 should exist by October 2026. **This has not been verified:** Go is not installed on the planning machine. The M1 platform epic must confirm the toolchain and pin it with a `toolchain` line.

## Consequences

- Positive: transactional integrity for the hearing chain; one authorisation layer; simple deployment.
- Negative: module discipline must be enforced. Add an import-boundary lint (e.g. `depguard` rules) to CI.
- Follow-up:
  - Restore or remove `internal/worker` so the scaffold builds.
  - Commit the scaffold on a branch labelled as a Phase 0 scaffold, per Gate G0.
  - Add `sqlc` and a migration tool in M1.

## Confirmation

- CI: `go build ./...`, `go vet`, `staticcheck`/`golangci-lint`, import-boundary lint, and the `internal/authz` scenario suite as a merge gate.
- Review: no package outside `internal/<context>` queries that context's tables.

## Open questions

- Should sqlc be adopted, or hand-written pgx queries kept? Decide after the first M1 slice.
- Source and status of the "CLO-2" spec referenced in the scaffold. **[DECISION REQUIRED]**
