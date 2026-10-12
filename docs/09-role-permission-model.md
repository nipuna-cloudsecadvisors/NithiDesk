# 09 — Role & Permission Model

Covers PART 22 (roles, matter-level access, ethical walls, document restrictions, admin privileges) at the authorisation level. Broader security controls: doc 10.

---

## 1. Model: four layers evaluated together

```
Access allowed  =  Role capability (what kind of action can this person ever do?)
                AND Matter access (is this person allowed into this matter?)
                AND Record restriction (is this document/note/field restricted further?)
                AND NOT Ethical wall (is this person walled off?)          ← overrides everything
```

| Layer | Question | Examples |
|---|---|---|
| 1. **Role capabilities** (RBAC) | Can a person with this role *ever* perform this action? | Clerk can verify hearing dates; assistant can upload; only partners decide conflicts |
| 2. **Matter access** (ACL) | Is this person a member of / granted access to this matter? | Junior on Matter A, not Matter B |
| 3. **Record restriction** | Is this specific item more restricted than the matter? | Privileged-internal memo; highly restricted evidence; NIC field masked |
| 4. **Ethical wall** (deny) | Is this person explicitly excluded? | Lawyer who previously acted for the opponent |

Deny wins. Ethical walls apply to every role **including managing partner and system administrator** (**[NEEDS LAWYER CONFIRMATION]** whether the firm wants any exception).

---

## 2. Role templates (configurable)

Firms assign *titles* freely; each user gets one or more **role templates**. **[MAKE CONFIGURABLE]** — templates can be cloned and edited.

| Template | Default matter scope | Key capabilities |
|---|---|---|
| Managing Partner | **All non-walled matters** (configurable) | All business actions; final conflict decisions; walls; approve closure/exports; view firm reports |
| Partner | Own + team matters (configurable: all matters read-only) | Conflict decision; engagement; client-update approval; closure approval; assign teams |
| Senior Lawyer | Member matters | Create proceedings/hearings; record outcomes; verify dates; manage documents; draft updates; assign tasks |
| Junior Lawyer | Member matters + **appearance-scoped** | Record outcomes; add notes; upload; create tasks; verify (configurable) |
| Legal Assistant | Member matters | Create enquiries/clients; run conflict search; upload/categorise documents; tasks |
| Court Clerk | **Diary scope**: all matters' hearing metadata (configurable to assigned courts/matters) | Verify dates; court-list check; registry tasks; upload court documents; **no access to privileged-internal docs or notes by default** |
| Admin Staff | Directory only | Create enquiries; search client contacts; see matter existence + responsible lawyer; log calls |
| Finance (Phase 2) | Billing data of all matters | Fee notes, payments; no documents |
| System Administrator | **None** (no matter content) | Users, roles, config, reference data, security logs |
| External Counsel | Explicit grants only, time-boxed | Read briefed proceeding/hearing/documents; optional notes/outcome |
| Read-only Reviewer | Explicit grants, time-boxed | Read; no download by default |
| Client (Phase 3) | Own matters, client-visible items only | Portal |

### 2.1 Clerk "diary scope" — an important special case

Clerks need to see *every* hearing (case number, court, parties, date) to protect the diary, but not the substance of every matter. Therefore a **diary scope** grants access to: proceeding metadata, party names, hearing records, court documents category — but **not** internal notes, privileged documents, communications or strategy. **[MAKE CONFIGURABLE]** whether clerks are firm-wide or court-assigned.

### 2.2 Appearance-scoped access

When a lawyer who is not on the matter is assigned to appear at a hearing (common for juniors covering), they automatically receive **time-boxed access**: the proceeding, that hearing, the previous hearing's outcome, documents flagged for the hearing/brief — from assignment until e.g. 48 hours after the hearing (**[MAKE CONFIGURABLE]**). The grant is audited, visible to the responsible partner, and blocked by any ethical wall (assignment fails with an explanation).

---

## 3. Matter confidentiality levels

