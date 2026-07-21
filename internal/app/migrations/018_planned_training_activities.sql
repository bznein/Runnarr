alter table if exists app_settings
    add column if not exists plan_year integer not null default 0;

create table if not exists planned_activities (
    id uuid primary key default gen_random_uuid(),
    source text not null,
    source_id text not null,
    workbook_id text not null,
    sheet_id text not null,
    sheet_title text not null default '',
    plan_cell text not null,
    planned_date date not null,
    name text not null,
    sport_type text not null default 'Run',
    notes text not null default '',
    status text not null default 'pending',
    source_url text not null default '',
    raw jsonb not null default '{}'::jsonb,
    first_seen_at timestamptz not null default now(),
    last_seen_at timestamptz not null default now(),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique(source, source_id)
);

create index if not exists planned_activities_date_idx
    on planned_activities(planned_date, status);

create table if not exists google_sheets_tokens (
    id text primary key,
    access_token_ciphertext bytea not null default decode('', 'hex'),
    refresh_token_ciphertext bytea not null,
    token_expires_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists google_oauth_states (
    state text primary key,
    session_id text not null,
    expires_at timestamptz not null,
    created_at timestamptz not null default now()
);

create index if not exists google_oauth_states_expiry_idx
    on google_oauth_states(expires_at);
