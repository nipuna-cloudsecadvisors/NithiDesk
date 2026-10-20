# 0000. Record architecture decisions

- **Status:** Proposed
- **Date:** 2026-10-13
- **Issue:** — (enables #17–#24)
- **Deciders:** product owner
- **Related:** [doc 18 §4 decision gates](../18-product-roadmap.md), [planning summary §11](../../NITHIDESK_PLANNING_SUMMARY.md)

## Context

The planning summary (§11) lists architecture *directions* and names several choices as hard to reverse: the single entity registry, the matter–proceeding graph, case-number history, hearing date assertions, local-date semantics, tenant ID on every record, central authorisation, and bilingual name fields with normalised search shadows. Gate G0 requires these to be approved as ADRs before M1 feature work starts. The team will be small (2–3 engineers, **[ASSUMPTION]**), and the reasons behind each decision must outlive any one person.

## Decision drivers

- Make the reason for each hard-to-reverse choice findable later.
- Give the owner, who is not necessarily an engineer, a clear approve/reject point.
- Keep the overhead low.

## Considered options

1. **MADR-style Markdown ADRs in `docs/adr/`, one per decision, approved by pull request.**
   - Good: lives next to the code and planning docs; reviewed like code; diffable.
   - Bad: needs discipline to keep the index current.
2. **Decisions recorded only in GitHub issues or discussions.**
   - Good: zero setup.
   - Bad: hard to find later; not versioned with the code; comment threads mix discussion and outcome.
3. **A wiki or external document tool.**
   - Good: easy for non-engineers to edit.
   - Bad: drifts from the code, and edit history is weaker.

## Decision

Use option 1. Each ADR follows the template in [README](README.md), starts as **Proposed**, and becomes **Accepted** when the owner approves the pull request that changes its status. Accepted ADRs are never rewritten; they are superseded. Each ADR links to its GitHub issue, and the issue is closed on acceptance.

## Consequences

- Positive: Gate G0 has a concrete, checkable artefact.
- Negative: one more document per decision.
- Follow-up: the CI documentation check (M1 platform epic) should verify that the ADR index lists every ADR file.

## Confirmation

All ADRs 0001–0008 are either Accepted or explicitly deferred before M1 feature work starts (Gate G0).

## Open questions

- Who besides the owner must approve security-relevant ADRs (e.g. an external security reviewer)? **[DECISION REQUIRED]**
