-- 0004 CSV import of the current diary (doc 16, logical-data-model §11) and
-- rendered daily-brief PDFs (ADR 0007).

create table import_batch (
    id               uuid primary key default uuidv7(),
    firm_id          uuid not null references firm (id),
    kind             text not null check (kind in ('diary_csv')),
    file_name        text not null,
    file_sha256      bytea not null,
    file_content     bytea not null,
    status           text not null default 'uploaded'
                         check (status in ('uploaded', 'validated', 'committed', 'rolled_back', 'failed')),
    row_count        integer not null default 0,
    ok_count         integer not null default 0,
    warning_count    integer not null default 0,
    error_count      integer not null default 0,
    created_at       timestamptz not null default now(),
    created_by       uuid not null,
    validated_at     timestamptz,
    committed_at     timestamptz,
    committed_by     uuid,
    rolled_back_at   timestamptz,
    rolled_back_by   uuid,
    updated_at       timestamptz not null default now(),
    row_version      integer not null default 1,
    unique (firm_id, id),
    check (octet_length(file_content) <= 5 * 1024 * 1024),
    foreign key (firm_id, created_by) references app_user (firm_id, id)
);
create trigger import_batch_touch before update on import_batch for each row execute function nd_touch_row();

create table import_row (
    id              uuid primary key default uuidv7(),
    firm_id         uuid not null references firm (id),
    batch_id        uuid not null,
    row_number      integer not null,
    raw             jsonb not null,
    status          text not null check (status in ('ok', 'warning', 'error')),
    messages        jsonb not null default '[]'::jsonb,
    created_matter_id     uuid,
    created_proceeding_id uuid,
    created_hearing_id    uuid,
    unique (firm_id, batch_id, row_number),
    foreign key (firm_id, batch_id) references import_batch (firm_id, id)
);

create table brief_render (
    id            uuid primary key default uuidv7(),
    firm_id       uuid not null references firm (id),
    requested_by  uuid not null,
    brief_date    date not null,
    days          integer not null default 2 check (days between 1 and 7),
    scope         text not null check (scope in ('mine', 'team', 'firm')),
    lang          text not null check (lang in ('en', 'si', 'both')),
    status        text not null default 'queued' check (status in ('queued', 'rendering', 'ready', 'failed')),
    object_key    text,
    byte_size     integer,
    sha256        bytea,
    error_code    text,
    created_at    timestamptz not null default now(),
    completed_at  timestamptz,
    expires_at    timestamptz not null default now() + interval '24 hours',
    unique (firm_id, id),
    foreign key (firm_id, requested_by) references app_user (firm_id, id)
);

do $$
declare
    t text;
begin
    foreach t in array array['import_batch', 'import_row', 'brief_render']
    loop
        execute format('alter table %I enable row level security', t);
        execute format('create policy firm_isolation on %I using (firm_id = nd_current_firm()) '
                       'with check (firm_id = nd_current_firm())', t);
        execute format('grant select, insert, update on %I to nithidesk_app', t);
    end loop;
end
$$;
grant delete on import_row to nithidesk_app;

-- Rolling an import back soft-deletes what it created (doc 16 §4.1); hearings
-- have no soft-delete column, so a rolled-back hearing is cancelled instead.
