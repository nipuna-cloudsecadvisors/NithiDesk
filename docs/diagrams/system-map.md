# Diagram — System Map

## 1. System-level workflow (the operating loop)

```mermaid
flowchart LR
    E[Enquiry] --> CC[Conflict check<br/>+ human decision]
    CC -->|cleared| ON[Engagement &<br/>client onboarding]
    CC -->|declined| X[Enquiry closed<br/>record kept]
    ON --> M[Matter opened<br/>team assigned]
    M --> P["Proceeding(s)<br/>court · case no. · parties"]
    M --> D[Documents &<br/>evidence]
    M -->|advisory / non-litigation| CL
    P --> H[Hearing<br/>date + source + verification]
    H --> PR[Preparation<br/>checklist · brief · documents]
    PR --> CD[Court day<br/>mobile court mode]
    CD --> O[Outcome recorded]
    O --> NH[Next hearing<br/>created from order]
    NH --> H
    O --> T[Follow-up tasks]
    O --> CU[Client update<br/>drafted · approved · logged]
    O --> AP{Final order /<br/>judgment?}
    AP -->|appeal / revision| P2[New linked proceeding<br/>same matter]
    P2 --> H
    AP -->|settled / concluded| CL[Matter closure<br/>checklist]
    CL --> AR[Archive · retention<br/>legal hold]
```

## 2. Module map

```mermaid
flowchart TB
    subgraph Core["Core domain"]
        REG[Person & Organisation<br/>registry]
        CLI[Clients &<br/>onboarding]
        CON[Intake &<br/>conflict checking]
        MAT[Matters]
        CRT[Court master data]
        PRO[Proceedings &<br/>parties]
        HEA[Hearings &<br/>court diary]
        DOC[Documents &<br/>evidence]
        TSK[Tasks &<br/>checklists]
        COM[Client<br/>communications]
    end

    subgraph Experience["Experience layer"]
        TODAY[Today / daily brief]
        WEEK[Weekly planner]
        COURT[Court-day mode<br/>mobile]
        SRCH[Global search]
        TL[Matter timeline]
        RPT[Reports & dashboards]
        PORTAL[Client portal<br/>later]
    end

    subgraph Platform["Platform services"]
        IAM[Identity, MFA,<br/>sessions]
        AUTHZ[Authorisation<br/>RBAC + matter ACL + walls]
        AUD[Audit log ·<br/>retention & legal hold]
        I18N[Localisation &<br/>Sinhala text services]
        NOTIF[Notifications]
        STORE[Object storage<br/>+ malware scan]
        OCR[OCR pipeline]
        IDX[Search index]
        JOBS[Background jobs]
        CFG[Configuration &<br/>reference data]
    end

    subgraph Edge["Adapters (pluggable)"]
        AIGW[AI gateway<br/>later]
        CCON[Court connectors<br/>later]
        IMP[Importers<br/>CSV · court list]
        MSG[Email / SMS /<br/>WhatsApp providers]
    end

    REG --- CLI & CON & PRO
    CLI --> MAT
    CON --> MAT
    MAT --> PRO --> HEA
    MAT --> DOC & TSK & COM
    HEA --> TSK
    CRT --> PRO & HEA

    Core --> Experience
    Experience --> Platform
    Core --> Platform
    Platform --> Edge
```

## 3. Deployment view (direction, not final)

```mermaid
flowchart LR
    U[Browser / installed PWA<br/>desktop · tablet · phone] -->|HTTPS| API[Go API<br/>modular monolith]
    API --> PG[(PostgreSQL<br/>relational + FTS + trigram)]
    API --> OBJ[(S3-compatible<br/>object storage, private)]
    API --> Q[Job queue]
    Q --> W[Go worker<br/>scan · OCR · index · notify · digest]
    W --> PG
    W --> OBJ
    W --> AV[Malware scanner]
    W --> OCRE[OCR engine]
    W --> MAIL[Email / SMS provider]
    API -. later .-> AIG[AI gateway]
    W -. later .-> GOV[Court connector adapters]
```

Legend: solid = MVP; dashed = later phase. See [08-information-model.md](../08-information-model.md) and the summary's architecture section.
