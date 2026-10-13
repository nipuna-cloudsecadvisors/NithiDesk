# 0008. Frontend

- **Status:** Proposed
- **Date:** 2026-10-13
- **Issue:** #24
- **Deciders:** product owner (approves); lead engineer (proposes)
- **Blocked on:** partly spike **#27** (Sinhala UI layout on low-end Android); lawyer question **#8** (Q7: phones and mobile data at court) **[NEEDS LAWYER CONFIRMATION]**
- **Related:** [doc 11 §2](../11-localization-plan.md), [doc 12 mobile & court-day mode](../12-mobile-court-mode.md), [doc 10 §10–11](../10-security-plan.md), [doc 17](../17-mvp-scope.md)

## Context

NithiDesk is an authenticated internal app with no SEO need. It must work well on desktop and on **phones in court**, where connectivity is poor. The MVP includes an **offline-tolerant outcome/note queue**; encrypted offline court packs come in Phase 2. The UI must be Sinhala/English from the start: no hardcoded strings, ICU plurals, dual-label mode, self-hosted Noto Sinhala fonts. Tamil comes later. The team is small, and the backend is Go (ADR 0001).

## Decision drivers

- Mobile court-day usability and an offline queue (service worker, IndexedDB).
- First-class i18n with CI checks (key parity, no hardcoded text).
- Security: strict Content Security Policy (CSP), no third-party scripts, session cookies (ADR 0004).
- A small team's productivity and the hiring pool.
- Low bundle size on low-end Android.

## Considered options

1. **React + TypeScript single-page app built with Vite, installable as a PWA.**
2. **Next.js (React with server rendering).**
3. **Server-rendered Go templates + htmx** (with a small service worker).
4. **SvelteKit (or another lighter SPA framework).**

### Option 1: React + Vite PWA
- Good:
  - largest ecosystem and hiring pool;
  - mature i18n (FormatJS/ICU or i18next with ICU), accessibility and form libraries;
  - a clean separation from the Go API;
  - a straightforward service worker and IndexedDB model for the offline queue;
  - static assets served with a strict CSP.
- Bad: we build the client-side data layer ourselves; the bundle needs to be watched on low-end phones.

### Option 2: Next.js
- Good: React ecosystem plus server rendering.
- Bad: server rendering brings no SEO benefit here and adds a Node runtime to operate and secure. Its server/client split complicates the offline model.

### Option 3: Go templates + htmx
- Good: one language; very small client; no API/SPA split.
- Bad: offline-tolerant outcome capture and the Phase 2 offline packs need substantial client-side logic anyway. Rich mobile interactions are harder, and i18n is split across server and client.

### Option 4: SvelteKit
- Good: small bundles; pleasant developer experience.
- Bad: smaller ecosystem and hiring pool. Its server features add a Node runtime unless it is used purely as a static SPA.

## Decision

**Option 1: React + TypeScript SPA (Vite), installable PWA.**
- **Hosting:** served as static assets behind the same origin as the API, so cookies work and CSP is simple.
- **i18n:** ICU message catalogues per locale (`en`, `si`, later `ta`).
  - Message keys only in components; a CI lint fails on hardcoded user-facing strings.
  - A key-parity check across locales; pseudo-localisation in CI.
  - The API returns codes, not sentences (doc 11).
- **Fonts:** Noto Sans Sinhala and Noto Sans, self-hosted; no Google Fonts CDN at runtime.
- **Offline (MVP):**
  - The service worker caches the app shell only.
  - The outcome/note queue is kept in IndexedDB, holds minimal data, and expires.
  - The queue survives session expiry and replays after re-authentication (doc 12).
  - No client data is cached offline beyond the queue until the Phase 2 encrypted court pack.
- **Design:** mobile-first layouts for Today and court-day mode, as separate views, not a shrunken desktop.
- **Security:** strict CSP (no inline scripts), no third-party analytics, and dependency audit in CI.

## Consequences

- Positive: the best fit for the offline queue and i18n requirements; easy hiring.
- Negative: two codebases (Go and TypeScript); an API contract must be maintained (OpenAPI, from the logical data model and API outline task #29).
- Follow-up: spike #27 validates Sinhala typography and dual-label layouts on a low-end Android device before the design system is fixed.

## Confirmation

- Spike #27 results: readable Sinhala at default sizes, no truncation of dual labels on a 360-px-wide screen, acceptable load time on a low-end device over 3G (budget **[DECISION REQUIRED]**, proposed: Today usable in under 3 s).
- CI: i18n lint and parity checks; Lighthouse PWA and performance checks for the mobile build.

## Open questions

- Which devices and browsers do lawyers and clerks use (Android vs iPhone, versions)? **[NEEDS LAWYER CONFIRMATION]** (#8)
- Does iOS Safari's PWA storage eviction affect the offline queue enough to need a fallback? Test in Phase 0.