| Level | Who has access | Use |
|---|---|---|
| **Standard** | Matter team + roles with firm-wide scope (managing partner, partners if configured, clerks' diary scope) | Most matters |
| **Restricted** | Matter team only + managing partner; clerks see diary metadata only with masked title ("Restricted matter — DC/xxx") (**[MAKE CONFIGURABLE]**) | Family, criminal, high-profile, employee matters |
| **Highly restricted** | Named individuals only; even managing partner must be named; clerks see only date + court + case number | Matters involving firm staff, politically exposed persons, sensitive investigations |

Changing level is a partner action, audited, and re-notifies affected users.

---

## 4. Record-level restrictions

| Item | Restriction options |
|---|---|
| Document | Standard · Restricted (team leads only) · Privileged-internal (lawyers on team only; never clients/external counsel unless explicitly shared) · Highly restricted (named users) |
| Notes | Team · Lawyers only · Author + partner |
| Communications | Follow matter; "Advice given" can be lawyers-only |
| Sensitive fields | NIC/passport masked except for roles with "view identifiers"; full value reveal is audited |
| Conflict information | Approvers + responsible partner |
| Financial data | Finance + partners |

---

## 5. Ethical walls

| Aspect | Design |
|---|---|
| Created by | Managing partner / partner with "manage walls" capability; from a conflict decision ("Cleared with conditions") or manually |
| Content | Protected matter(s) (or client, extending to all their matters); excluded users; reason; start date; review date |
| Effect | Excluded users cannot see the matter in any list, search, diary, report, notification, AI answer or conflict-detail view; existence hidden (**[MAKE CONFIGURABLE]**: show "restricted" placeholder in diary to avoid double-booking the walled user) |
| Assignment protection | System refuses to add an excluded user to the matter team or a hearing appearance |
| Visibility | Excluded user is told they are subject to a wall (not the details) — **[NEEDS LAWYER CONFIRMATION]** practice |
| Audit | Creation, change, removal; any denied access attempt logged and alerted |
| Removal | Two-person approval (**[MAKE CONFIGURABLE]**) |

---

## 6. Capability matrix (default templates — illustrative)

Legend: ✔ allowed · M within member/granted matters · D diary scope · ✖ not allowed · A approve

| Capability | MP | Partner | Senior | Junior | Assistant | Clerk | Admin staff | SysAdmin | Ext. counsel |
|---|---|---|---|---|---|---|---|---|---|
| Create enquiry | ✔ | ✔ | ✔ | ✔ | ✔ | ✖ | ✔ | ✖ | ✖ |
| Run conflict search | ✔ | ✔ | ✔ | ✖ | ✔ | ✖ | ✖ | ✖ | ✖ |
| Decide conflict | A | A | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |
| Accept engagement / open matter | A | A | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |
| View matter | all* | M/all* | M | M | M | D | existence | ✖ | grant |
| Edit matter core fields | ✔ | M | M | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |
| Manage team / access | ✔ | M | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |
| Create proceeding | ✔ | M | M | M | M | D | ✖ | ✖ | ✖ |
| Create / edit hearing | ✔ | M | M | M | M | D | ✖ | ✖ | ✖ |
| Verify hearing date | ✔ | M | M | config | ✖ | D | ✖ | ✖ | ✖ |
| Record outcome | ✔ | M | M | M | ✖ | config | ✖ | ✖ | grant |
| View documents | all* | M | M | M | M | court docs (D) | ✖ | ✖ | shared |
| Download documents | ✔ | M | M | M | M | court docs | ✖ | ✖ | config |
| Upload documents | ✔ | M | M | M | M | D | ✖ | ✖ | config |
| Delete (soft) document | ✔ | M | own uploads | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |
| Approve client update | ✔ | M | config | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |
| Close matter | A | A | request | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |
| Manage walls | ✔ | config | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |
| Export data (bulk) | A | request | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |
| Firm reports | ✔ | portfolio | own | own | ✖ | diary | ✖ | system | ✖ |
| Manage users / roles | approve | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ | ✔ (execute) | ✖ |
| Reference data (courts etc.) | ✔ | ✖ | ✖ | ✖ | ✖ | suggest | ✖ | ✔ | ✖ |
| View security audit log | ✔ | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ | ✔ | ✖ |
| Break-glass access | request | request | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ | ✖ |

\* "all" = all non-walled matters at Standard level, subject to configuration.

---

## 7. Break-glass access

For emergencies (lead lawyer unreachable, hearing in one hour): an authorised user requests access to a matter they are not on → states a reason → access granted immediately for a short window (e.g. 4 hours) **unless** an ethical wall applies → responsible partner and managing partner notified instantly → logged and reviewed. **[MAKE CONFIGURABLE]** whether a second approval is required first.

---

## 8. Enforcement principles (for later engineering)

- Authorisation decided **server-side** in one policy component; the UI only hides what the API would refuse.
- Every query for lists/search joins against the user's accessible-matter set (prevents IDOR and data leaks via search, reports, notifications, exports, AI).
- Object IDs are non-guessable, but security never relies on that.
- Permission tests are mandatory for every endpoint (horizontal/vertical escalation, wall bypass).
- Access changes take effect immediately (cached permission sets invalidated on change).
