-- 0003 Hearings: the court diary core (doc 06, logical-data-model §8).
--
-- Two independent dimensions per hearing (doc 06 §4.2):
--   listing_status      what is happening to the listing
--   verification_state  how sure we are of the date (derived, cached)
-- A date is never edited in place. It is a sequence of append-only date
-- assertions, each with its source; verifications confirm one assertion.

create table hearing (
    id                  uuid primary key default uuidv7(),
    firm_id             uuid not null references firm (id),
    proceeding_id       uuid not null,
    -- Denormalised from the proceeding so every diary query can join the
    -- visibility rule directly. Kept consistent by hearing_matter_matches.
    matter_id           uuid not null,
    court_id            uuid not null,
    courtroom_id        uuid,
    -- The current effective date: the value of the current assertion,
    -- maintained in the same transaction by internal/hearings.
    hearing_date        date not null,
    session             text not null default 'unspecified' check (session in ('morning', 'afternoon', 'unspecified')),
    time_of_day         time,
    purpose_id          uuid,
    purpose_kind        text not null default 'hearing_purpose' check (purpose_kind = 'hearing_purpose'),
    purpose_note        text,
    listing_status      text not null default 'scheduled'
                            check (listing_status in ('scheduled', 'held', 'not_reached', 'court_not_sitting',
                                                      'vacated', 'cancelled', 'outcome_recorded')),
    verification_state  text not null default 'unverified' check (verification_state in ('unverified', 'verified', 'conflict')),
    preparation_status  text not null default 'not_started'
                            check (preparation_status in ('not_started', 'in_progress', 'ready', 'blocked')),
    preparation_note    text,
    previous_hearing_id uuid,
    replaces_hearing_id uuid,
    instructions        text,
    notes               text,
    assigned_clerk_id   uuid,
    status_reason       text,
    import_batch_id     uuid,
    created_at          timestamptz not null default now(),
    created_by          uuid,
    created_via         text not null default 'user' check (created_via in ('user', 'import', 'outcome', 'system')),
    updated_at          timestamptz not null default now(),
    updated_by          uuid,
    row_version         integer not null default 1,
    unique (firm_id, id),
    check (listing_status not in ('cancelled', 'vacated') or status_reason is not null),
    check (previous_hearing_id is distinct from id and replaces_hearing_id is distinct from id),
    foreign key (firm_id, proceeding_id) references proceeding (firm_id, id),
    foreign key (firm_id, matter_id) references matter (firm_id, id),
    foreign key (firm_id, court_id) references court (firm_id, id),
    foreign key (firm_id, courtroom_id) references courtroom (firm_id, id),
    foreign key (firm_id, purpose_kind, purpose_id) references reference_item (firm_id, kind, id),
    foreign key (firm_id, previous_hearing_id) references hearing (firm_id, id),
    foreign key (firm_id, replaces_hearing_id) references hearing (firm_id, id),
    foreign key (firm_id, assigned_clerk_id) references app_user (firm_id, id)
);
create index hearing_date on hearing (firm_id, hearing_date, court_id);
create index hearing_matter on hearing (firm_id, matter_id, hearing_date);
create index hearing_proceeding on hearing (firm_id, proceeding_id, hearing_date);
create index hearing_unverified on hearing (firm_id, hearing_date)
    where verification_state <> 'verified' and listing_status = 'scheduled';
create trigger hearing_touch before update on hearing for each row execute function nd_touch_row();

create function nd_hearing_matter_matches() returns trigger
    language plpgsql
    as $$
begin
    if not exists (select 1 from proceeding p
                   where p.firm_id = new.firm_id and p.id = new.proceeding_id and p.matter_id = new.matter_id) then
        raise exception 'hearing matter does not match its proceeding' using errcode = '23514';
    end if;
    return new;
end
$$;
create trigger hearing_matter_matches before insert or update of proceeding_id, matter_id on hearing
    for each row execute function nd_hearing_matter_matches();

