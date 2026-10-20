-- 0001 Foundation: tenancy, row-level security, identity, sessions, audit,
-- idempotency and the background job queue.
--
-- Conventions (docs/design/logical-data-model.md §1):
--   * Every business table carries firm_id and is protected by row-level
--     security on it (ADR 0002). The application connects as a member of
--     nithidesk_app, which never owns tables, so RLS always applies to it.
--   * firm_id is part of a unique (firm_id, id) key so child rows reference
--     their parent with a composite foreign key and cannot point across firms.
--   * Court dates are `date`; instants are `timestamptz` (UTC).
--   * Append-only tables grant the application INSERT and SELECT only.

create extension if not exists pg_trgm;

-- The runtime role. Login users (for example nithidesk_api) are granted
-- membership by the deployment; they are never table owners.
do $$
begin
    if not exists (select 1 from pg_roles where rolname = 'nithidesk_app') then
        create role nithidesk_app nologin;
    end if;
end
$$;

grant usage on schema public to nithidesk_app;

-- Tenant context. The application sets these with SET LOCAL at the start of
-- every transaction (internal/platform/db). An unset firm matches no rows.
create function nd_current_firm() returns uuid
    language sql stable parallel safe
    as $$ select nullif(current_setting('app.firm_id', true), '')::uuid $$;

create function nd_current_user() returns uuid
    language sql stable parallel safe
    as $$ select nullif(current_setting('app.user_id', true), '')::uuid $$;

-- Keeps updated_at and row_version honest on every mutable table. row_version
-- is the API ETag (optimistic concurrency, api-outline §2.5).
create function nd_touch_row() returns trigger
    language plpgsql
    as $$
begin
    new.updated_at := now();
    new.row_version := old.row_version + 1;
    return new;
end
$$;

-- Rejects every UPDATE and DELETE on an append-only table, including from the
-- table owner, so the rule does not rest on grants alone.
create function nd_reject_change() returns trigger
    language plpgsql
    as $$
begin
    raise exception 'table % is append-only', tg_table_name using errcode = 'P0001';
end
$$;

-- ---------------------------------------------------------------------------
-- Firm (tenant root)
-- ---------------------------------------------------------------------------

create table firm (
    id             uuid primary key default uuidv7(),
    code           text not null unique check (code ~ '^[a-z0-9][a-z0-9-]{1,40}$'),
    name_en        text,
    name_si        text,
    name_ta        text,
    default_locale text not null default 'en' check (default_locale in ('en', 'si', 'ta')),
    time_zone      text not null default 'Asia/Colombo',
    settings       jsonb not null default '{}'::jsonb,
    created_at     timestamptz not null default now(),
    check (coalesce(name_en, name_si, name_ta) is not null)
);

alter table firm enable row level security;
create policy firm_isolation on firm using (id = nd_current_firm());
grant select on firm to nithidesk_app;
grant update (settings, name_en, name_si, name_ta, default_locale) on firm to nithidesk_app;

-- Resolving a firm code is the one lookup that happens before a tenant is
-- known (login). It returns the id only.
create function nd_firm_id_by_code(p_code text) returns uuid
    language sql stable security definer
    set search_path = public, pg_temp
    as $$ select id from firm where code = lower(p_code) $$;
revoke all on function nd_firm_id_by_code(text) from public;
grant execute on function nd_firm_id_by_code(text) to nithidesk_app;

-- ---------------------------------------------------------------------------
-- Users, roles, MFA
-- ---------------------------------------------------------------------------

create table app_user (
    id                  uuid primary key default uuidv7(),
    firm_id             uuid not null references firm (id),
    email               text not null,
    display_name_en     text,
    display_name_si     text,
    preferred_locale    text not null default 'en' check (preferred_locale in ('en', 'si', 'ta')),
    label_mode          text not null default 'single' check (label_mode in ('single', 'dual')),
    status              text not null default 'invited' check (status in ('invited', 'active', 'disabled')),
    password_hash       text,
    password_changed_at timestamptz,
    failed_login_count  integer not null default 0,
    locked_until        timestamptz,
    last_login_at       timestamptz,
    created_at          timestamptz not null default now(),
    created_by          uuid,
    updated_at          timestamptz not null default now(),
    updated_by          uuid,
    row_version         integer not null default 1,
    unique (firm_id, id),
    check (coalesce(display_name_en, display_name_si) is not null),
    check (email = lower(email) and position('@' in email) > 1)
);
create unique index app_user_email on app_user (firm_id, email);
create trigger app_user_touch before update on app_user for each row execute function nd_touch_row();

