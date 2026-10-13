# 0005. File storage

- **Status:** Proposed
- **Date:** 2026-10-13
- **Issue:** #21
- **Deciders:** product owner
- **Blocked on:** hosting location ([ADR 0003](0003-hosting-and-deployment.md), owner decision #12) for the production backend only; the interface decision is not blocked
- **Related:** [doc 07 §3–7](../07-document-workflow.md), [doc 10 §6–7, §14](../10-security-plan.md)

## Context

Documents include scanned court records, photos taken in court, pleadings and evidence. They arrive from untrusted parties, must be malware-scanned, are versioned, and must never be reachable through a public URL. Doc 07 §6 defines an upload pipeline: quarantine → validate → scan → hash → private storage → preview → OCR. Legal hold and retention (doc 07 §7) must stop deletion. The scaffold already has `internal/platform/objectstore` using the S3 API through `minio-go`.

## Decision drivers

- Confidentiality: private by default, short-lived access, encryption at rest.
- Integrity: originals never overwritten; hash recorded; versioning.
- Legal hold and retention enforceable.
- Portable across hosting options.
- Database stays small enough for fast backup and restore.

## Considered options

1. **S3-compatible object storage** (MinIO locally or on-prem; any S3-compatible cloud service in production), with metadata in PostgreSQL.
2. **Files in PostgreSQL** (`bytea` or large objects).
3. **Local or network filesystem** on the application server.

### Option 1: S3-compatible
- Good: versioning and object lock (WORM) support legal hold and immutable backups; server-side encryption; signed URLs; scales independently; portable through the S3 API.
- Bad: two stores to keep consistent (row ↔ object), so upload needs a two-phase pattern and orphan cleanup.

### Option 2: in PostgreSQL
- Good: transactional with the metadata; a single backup.
- Bad: database size balloons (scans and video), slowing PITR and restores; no signed URLs; streaming large files is awkward.

### Option 3: filesystem
- Good: simple.
- Bad: no native versioning, object lock or signed URLs; ties the system to one server; backups and encryption are hand-built.

## Decision

**Option 1.**
- **Buckets:** separate `quarantine` and `documents` buckets (plus `previews`).
  - `quarantine` is reachable only by the worker.
  - Object keys are random IDs; no client or matter names appear in paths.
- **Versioning** is on for `documents`. Deleting a document version is a soft delete in Postgres; physical purge only through the retention/disposal workflow, never while a legal hold applies. Use object lock (governance mode) where the backend supports it.
- **Integrity:** SHA-256 is computed in the pipeline and stored with the version; it is re-checked on restore drills.
- **Access:**
  - Download URLs are issued only after an `authz` check, as signed URLs valid for 60–300 s (**[MAKE CONFIGURABLE]**).
  - For highly restricted documents, stream the file through the API instead of issuing a URL.
  - Previews are separate objects, so users can view without downloading.
- **Encryption:** server-side encryption at rest. Consider envelope encryption with per-matter data keys for highly restricted documents (decide in M3).
- **Consistency:** write the object first, then commit the metadata. A worker job removes orphan objects older than 24 h.

## Consequences

- Positive: matches the existing scaffold; meets the legal-hold, integrity and privacy requirements; backend can change with hosting.
- Negative: two-store consistency and an orphan sweeper to maintain.
- Follow-up: M3 documents epic (pipeline, ClamAV-class scanner, previews); backup of object storage with versioning and an immutable offsite copy (doc 10 §14).

## Confirmation

- Tests: no unauthenticated or expired URL returns content; a quarantined object is never served; a document under legal hold cannot be purged.
- Restore drill verifies stored hashes.

## Open questions

- Production backend, which follows #12 (managed S3-compatible service vs self-hosted MinIO). **[DECISION REQUIRED]**
- Retention periods per document class. **[NEEDS LAWYER CONFIRMATION]**
