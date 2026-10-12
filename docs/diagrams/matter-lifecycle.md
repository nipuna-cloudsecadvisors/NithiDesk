# Diagram — Matter Lifecycle

## 1. Enquiry → Matter (separate entities)

```mermaid
stateDiagram-v2
    [*] --> New: Enquiry recorded
    New --> ConflictPending: Conflict search run
    ConflictPending --> UnderReview: Matches to review
    UnderReview --> Cleared: Human decision
    UnderReview --> NotCleared: Human decision
    Cleared --> Accepted: Engagement approved
    Cleared --> Declined: Firm declines / refers
    NotCleared --> Declined
    Accepted --> [*]: Matter created
    Declined --> [*]: Minimal record kept for future conflicts
```

## 2. Matter lifecycle status

```mermaid
stateDiagram-v2
    [*] --> Open: Created from accepted enquiry
    Open --> OnHold: Instructions awaited / client paused (reason)
    OnHold --> Open: Resumed
    Open --> ClosureRequested: Lead lawyer requests closure
    ClosureRequested --> Open: Checklist fails / partner rejects
    ClosureRequested --> Closed: Partner approves
    Closed --> Open: Reopened (partner, reason)
    Closed --> Archived: Retention period begins / auto after N days
    Archived --> Disposed: Retention expired + no legal hold + partner approval
    Disposed --> [*]: Parties & summary kept for conflicts
    note right of Archived
        Legal hold blocks disposal
        and any deletion
    end note
```

## 3. Phase labels (independent of lifecycle)

```mermaid
flowchart LR
    PL[Pre-litigation<br/>demand · negotiation] --> LI[Litigation<br/>original court]
    LI --> AP[Appeal<br/>HCCA / CA / SC]
    LI --> SE[Settlement]
    AP --> SE
    AP --> EN[Enforcement /<br/>execution]
    LI --> EN
    SE --> PJ[Post-judgment /<br/>monitoring]
    EN --> PJ
    ADV[Advisory / documentation<br/>no proceedings]
```

Phase is a configurable label; attention flags (unverified dates, overdue tasks, missing documents, inactivity) are derived automatically and never typed by users.

## 4. Matter containing multiple proceedings over time

```mermaid
gantt
    dateFormat  YYYY-MM-DD
    title Example — Matter ND-2026-0142 "Perera v. Silva — land at Kotte"
    section District Court
    DC/L/1234 original action         :done, dc, 2021-03-01, 2024-06-15
    section Civil Appellate HC
    HCCA appeal (final appeal)        :done, hc, 2024-07-10, 2025-11-20
    section Supreme Court
    Leave to appeal application       :active, sc1, 2025-12-15, 2026-08-01
    SC Appeal (leave granted)         :sc2, 2026-08-01, 2027-06-30
    section District Court
    Execution of decree (stayed)      :crit, ex, 2024-07-01, 2027-06-30
```

All proceedings keep their own case numbers, parties-with-roles, hearings and documents, under one matter.