create table role_template (
    id           uuid primary key default uuidv7(),
    firm_id      uuid not null references firm (id),
    code         text not null,
    name_en      text not null,
    name_si      text,
    capabilities text[] not null default '{}',
    -- member: only matters whose team the user is on.
    -- diary:  hearing metadata of every non-highly-restricted matter (clerks).
    -- all_non_walled: every standard matter not behind a wall (configurable).
    -- none:   no matter content at all (system administrator).
    matter_scope text not null check (matter_scope in ('member', 'diary', 'all_non_walled', 'none')),
    is_system    boolean not null default false,
    created_at   timestamptz not null default now(),
    updated_at   timestamptz not null default now(),
    row_version  integer not null default 1,
    unique (firm_id, id),
    unique (firm_id, code)
);
create trigger role_template_touch before update on role_template for each row execute function nd_touch_row();

create table user_role (
    id               uuid primary key default uuidv7(),
    firm_id          uuid not null references firm (id),
    user_id          uuid not null,
    role_template_id uuid not null,
    valid_from       timestamptz not null default now(),
    valid_to         timestamptz,
    created_at       timestamptz not null default now(),
    created_by       uuid,
    foreign key (firm_id, user_id) references app_user (firm_id, id),
    foreign key (firm_id, role_template_id) references role_template (firm_id, id)
);
create index user_role_user on user_role (firm_id, user_id) where valid_to is null;

create table mfa_factor (
    id               uuid primary key default uuidv7(),
    firm_id          uuid not null references firm (id),
    user_id          uuid not null,
    kind             text not null check (kind in ('totp')),
    secret_encrypted bytea not null,
    confirmed_at     timestamptz,
    -- The last accepted TOTP time step, so a code cannot be replayed.
    last_used_step   bigint not null default 0,
    created_at       timestamptz not null default now(),
    revoked_at       timestamptz,
    foreign key (firm_id, user_id) references app_user (firm_id, id)
);
create unique index mfa_factor_one_active on mfa_factor (firm_id, user_id, kind) where revoked_at is null;

create table mfa_recovery_code (
    id         uuid primary key default uuidv7(),
    firm_id    uuid not null references firm (id),
    user_id    uuid not null,
    code_hash  bytea not null,
    used_at    timestamptz,
    created_at timestamptz not null default now(),
    foreign key (firm_id, user_id) references app_user (firm_id, id)
);
create index mfa_recovery_code_user on mfa_recovery_code (firm_id, user_id) where used_at is null;

-- ---------------------------------------------------------------------------
-- Sessions (ADR 0004): opaque token in a cookie, only its hash stored here.
-- ---------------------------------------------------------------------------

create table user_session (
    id                  uuid primary key default uuidv7(),
    firm_id             uuid not null references firm (id),
    user_id             uuid not null,
    token_hash          bytea not null unique,
    csrf_token          text not null,
    client_kind         text not null default 'desktop' check (client_kind in ('desktop', 'mobile')),
    device_label        text,
    ip                  inet,
    user_agent          text,
    created_at          timestamptz not null default now(),
    last_seen_at        timestamptz not null default now(),
    idle_expires_at     timestamptz not null,
    absolute_expires_at timestamptz not null,
    mfa_satisfied_at    timestamptz,
    step_up_at          timestamptz,
    pin_hash            text,
    pin_failures        integer not null default 0,
    locked_at           timestamptz,
    revoked_at          timestamptz,
    revoked_reason      text check (revoked_reason in ('logout', 'user_revoked', 'admin_revoked', 'user_disabled',
                                                       'pin_failures', 'password_changed', 'mfa_reset')),
    foreign key (firm_id, user_id) references app_user (firm_id, id)
);
create index user_session_user on user_session (firm_id, user_id) where revoked_at is null;

-- A session cookie arrives before the tenant is known. This returns only the
-- firm the hashed token belongs to; everything else is read under RLS.
create function nd_session_firm(p_token_hash bytea) returns uuid
    language sql stable security definer
    set search_path = public, pg_temp
    as $$ select firm_id from user_session where token_hash = p_token_hash and revoked_at is null $$;
