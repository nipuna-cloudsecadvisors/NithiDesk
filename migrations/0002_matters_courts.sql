-- 0002 Reference data, registry (thin), matters, walls, proceedings, parties.
-- See docs/design/logical-data-model.md §4–7 and docs/05-court-proceeding-model.md.

-- ---------------------------------------------------------------------------
-- Configurable reference data (doc 05 §8, doc 17 §7).
--
-- Simple catalogues share one table keyed by kind. Business code reads the
-- typed attributes it needs (for example a date source's default authority);
-- the attribute shape per kind is validated in internal/courts. Rows are
-- end-dated, never deleted, so history always resolves.
-- ---------------------------------------------------------------------------

create table reference_item (
    id          uuid primary key default uuidv7(),
    firm_id     uuid not null references firm (id),
    kind        text not null check (kind in ('court_level', 'practice_area', 'proceeding_type', 'party_role_type',
                                              'hearing_purpose', 'outcome_type', 'relationship_type',
                                              'date_source_type')),
    code        text not null check (code ~ '^[a-z0-9][a-z0-9_]{0,60}$'),
    name_en     text not null,
    name_si     text,
    name_ta     text,
    sort_order  integer not null default 100,
    attributes  jsonb not null default '{}'::jsonb,
    valid_from  date not null default current_date,
    valid_to    date,
    created_at  timestamptz not null default now(),
    updated_at  timestamptz not null default now(),
    row_version integer not null default 1,
    unique (firm_id, id),
    unique (firm_id, kind, id),
    unique (firm_id, kind, code)
);
create trigger reference_item_touch before update on reference_item for each row execute function nd_touch_row();

create table court (
    id                   uuid primary key default uuidv7(),
    firm_id              uuid not null references firm (id),
    code                 text not null check (code ~ '^[A-Z0-9][A-Z0-9-]{1,40}$'),
    court_level_id       uuid not null,
    court_level_kind     text not null default 'court_level' check (court_level_kind = 'court_level'),
    name_en              text not null,
    name_si              text,
    name_ta              text,
    location_en          text,
    location_si          text,
    address_en           text,
    address_si           text,
    -- [{"pattern": "^DC/L/\\d+$", "example": "DC/L/1234"}]; used to warn, never to block.
    case_number_patterns jsonb not null default '[]'::jsonb,
    sort_order           integer not null default 100,
    valid_from           date not null default current_date,
    valid_to             date,
    created_at           timestamptz not null default now(),
    updated_at           timestamptz not null default now(),
    row_version          integer not null default 1,
    unique (firm_id, id),
    unique (firm_id, code),
    foreign key (firm_id, court_level_kind, court_level_id) references reference_item (firm_id, kind, id)
);
create trigger court_touch before update on court for each row execute function nd_touch_row();

create table courtroom (
    id         uuid primary key default uuidv7(),
    firm_id    uuid not null references firm (id),
    court_id   uuid not null,
    label      text not null,
    valid_to   date,
    created_at timestamptz not null default now(),
    unique (firm_id, id),
    unique (firm_id, court_id, label),
    foreign key (firm_id, court_id) references court (firm_id, id)
);

-- Holidays, Poya days, vacations and ad-hoc non-sitting days. court_id null
-- means every court.
create table court_calendar_day (
    id         uuid primary key default uuidv7(),
    firm_id    uuid not null references firm (id),
    court_id   uuid,
    day        date not null,
    kind       text not null check (kind in ('public_holiday', 'poya', 'vacation', 'not_sitting')),
    label_en   text,
    label_si   text,
    created_at timestamptz not null default now(),
    foreign key (firm_id, court_id) references court (firm_id, id)
);
create unique index court_calendar_day_unique on court_calendar_day (firm_id, coalesce(court_id, '00000000-0000-0000-0000-000000000000'::uuid), day);

-- ---------------------------------------------------------------------------
-- Registry: people and organisations, thin in M1 (doc 04 §1, ADR 0006).
-- Raw Sinhala is never trigram-indexed (pg_trgm produces no trigrams for it);
-- the Latin shadows maintained by internal/platform/textnorm are.
-- ---------------------------------------------------------------------------

create table entity (
    id             uuid primary key default uuidv7(),
    firm_id        uuid not null references firm (id),
    kind           text not null check (kind in ('person', 'organisation', 'government_body')),
    name_en        text,
    name_si        text,
    name_ta        text,
    name_norm      text not null,
    name_latin     text not null,
    name_key       text not null,
    name_skeleton  text not null,
    initials       text not null default '',
    textnorm_rules integer not null,
    merged_into_id uuid,
    created_at     timestamptz not null default now(),
    created_by     uuid,
    created_via    text not null default 'user' check (created_via in ('user', 'import', 'system')),
    updated_at     timestamptz not null default now(),
    updated_by     uuid,
    row_version    integer not null default 1,
    unique (firm_id, id),
    check (coalesce(name_en, name_si, name_ta) is not null),
    foreign key (firm_id, merged_into_id) references entity (firm_id, id)
);
create index entity_latin_trgm on entity using gin (name_latin gin_trgm_ops);
create index entity_key_trgm on entity using gin (name_key gin_trgm_ops);
create index entity_norm on entity (firm_id, name_norm);
create trigger entity_touch before update on entity for each row execute function nd_touch_row();

-- ---------------------------------------------------------------------------
-- Matters (doc 04 §4)
-- ---------------------------------------------------------------------------

create table matter (
    id                 uuid primary key default uuidv7(),
    firm_id            uuid not null references firm (id),
    matter_number      text not null,
    title_en           text,
    title_si           text,
    title_ta           text,
    title_latin        text not null default '',
    description        text,
    practice_area_id   uuid,
    practice_area_kind text not null default 'practice_area' check (practice_area_kind = 'practice_area'),
    lifecycle          text not null default 'open' check (lifecycle in ('open', 'on_hold', 'closure_requested',
                                                                          'closed', 'archived')),
    priority           text not null default 'normal' check (priority in ('normal', 'high', 'urgent')),
    confidentiality    text not null default 'standard' check (confidentiality in ('standard', 'restricted',
                                                                                     'highly_restricted')),
    opened_on          date not null default current_date,
    closed_on          date,
    physical_file      text,
    created_at         timestamptz not null default now(),
    created_by         uuid,
    created_via        text not null default 'user' check (created_via in ('user', 'import', 'system')),
    import_batch_id    uuid,
    updated_at         timestamptz not null default now(),
    updated_by         uuid,
    row_version        integer not null default 1,
    deleted_at         timestamptz,
    deleted_by         uuid,
    delete_reason      text,
    unique (firm_id, id),
    unique (firm_id, matter_number),
    check (coalesce(title_en, title_si, title_ta) is not null),
    check ((closed_on is not null) = (lifecycle in ('closed', 'archived'))),
    check ((deleted_at is null) = (delete_reason is null)),
    foreign key (firm_id, practice_area_kind, practice_area_id) references reference_item (firm_id, kind, id)
);
create index matter_title_trgm on matter using gin (title_latin gin_trgm_ops);
create trigger matter_touch before update on matter for each row execute function nd_touch_row();

-- Per-firm, per-year sequence for matter numbers (format is a firm setting).
create table matter_number_counter (
    firm_id  uuid not null references firm (id),
    year     integer not null,
    last_seq integer not null default 0,
    primary key (firm_id, year)
);

create table matter_client (
    id          uuid primary key default uuidv7(),
    firm_id     uuid not null references firm (id),
    matter_id   uuid not null,
    entity_id   uuid not null,
    client_role text not null default 'primary' check (client_role in ('primary', 'co_client', 'instructing',
                                                                        'guarantor', 'paying')),
    created_at  timestamptz not null default now(),
    created_by  uuid,
    unique (firm_id, matter_id, entity_id),
    foreign key (firm_id, matter_id) references matter (firm_id, id),
    foreign key (firm_id, entity_id) references entity (firm_id, id)
);

-- Team history is kept: the current team is valid_to is null.
create table matter_member (
    id         uuid primary key default uuidv7(),
    firm_id    uuid not null references firm (id),
    matter_id  uuid not null,
    user_id    uuid not null,
    team_role  text not null check (team_role in ('responsible_partner', 'lead', 'supporting', 'clerk', 'assistant')),
    valid_from date not null default current_date,
    valid_to   date,
    created_at timestamptz not null default now(),
    created_by uuid,
    ended_by   uuid,
    foreign key (firm_id, matter_id) references matter (firm_id, id),
    foreign key (firm_id, user_id) references app_user (firm_id, id)
);
create unique index matter_member_current on matter_member (firm_id, matter_id, user_id) where valid_to is null;
create unique index matter_member_one_partner on matter_member (firm_id, matter_id)
    where valid_to is null and team_role = 'responsible_partner';
create unique index matter_member_one_lead on matter_member (firm_id, matter_id)
    where valid_to is null and team_role = 'lead';
create index matter_member_user on matter_member (firm_id, user_id) where valid_to is null;

-- ---------------------------------------------------------------------------
-- Ethical walls (doc 09 §5). A wall overrides every role, including the
-- managing partner. Removal needs a second approver.
-- ---------------------------------------------------------------------------

create table ethical_wall (
    id                   uuid primary key default uuidv7(),
    firm_id              uuid not null references firm (id),
    reason               text not null,
    starts_on            date not null default current_date,
    review_on            date,
    created_at           timestamptz not null default now(),
    created_by           uuid not null,
    removal_requested_by uuid,
    removal_requested_at timestamptz,
    removal_reason       text,
    removed_at           timestamptz,
    removed_by           uuid,
    unique (firm_id, id),
    check (removed_by is null or removed_by <> removal_requested_by)
);

create table ethical_wall_matter (
    firm_id   uuid not null references firm (id),
    wall_id   uuid not null,
    matter_id uuid not null,
    primary key (firm_id, wall_id, matter_id),
    foreign key (firm_id, wall_id) references ethical_wall (firm_id, id),
    foreign key (firm_id, matter_id) references matter (firm_id, id)
);
create index ethical_wall_matter_matter on ethical_wall_matter (firm_id, matter_id);

create table ethical_wall_user (
    firm_id uuid not null references firm (id),
    wall_id uuid not null,
    user_id uuid not null,
    primary key (firm_id, wall_id, user_id),
    foreign key (firm_id, wall_id) references ethical_wall (firm_id, id),
    foreign key (firm_id, user_id) references app_user (firm_id, id)
);
create index ethical_wall_user_user on ethical_wall_user (firm_id, user_id);

-- ---------------------------------------------------------------------------
-- Proceedings (doc 05 §4). A matter holds any number of proceedings; a case
-- number is a history, never a single overwritten field.
-- ---------------------------------------------------------------------------

create table proceeding (
    id                       uuid primary key default uuidv7(),
    firm_id                  uuid not null references firm (id),
    matter_id                uuid not null,
    proceeding_type_id       uuid not null,
    proceeding_type_kind     text not null default 'proceeding_type' check (proceeding_type_kind = 'proceeding_type'),
    current_court_id         uuid not null,
    title_en                 text,
    title_si                 text,
    title_latin              text not null default '',
    our_side                 text check (our_side in ('plaintiff_side', 'defendant_side', 'petitioner', 'respondent',
                                                       'appellant', 'accused', 'complainant', 'intervenient', 'other')),
    our_role                 text not null default 'attorney_on_record'
                                 check (our_role in ('attorney_on_record', 'counsel_only', 'watching_brief')),
    instituted_on            date,
    status                   text not null default 'active' check (status in ('active', 'dormant', 'concluded',
                                                                                  'transferred_out')),
    outcome_type_id          uuid,
    outcome_type_kind        text not null default 'outcome_type' check (outcome_type_kind = 'outcome_type'),
    confidentiality_override text check (confidentiality_override in ('restricted', 'highly_restricted')),
    created_at               timestamptz not null default now(),
    created_by               uuid,
    created_via              text not null default 'user' check (created_via in ('user', 'import', 'system')),
    import_batch_id          uuid,
    updated_at               timestamptz not null default now(),
    updated_by               uuid,
    row_version              integer not null default 1,
    deleted_at               timestamptz,
    deleted_by               uuid,
    delete_reason            text,
    unique (firm_id, id),
    check ((deleted_at is null) = (delete_reason is null)),
    check (status <> 'concluded' or outcome_type_id is not null),
    foreign key (firm_id, matter_id) references matter (firm_id, id),
    foreign key (firm_id, proceeding_type_kind, proceeding_type_id) references reference_item (firm_id, kind, id),
    foreign key (firm_id, current_court_id) references court (firm_id, id),
    foreign key (firm_id, outcome_type_kind, outcome_type_id) references reference_item (firm_id, kind, id)
);
create index proceeding_matter on proceeding (firm_id, matter_id) where deleted_at is null;
create index proceeding_title_trgm on proceeding using gin (title_latin gin_trgm_ops);
create trigger proceeding_touch before update on proceeding for each row execute function nd_touch_row();

create table case_number_record (
    id            uuid primary key default uuidv7(),
    firm_id       uuid not null references firm (id),
    proceeding_id uuid not null,
    court_id      uuid not null,
    raw           text not null check (length(raw) between 1 and 80),
    normalised    text not null,
    valid_from    date not null default current_date,
    valid_to      date,
    reason        text not null check (reason in ('assigned', 'corrected', 'renumbered', 'transferred', 'migrated')),
    source        text,
    created_at    timestamptz not null default now(),
    created_by    uuid,
    ended_by      uuid,
    unique (firm_id, id),
    foreign key (firm_id, proceeding_id) references proceeding (firm_id, id),
    foreign key (firm_id, court_id) references court (firm_id, id)
);
-- I-PR-2: at most one current number per proceeding.
create unique index case_number_current on case_number_record (firm_id, proceeding_id) where valid_to is null;
create index case_number_lookup on case_number_record (firm_id, normalised);
create index case_number_trgm on case_number_record using gin (normalised gin_trgm_ops);

create table proceeding_relationship (
    id                     uuid primary key default uuidv7(),
    firm_id                uuid not null references firm (id),
    from_proceeding_id     uuid not null,
    to_proceeding_id       uuid not null,
    relationship_type_id   uuid not null,
    relationship_type_kind text not null default 'relationship_type' check (relationship_type_kind = 'relationship_type'),
    noted_on               date,
    note                   text,
    created_at             timestamptz not null default now(),
    created_by             uuid,
    unique (firm_id, from_proceeding_id, to_proceeding_id, relationship_type_id),
    check (from_proceeding_id <> to_proceeding_id),
    foreign key (firm_id, from_proceeding_id) references proceeding (firm_id, id),
    foreign key (firm_id, to_proceeding_id) references proceeding (firm_id, id),
    foreign key (firm_id, relationship_type_kind, relationship_type_id) references reference_item (firm_id, kind, id)
);

create table party_role (
    id                   uuid primary key default uuidv7(),
    firm_id              uuid not null references firm (id),
    proceeding_id        uuid not null,
    entity_id            uuid not null,
    party_role_type_id   uuid not null,
    party_role_type_kind text not null default 'party_role_type' check (party_role_type_kind = 'party_role_type'),
    ordinal              integer check (ordinal between 1 and 999),
    side                 text not null check (side in ('our_client', 'aligned', 'opposing', 'neutral')),
    status               text not null default 'active' check (status in ('active', 'substituted', 'removed', 'deceased')),
    from_on              date,
    to_on                date,
    created_at           timestamptz not null default now(),
    created_by           uuid,
    updated_at           timestamptz not null default now(),
    updated_by           uuid,
    row_version          integer not null default 1,
    unique (firm_id, id),
    foreign key (firm_id, proceeding_id) references proceeding (firm_id, id),
    foreign key (firm_id, entity_id) references entity (firm_id, id),
    foreign key (firm_id, party_role_type_kind, party_role_type_id) references reference_item (firm_id, kind, id)
);
create unique index party_role_ordinal on party_role (firm_id, proceeding_id, party_role_type_id, coalesce(ordinal, 0))
    where status = 'active';
create index party_role_entity on party_role (firm_id, entity_id);
create trigger party_role_touch before update on party_role for each row execute function nd_touch_row();

-- ---------------------------------------------------------------------------
-- RLS and grants
-- ---------------------------------------------------------------------------

do $$
declare
    t text;
begin
    foreach t in array array['reference_item', 'court', 'courtroom', 'court_calendar_day', 'entity', 'matter',
                             'matter_number_counter', 'matter_client', 'matter_member', 'ethical_wall',
                             'ethical_wall_matter', 'ethical_wall_user', 'proceeding', 'case_number_record',
                             'proceeding_relationship', 'party_role']
    loop
        execute format('alter table %I enable row level security', t);
        execute format('create policy firm_isolation on %I using (firm_id = nd_current_firm()) '
                       'with check (firm_id = nd_current_firm())', t);
        execute format('grant select, insert, update on %I to nithidesk_app', t);
    end loop;
end
$$;
grant delete on ethical_wall_user, ethical_wall_matter to nithidesk_app;

-- ---------------------------------------------------------------------------
-- The matter-visibility rule (doc 09 §1, ADR 0006). Every list, queue, view,
-- count and search joins this function, so the rule exists in one place and
-- is applied inside the query, never by filtering results afterwards.
--
--   access = role scope + team membership + confidentiality − ethical wall
--
-- Returns one row per visible matter with the access level:
--   full  — the caller may read the matter's content.
--   diary — hearing metadata only (clerk diary scope, doc 09 §2.1); notes,
--           instructions and the title of a non-standard matter are withheld.
-- The caller's scope is set per transaction by internal/authz as
-- app.matter_scope. A wall applies to every scope, including membership.
-- ---------------------------------------------------------------------------

create function nd_visible_matters() returns table (matter_id uuid, access text, title_masked boolean)
    language sql stable
    as $$
    with me as (
        select nd_current_user() as uid,
               coalesce(nullif(current_setting('app.matter_scope', true), ''), 'none') as scope
    ),
    walled as (
        select ewm.matter_id
        from ethical_wall_user ewu
        join ethical_wall ew on ew.firm_id = ewu.firm_id and ew.id = ewu.wall_id
        join ethical_wall_matter ewm on ewm.firm_id = ew.firm_id and ewm.wall_id = ew.id
        where ewu.user_id = (select uid from me)
          and ew.removed_at is null
          and ew.starts_on <= current_date
    )
    select m.id,
           case when mm.user_id is not null then 'full'
                when me.scope = 'all_non_walled' and m.confidentiality = 'standard' then 'full'
                else 'diary' end,
           (mm.user_id is null and m.confidentiality <> 'standard')
    from matter m
    cross join me
    left join matter_member mm
           on mm.firm_id = m.firm_id and mm.matter_id = m.id and mm.user_id = me.uid and mm.valid_to is null
    where m.deleted_at is null
      and me.uid is not null
      and me.scope <> 'none'
      and not exists (select 1 from walled w where w.matter_id = m.id)
      and (mm.user_id is not null
           or (me.scope in ('all_non_walled', 'diary') and m.confidentiality <> 'highly_restricted'))
    $$;
grant execute on function nd_visible_matters() to nithidesk_app;