create table hearing_appearance (
    id         uuid primary key default uuidv7(),
    firm_id    uuid not null references firm (id),
    hearing_id uuid not null,
    user_id    uuid not null,
    capacity   text not null default 'appearing' check (capacity in ('appearing', 'instructing', 'observing')),
    created_at timestamptz not null default now(),
    created_by uuid,
    unique (firm_id, hearing_id, user_id),
    foreign key (firm_id, hearing_id) references hearing (firm_id, id),
    foreign key (firm_id, user_id) references app_user (firm_id, id)
);
create index hearing_appearance_user on hearing_appearance (firm_id, user_id);

-- ---------------------------------------------------------------------------
-- Date provenance (doc 06 §3). Append-only.
-- ---------------------------------------------------------------------------

create table date_assertion (
    id               uuid primary key default uuidv7(),
    firm_id          uuid not null references firm (id),
    hearing_id       uuid not null,
    asserted_date    date not null,
    asserted_session text not null default 'unspecified' check (asserted_session in ('morning', 'afternoon', 'unspecified')),
    asserted_time    time,
    source_type_id   uuid not null,
    source_type_kind text not null default 'date_source_type' check (source_type_kind = 'date_source_type'),
    -- Copied from the source type at write time so a later configuration
    -- change never rewrites history.
    authority        text not null check (authority in ('authoritative', 'primary', 'secondary', 'hearsay')),
    source_detail    text,
    kind             text not null check (kind in ('initial', 'correction', 'from_outcome', 'from_court_list',
                                                   'from_import', 'from_connector', 'replacement')),
    supersedes_id    uuid,
    asserted_by      uuid,
    asserted_at      timestamptz not null default now(),
    unique (firm_id, id),
    foreign key (firm_id, hearing_id) references hearing (firm_id, id),
    foreign key (firm_id, source_type_kind, source_type_id) references reference_item (firm_id, kind, id),
    foreign key (firm_id, supersedes_id) references date_assertion (firm_id, id),
    foreign key (firm_id, asserted_by) references app_user (firm_id, id)
);
create index date_assertion_hearing on date_assertion (firm_id, hearing_id, asserted_at);
create trigger date_assertion_append_only before update or delete on date_assertion
    for each row execute function nd_reject_change();

create table verification (
    id                uuid primary key default uuidv7(),
    firm_id           uuid not null references firm (id),
    hearing_id        uuid not null,
    date_assertion_id uuid not null,
    source_type_id    uuid not null,
    source_type_kind  text not null default 'date_source_type' check (source_type_kind = 'date_source_type'),
    evidence_reference text not null check (length(trim(evidence_reference)) > 0),
    verified_by       uuid not null,
    verified_at       timestamptz not null default now(),
    revoked_at        timestamptz,
    revoked_by        uuid,
    revoke_reason     text,
    unique (firm_id, id),
    check ((revoked_at is null) = (revoked_by is null) and (revoked_at is null) = (revoke_reason is null)),
    foreign key (firm_id, hearing_id) references hearing (firm_id, id),
    foreign key (firm_id, date_assertion_id) references date_assertion (firm_id, id),
    foreign key (firm_id, source_type_kind, source_type_id) references reference_item (firm_id, kind, id),
    foreign key (firm_id, verified_by) references app_user (firm_id, id),
    foreign key (firm_id, revoked_by) references app_user (firm_id, id)
);
create index verification_hearing on verification (firm_id, hearing_id) where revoked_at is null;

-- A verification may only be revoked, once. Nothing else about it changes.
create function nd_verification_revoke_only() returns trigger
    language plpgsql
    as $$
begin
    if tg_op = 'DELETE' then
        raise exception 'verification is append-only' using errcode = 'P0001';
    end if;
    if old.revoked_at is not null
       or new.id <> old.id or new.firm_id <> old.firm_id or new.hearing_id <> old.hearing_id
       or new.date_assertion_id <> old.date_assertion_id or new.source_type_id <> old.source_type_id
       or new.evidence_reference <> old.evidence_reference or new.verified_by <> old.verified_by
       or new.verified_at <> old.verified_at then
        raise exception 'a verification can only be revoked, once' using errcode = 'P0001';
    end if;
    return new;
