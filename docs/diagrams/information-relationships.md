# Diagram — Information Relationships (Conceptual ERD)

Conceptual only. Names are domain concepts, not table names. Attributes shown are illustrative key attributes.

## 1. Core conceptual ERD

```mermaid
erDiagram
    ENTITY ||--o{ ENTITY_RELATIONSHIP : "relates to"
    ENTITY ||--o| CLIENT_PROFILE : "may be a client"
    ENTITY ||--o{ NAME_VARIANT : "known as"
    ENTITY ||--o{ IDENTIFIER : "NIC / passport / reg no"

    ENQUIRY }o--o{ ENTITY : "involves"
    ENQUIRY ||--|{ CONFLICT_CHECK : "requires"
    CONFLICT_CHECK ||--o{ CONFLICT_MATCH : "finds"
    CONFLICT_MATCH }o--|| ENTITY : "points to"
    CONFLICT_CHECK ||--|{ CONFLICT_DECISION : "decided by human"
    ENQUIRY ||--o| MATTER : "becomes"

    MATTER }o--|{ CLIENT_PROFILE : "for client(s)"
    MATTER ||--|{ MATTER_MEMBER : "team"
    MATTER_MEMBER }o--|| USER : "is"
    MATTER }o--o{ MATTER : "related to"
    MATTER ||--o{ PROCEEDING : "contains"
    MATTER ||--o{ DOCUMENT : "owns"
    MATTER ||--o{ TASK : "anchors"
    MATTER ||--o{ COMMUNICATION : "records"

    COURT ||--o{ PROCEEDING : "hears (current)"
    PROCEEDING_TYPE ||--o{ PROCEEDING : "templates"
    PROCEEDING ||--|{ CASE_NUMBER_RECORD : "numbered (history)"
    PROCEEDING }o--o{ PROCEEDING : "appeal / revision / transfer"
    PROCEEDING ||--o{ PARTY_ROLE : "has parties"
    PARTY_ROLE }o--|| ENTITY : "played by"
    PARTY_ROLE ||--o{ REPRESENTATION : "represented by"
    REPRESENTATION }o--|| ENTITY : "lawyer"
    PROCEEDING ||--o{ HEARING : "listed as"
    PROCEEDING ||--o{ EVIDENCE_ITEM : "evidence"

    HEARING ||--|{ DATE_ASSERTION : "date claims"
    HEARING ||--o{ VERIFICATION : "verified by"
    HEARING ||--o| OUTCOME : "result"
    HEARING ||--o| HEARING : "next hearing"
    HEARING }o--o{ USER : "appearing lawyer"
    HEARING ||--o{ CHECKLIST_ITEM : "preparation"

    DOCUMENT ||--|{ DOCUMENT_VERSION : "versions"
    DOCUMENT }o--o{ PROCEEDING : "linked"
    DOCUMENT }o--o{ HEARING : "needed for"
    DOCUMENT }o--o{ EVIDENCE_ITEM : "copy of"
    EVIDENCE_ITEM ||--o{ CUSTODY_EVENT : "custody log"

    TASK }o--o{ HEARING : "linked"
    TASK }o--o{ DOCUMENT : "linked"
    TASK }o--|| USER : "owner"
    TASK }o--o{ TASK : "depends on"

    COMMUNICATION }o--o{ ENTITY : "participants"

    USER ||--o{ NOTIFICATION : "receives"
    USER }o--o{ ETHICAL_WALL : "excluded by"
    ETHICAL_WALL }o--o{ MATTER : "protects"

    LEGAL_HOLD }o--o{ MATTER : "holds"
    LEGAL_HOLD }o--o{ DOCUMENT : "holds"
    AUDIT_EVENT }o--|| USER : "actor"
```

## 2. Simplified "spine" view for non-technical review

```mermaid
flowchart TB
    P[People & organisations<br/>one record each] -->|client role| C[Client]
    P -->|party role per case| PR
    C --> M[MATTER<br/>client's legal problem]
    M --> PR[Proceedings<br/>each court case, own case no.]
    PR -->|appeal / revision / transfer| PR
    PR --> H[Hearings<br/>date + source + verified?]
    H --> O[Outcome] --> H2[Next hearing]
    M --> D[Documents & evidence]
    D -. linked to .-> PR
    D -. linked to .-> H
    M --> T[Tasks]
    T -. linked to .-> H
    M --> CM[Communications<br/>updates · instructions]
    M --> TM[Team & access]
    G[Audit log · legal hold · retention] -. governs .-> M
```

## 3. Hearing date provenance detail

```mermaid
erDiagram
    HEARING ||--|{ DATE_ASSERTION : has
    DATE_ASSERTION {
        localdate date
        string session_or_time
        enum source_type
        string source_detail
        enum authority
        user asserted_by
        timestamp asserted_at
    }
    HEARING ||--o{ VERIFICATION : has
    VERIFICATION {
        ref assertion_verified
        enum authoritative_source
        string evidence_reference
        user verified_by
        timestamp verified_at
        bool revoked
    }
    HEARING {
        enum listing_status
        enum verification_state_derived
        enum preparation_status
        string purpose
    }
```