revoke all on function nd_session_firm(bytea) from public;
grant execute on function nd_session_firm(bytea) to nithidesk_app;

-- ---------------------------------------------------------------------------
-- Idempotency keys (api-outline §2.5): safe retries for the offline queue.
-- ---------------------------------------------------------------------------

create table idempotency_record (
    firm_id         uuid not null references firm (id),
    user_id         uuid not null,
    idem_key        text not null check (length(idem_key) between 8 and 200),
    method          text not null,
    route           text not null,
    request_hash    bytea not null,
    response_status integer,
    response_body   jsonb,
    created_at      timestamptz not null default now(),
    primary key (firm_id, user_id, idem_key),
    foreign key (firm_id, user_id) references app_user (firm_id, id)
);
create index idempotency_record_age on idempotency_record (created_at);

-- ---------------------------------------------------------------------------
-- Audit (doc 10 §9). Append-only and hash-chained per firm.
-- ---------------------------------------------------------------------------

create table audit_event (
    id            bigint generated always as identity primary key,
    firm_id       uuid not null references firm (id),
    occurred_at   timestamptz not null default now(),
    actor_kind    text not null check (actor_kind in ('user', 'system', 'import', 'connector')),
    actor_user_id uuid,
    session_id    uuid,
    request_id    text,
    ip            inet,
    action        text not null check (action ~ '^[a-z_]+(\.[a-z_]+)+$'),
    target_kind   text,
    target_id     uuid,
    matter_id     uuid,
    result        text not null check (result in ('success', 'denied', 'not_found', 'error')),
    reason        text,
    changes       jsonb,
    prev_hash     bytea not null,
    hash          bytea not null unique
);
create index audit_event_firm_time on audit_event (firm_id, occurred_at desc);
create index audit_event_matter on audit_event (firm_id, matter_id, occurred_at desc) where matter_id is not null;
create index audit_event_actor on audit_event (firm_id, actor_user_id, occurred_at desc);
create trigger audit_event_append_only before update or delete on audit_event
    for each row execute function nd_reject_change();

-- The last hash per firm. Appending locks this row, which serialises the
-- chain; one firm's audit volume makes that acceptable.
create table audit_chain_head (
    firm_id   uuid primary key references firm (id),
    last_id   bigint not null default 0,
    last_hash bytea not null
);

-- ---------------------------------------------------------------------------
-- Background jobs (ADR 0001): a Postgres queue claimed with SKIP LOCKED.
-- The worker serves every firm, so this table is not under RLS; each job
-- carries its firm and the worker sets the tenant context before running it.
-- ---------------------------------------------------------------------------

create table job (
    id              bigint generated always as identity primary key,
    firm_id         uuid references firm (id),
    kind            text not null,
    payload         jsonb not null default '{}'::jsonb,
    run_at          timestamptz not null default now(),
    attempts        integer not null default 0,
    max_attempts    integer not null default 5,
    locked_at       timestamptz,
    locked_by       text,
    last_error_code text,
    completed_at    timestamptz,
    failed_at       timestamptz,
    created_at      timestamptz not null default now()
);
create index job_ready on job (run_at) where completed_at is null and failed_at is null;

-- ---------------------------------------------------------------------------
-- Row-level security and grants for the tables above.
-- ---------------------------------------------------------------------------

do $$
declare
    t text;
begin
    foreach t in array array['app_user', 'role_template', 'user_role', 'mfa_factor', 'mfa_recovery_code',
                             'user_session', 'idempotency_record', 'audit_event', 'audit_chain_head']
    loop
        execute format('alter table %I enable row level security', t);
        execute format('create policy firm_isolation on %I using (firm_id = nd_current_firm()) '
                       'with check (firm_id = nd_current_firm())', t);
    end loop;
end
$$;

grant select, insert, update on app_user, role_template, user_role, mfa_factor, mfa_recovery_code,
    user_session, idempotency_record to nithidesk_app;
grant delete on idempotency_record to nithidesk_app;
grant select, insert on audit_event to nithidesk_app;
grant select, insert, update on audit_chain_head to nithidesk_app;
grant select, insert, update on job to nithidesk_app;