end
$$;
create trigger verification_revoke_only before update or delete on verification
    for each row execute function nd_verification_revoke_only();

-- ---------------------------------------------------------------------------
-- Outcomes (doc 06 §6.4). Append-only; a correction is a new row.
-- ---------------------------------------------------------------------------

create table outcome (
    id                 uuid primary key default uuidv7(),
    firm_id            uuid not null references firm (id),
    hearing_id         uuid not null,
    outcome_type_id    uuid not null,
    outcome_type_kind  text not null default 'outcome_type' check (outcome_type_kind = 'outcome_type'),
    summary            text,
    order_text         text,
    next_hearing_id    uuid,
    no_next_reason     text check (no_next_reason in ('concluded', 'laid_by', 'date_from_registry', 'other')),
    no_next_note       text,
    inform_client      boolean not null default true,
    certified_copy     boolean not null default false,
    appeared_user_id   uuid,
    corrects_id        uuid,
    recorded_by        uuid not null,
    recorded_at        timestamptz not null default now(),
    client_recorded_at timestamptz,
    unique (firm_id, id),
    -- I-OU-1: a next hearing or a stated reason, never neither.
    check ((next_hearing_id is null) <> (no_next_reason is null)),
    foreign key (firm_id, hearing_id) references hearing (firm_id, id),
    foreign key (firm_id, outcome_type_kind, outcome_type_id) references reference_item (firm_id, kind, id),
    foreign key (firm_id, next_hearing_id) references hearing (firm_id, id),
    foreign key (firm_id, corrects_id) references outcome (firm_id, id),
    foreign key (firm_id, recorded_by) references app_user (firm_id, id),
    foreign key (firm_id, appeared_user_id) references app_user (firm_id, id)
);
create index outcome_hearing on outcome (firm_id, hearing_id, recorded_at);
create unique index outcome_one_original on outcome (firm_id, hearing_id) where corrects_id is null;
create trigger outcome_append_only before update or delete on outcome
    for each row execute function nd_reject_change();

-- Quick court-day notes; append-only so an offline replay cannot overwrite.
create table hearing_note (
    id                 uuid primary key default uuidv7(),
    firm_id            uuid not null references firm (id),
    hearing_id         uuid not null,
    body               text not null check (length(trim(body)) > 0),
    created_by         uuid not null,
    created_at         timestamptz not null default now(),
    client_recorded_at timestamptz,
    unique (firm_id, id),
    foreign key (firm_id, hearing_id) references hearing (firm_id, id),
    foreign key (firm_id, created_by) references app_user (firm_id, id)
);
create index hearing_note_hearing on hearing_note (firm_id, hearing_id, created_at);
create trigger hearing_note_append_only before update or delete on hearing_note
    for each row execute function nd_reject_change();

-- A record of each manual court-list tick-off (doc 06 §4.4), for audit.
create table court_list_check (
    id           uuid primary key default uuidv7(),
    firm_id      uuid not null references firm (id),
    court_id     uuid not null,
    list_date    date not null,
    numbers_seen text[] not null,
    result       jsonb not null,
    source_note  text,
    checked_by   uuid not null,
    checked_at   timestamptz not null default now(),
    foreign key (firm_id, court_id) references court (firm_id, id),
    foreign key (firm_id, checked_by) references app_user (firm_id, id)
);
create trigger court_list_check_append_only before update or delete on court_list_check
    for each row execute function nd_reject_change();

do $$
declare
    t text;
begin
    foreach t in array array['hearing', 'hearing_appearance', 'date_assertion', 'verification', 'outcome',
                             'hearing_note', 'court_list_check']
    loop
        execute format('alter table %I enable row level security', t);
        execute format('create policy firm_isolation on %I using (firm_id = nd_current_firm()) '
                       'with check (firm_id = nd_current_firm())', t);
        execute format('grant select, insert on %I to nithidesk_app', t);
    end loop;
end
$$;
grant update on hearing to nithidesk_app;
grant delete on hearing_appearance to nithidesk_app;
grant update (revoked_at, revoked_by, revoke_reason) on verification to nithidesk_app;
