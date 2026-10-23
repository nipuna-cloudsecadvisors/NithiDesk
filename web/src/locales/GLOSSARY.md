# UI glossary — Sinhala terms used in `si.json`

Source: docs/11-localization-plan.md §3. Every term is **[NEEDS LAWYER CONFIRMATION]**;
the entries marked ⚠ are the ones the translator was least sure of and should be
reviewed first. Dual-label mode ("සිංහල / English") is applied to labels and terms
(`tl()` in code), not to sentences.

| English (UI) | Sinhala used | Status / note |
|---|---|---|
| Case number | නඩු අංකය | From doc 11 glossary |
| Court | අධිකරණය | From doc 11 glossary |
| Next court date / next date | ඊළඟ නඩු දිනය / ඊළඟ දිනය | From doc 11 glossary |
| Client | සේවාදායකයා | ⚠ doc 11 asks: some firms may prefer පක්ෂකරු |
| Lawyer | නීතිඥ | From doc 11 glossary |
| Plaintiff / Defendant | පැමිණිලිකරු / විත්තිකරු | From doc 11 glossary (also seeded party-role labels) |
| Petitioner / Respondent | පෙත්සම්කරු / වගඋත්තරකරු | From doc 11 glossary |
| Accused | චූදිතයා | From doc 11 glossary |
| Judgment / Order | තීන්දුව / නියෝගය | From doc 11 glossary |
| Verified / Not verified | තහවුරු කළ / තහවුරු කර නැත | From doc 11 glossary |
| Dates disagree (conflict) | දින එකඟ නැත | ⚠ new term |
| Hearing (one listing on a date) | නඩු දිනය (pl. නඩු දින) | ⚠ doc 11 proposes විභාගය / නඩු විභාගය for hearing/trial; the UI uses "නඩු දිනය" for a listing so that "නඩු විභාගය" stays reserved for *trial* (a hearing purpose). Confirm. |
| Trial / Further trial | නඩු විභාගය / වැඩිදුර නඩු විභාගය | From seeded purposes |
| Matter | කාර්යය (pl. කාර්ය) | ⚠ no glossary entry; alternatives: "නඩු කටයුත්ත", "ගොනුව". Confirm. |
| Case (proceeding) | නඩුව (pl. නඩු) | ⚠ "Case" is used for a proceeding in the UI |
| Outcome | ප්‍රතිඵලය | ⚠ |
| Adjourned | කල් තැබුවා | Seeded outcome label |
| Not reached | කැඳවූයේ නැත | ⚠ seeded; literal "was not called" |
| Laid by / sine die | නියමිත දිනයක් නොමැතිව | Seeded; English kept in brackets in the UI |
| Journal entry (JE) | ජර්නල් සටහන (ජ.ස.) | ⚠ doc 11: "(confirm)" |
| Registry | රෙජිස්ට්‍රාර් කාර්යාලය | ⚠ |
| Certified copy | සහතික කළ පිටපත | |
| Instructions (senior's brief note) | උපදෙස් | |
| Preparation: Ready / Blocked | සූදානම් / අවහිරයි | |
| Session: Morning / Afternoon | පෙරවරු / පස්වරු | |
| Source authority: authoritative / primary / secondary / hearsay | නිල මූලාශ්‍රය / ප්‍රාථමික / ද්විතීයික / ඇසූ පමණි | ⚠ "hearsay" rendered as "heard only"; avoids the evidence-law term |
| Court-list check | නඩු ලැයිස්තුව පරීක්ෂාව | |
| Cause list / court list | නඩු ලැයිස්තුව | |
| Responsible partner | වගකිව යුතු හවුල්කරු | |
| Attorney on record / Counsel / Watching brief | වාර්තාගත නීතිඥ / නීතිඥ (Counsel) / නිරීක්ෂණ (Watching brief) | ⚠ English kept in brackets — commonly used in English in practice |
| Ethical wall | ආචාරධර්ම බාධකය | ⚠ |
| Two-step sign-in | පියවර දෙකේ පිවිසුම | |
| Poya day | පෝය දිනය | |
| Court vacation | අධිකරණ නිවාඩු කාලය | |
| PW1 (prosecution/plaintiff witness 1) | පැ.සා.1 | ⚠ abbreviation in a hint text only |

Conventions:
- Dates: long form `{year} {month} {day}, {weekday}` (e.g. 2026 නොවැම්බර් 24, අඟහරුවාදා), short form DD/MM/YYYY in both languages; Western digits.
- Conjuncts keep their ZWJ (U+200D): ප්‍ර, ශ්‍ර, ද්‍ය, බ්‍ර. A test checks this.
- Tamil (`ta`) is not translated in M1; add `ta.json` with the same keys and register it in `LOCALES` / `loaders` in `src/i18n/I18nProvider.tsx`.
