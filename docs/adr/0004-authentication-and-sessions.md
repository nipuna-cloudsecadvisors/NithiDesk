# 0004. Authentication and sessions

- **Status:** Proposed
- **Date:** 2026-10-13
- **Issue:** #20
- **Deciders:** product owner
- **Blocked on:** owner decision **#14** (authentication approach) **[DECISION REQUIRED]**; whether the firm uses Microsoft 365 or Google Workspace for email (lawyer questions #47, Current software)
- **Related:** [doc 10 §3–4, §10](../10-security-plan.md), [doc 09](../09-role-permission-model.md), [doc 12](../12-mobile-court-mode.md)

## Context

Doc 10 requires:
- MFA mandatory for every internal user from day one;
- immediate revocation when a user is disabled;
- device/session visibility;
- idle timeouts of 30 min on desktop and 15 min on mobile, with a quick re-unlock (PIN in MVP, passkeys in Phase 2);
- step-up authentication for sensitive actions.

Lawyers use phones in court with poor connectivity, so login friction matters. Clients (Phase 3) will use a separate realm.

## Decision drivers

- Instant revocation (lost phone, departing staff).
- Strong MFA without SMS dependency.
- Low friction on mobile in court.
- Room to add SSO and passkeys later without redesign.
- Few external dependencies for a small team.

## Considered options

1. **Built-in local accounts + server-side sessions (opaque cookie) + TOTP MFA; OIDC-ready; passkeys in Phase 2.**
2. **Stateless JWT access/refresh tokens.**
3. **External identity provider from day one** (self-hosted Keycloak/Zitadel, or the firm's Microsoft Entra ID / Google Workspace via OIDC).

### Option 1: local accounts + server-side sessions
- Good: revocation is immediate (delete the session row); the session list per user is trivial; cookies are `Secure; HttpOnly; SameSite`. The idle-timeout and PIN re-unlock rules are easy to apply server-side.
- Bad: we own password storage (argon2id, breached-password check), MFA enrolment, recovery and lockout. That is security-critical code.

### Option 2: JWT
- Good: no session store.
- Bad: revocation needs a denylist, which turns it back into a session store; tokens in browser storage widen the XSS impact. Wrong fit for an internal app.

### Option 3: external IdP
- Good: MFA, passkeys and recovery handled by a mature product. With Entra or Google, staff use their existing work login.
- Bad: Keycloak or Zitadel is another critical service to run, patch and back up. The firm may not have Entra or Google Workspace, and staff may use personal email (**[ASSUMPTION]**). Revocation is split across two systems.

## Decision

**Option 1 in MVP, designed so option 3 can be added (OIDC) without schema change.**
- **Passwords:** argon2id; minimum 12 characters; breached-password check; no forced rotation.
- **MFA:** TOTP mandatory at first login. Recovery codes plus admin-assisted reset, audited. SMS OTP fallback only if #14 accepts it.
- **Sessions:** server-side in Postgres, opaque 256-bit ID in a cookie.
  - Idle timeout 30 min (desktop) / 15 min (mobile), **[MAKE CONFIGURABLE]**; absolute lifetime 12 h.
  - Mobile PIN re-unlock bound to the session; 5 failures force a full login.
  - Users can see and revoke their own sessions; an admin disable kills all of that user's sessions immediately.
- **Step-up:** fresh MFA within 5 minutes before export, wall or permission changes, break-glass, revealing a full NIC, or downloading highly restricted documents.
- **CSRF:** SameSite plus anti-CSRF tokens on state-changing requests.
- **Identity model:** a `user` has one-or-more `credential`s (password, TOTP, later passkey, later OIDC subject), so SSO and passkeys are added as new credential types.

## Consequences

- Positive: meets doc 10 with minimal infrastructure; revocation is reliable.
- Negative: security-critical code to own. It needs focused review and a pentest before go-live (Gate G3).
- Follow-up: the M1 identity epic; audit events for login, failure, MFA reset and session revoke (doc 10 §9).

## Confirmation

- Automated tests: a disabled user's next request is rejected; an idle-timeout and absolute-lifetime matrix; step-up enforced on each listed action.
- External penetration test before go-live.

## Open questions

- #14: is local + TOTP acceptable, or must staff sign in with an existing Microsoft or Google account? **[DECISION REQUIRED]**
- Is SMS OTP acceptable as a fallback? **[DECISION REQUIRED]**
- Are staff devices firm-owned or personal (BYOD)? This affects device binding. **[NEEDS LAWYER CONFIRMATION]**
