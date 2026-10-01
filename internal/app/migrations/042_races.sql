create table race_groups (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    name text not null check (length(name) between 1 and 160),
    revision integer not null default 1,
    unique(user_id, name)
);

create table race_checklist_templates (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    name text not null check (length(name) between 1 and 160),
    items jsonb not null default '[]'::jsonb check (jsonb_typeof(items) = 'array'),
    revision integer not null default 1,
    unique(user_id, name)
);

create table races (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    group_id uuid references race_groups(id) on delete set null,
    activity_id uuid unique references activities(id) on delete set null,
    planned_activity_id uuid references planned_activities(id) on delete set null,
    course_id uuid references courses(id) on delete set null,
    race_date date,
    name text not null check (length(name) between 1 and 160),
    status text not null check (status in ('wishlist','planned','registered','finished','dns','dnf','disqualified','cancelled','postponed')),
    discipline text not null check (discipline in ('road','track','trail','cross_country')),
    kind text not null check (kind in ('race','parkrun','virtual','time_trial')),
    distance_m double precision check (distance_m > 0 and distance_m < 10000000),
    data jsonb not null default '{}'::jsonb check (jsonb_typeof(data) = 'object'),
    course_snapshot jsonb,
    recording_hash text not null default '',
    revision integer not null default 1,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);
create index races_user_date_idx on races(user_id,race_date desc,id);
create index races_user_group_idx on races(user_id,group_id);

create table race_settings (
    user_id uuid primary key references users(id) on delete cascade,
    birth_date date,
    grading_table text not null default '' check (grading_table in ('','M','F')),
    vdot_reference_id uuid references races(id) on delete set null,
    predictions_enabled boolean not null default false,
    weather_enabled boolean not null default false,
    prediction_synced_at timestamptz,
    prediction_attempted_at timestamptz,
    prediction_backfilled boolean not null default false,
    prediction_error text not null default '',
    revision integer not null default 1
);

-- The natural provider identity remembers dismissals even after an activity is reimported.
create table race_discovery (
    user_id uuid not null references users(id) on delete cascade,
    source text not null,
    source_id text not null,
    activity_id uuid references activities(id) on delete set null,
    state text not null default 'pending' check (state in ('pending','dismissed','confirmed')),
    reasons jsonb not null default '[]'::jsonb,
    confidence text not null default 'possible',
    discovered_at timestamptz not null default now(),
    primary key(user_id,source,source_id)
);
create table race_discovery_pending (
    activity_id uuid primary key references activities(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    queued_at timestamptz not null default now()
);

create table race_predictions (
    user_id uuid not null references users(id) on delete cascade,
    prediction_date date not null,
    distance_m double precision not null check (distance_m in (5000,10000,21097.5,42195)),
    time_ms bigint check (time_ms > 0),
    fetched_at timestamptz not null default now(),
    backfilled boolean not null,
    raw jsonb not null default '{}'::jsonb,
    primary key(user_id,prediction_date,distance_m)
);
create table race_prediction_fetches (
    id bigserial primary key,
    user_id uuid not null references users(id) on delete cascade,
    date_from date not null,
    date_to date not null,
    fetched_at timestamptz not null default now(),
    raw jsonb not null
);
-- Snapshots are immutable evidence captured before the event, independent of later refreshes.
create table race_prediction_snapshots (
    race_id uuid primary key references races(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    cutoff timestamptz not null,
    captured_at timestamptz not null,
    data jsonb not null
);
create table race_forecasts (
    race_id uuid primary key references races(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    target_key text not null,
    fetched_at timestamptz,
    attempted_at timestamptz not null default now(),
    error text not null default '',
    data jsonb,
    raw jsonb
);
create table race_reports (
    race_id uuid primary key references races(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    revision integer not null default 1,
    data jsonb not null default '{}'::jsonb,
    updated_at timestamptz not null default now()
);
