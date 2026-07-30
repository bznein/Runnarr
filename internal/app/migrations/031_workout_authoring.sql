alter table user_settings
    add column if not exists workout_sync_enabled boolean not null default false,
    add column if not exists workout_default_pace_tolerance_s integer not null default 0,
    add column if not exists workout_timezone text not null default '',
    add column if not exists garmin_workout_owner_token uuid not null default gen_random_uuid();

alter table user_settings drop constraint if exists user_settings_workout_pace_tolerance_check;
alter table user_settings
    add constraint user_settings_workout_pace_tolerance_check
    check (workout_default_pace_tolerance_s between 0 and 60);

create table if not exists workouts (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    source text not null check (source in ('training_sheet', 'manual')),
    planned_activity_id uuid references planned_activities(id) on delete set null,
    copied_from_workout_id uuid references workouts(id) on delete set null,
    name text not null,
    sport_type text not null default 'Run',
    source_text text not null default '',
    source_hash text not null default '',
    definition jsonb not null default '{"version":1,"sportType":"Run","steps":[]}'::jsonb,
    parse_status text not null default 'ready' check (parse_status in ('ready', 'warning', 'error')),
    parse_messages jsonb not null default '[]'::jsonb,
    scheduled_date date,
    pace_tolerance_s integer check (pace_tolerance_s between 0 and 60),
    garmin_excluded boolean not null default false,
    revision integer not null default 1 check (revision > 0),
    generated_at timestamptz not null default now(),
    archived_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create unique index if not exists workouts_training_sheet_plan_idx
    on workouts(user_id, planned_activity_id)
    where source = 'training_sheet' and archived_at is null;
create index if not exists workouts_user_date_idx
    on workouts(user_id, scheduled_date, archived_at);
create index if not exists workouts_user_attention_idx
    on workouts(user_id, parse_status, garmin_excluded, updated_at desc);

create table if not exists garmin_workout_templates (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    definition_hash text not null,
    provider_workout_id text not null default '',
    name text not null,
    ownership_marker text not null,
    payload jsonb not null default '{}'::jsonb,
    remote jsonb not null default '{}'::jsonb,
    status text not null default 'pending' check (status in ('pending', 'uploaded', 'error', 'deleted')),
    error text not null default '',
    uploaded_at timestamptz,
    last_seen_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create unique index if not exists garmin_workout_templates_hash_idx
    on garmin_workout_templates(user_id, definition_hash)
    where deleted_at is null;
create unique index if not exists garmin_workout_templates_provider_idx
    on garmin_workout_templates(user_id, provider_workout_id)
    where provider_workout_id <> '' and deleted_at is null;

create table if not exists garmin_workout_schedules (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    workout_id uuid not null references workouts(id) on delete cascade,
    template_id uuid references garmin_workout_templates(id) on delete set null,
    workout_revision integer not null,
    scheduled_date date not null,
    provider_schedule_id text not null default '',
    desired_state text not null default 'scheduled' check (desired_state in ('scheduled', 'absent')),
    status text not null default 'pending' check (status in ('pending', 'scheduled', 'removing', 'removed', 'error')),
    error text not null default '',
    remote jsonb not null default '{}'::jsonb,
    scheduled_at timestamptz,
    removed_at timestamptz,
    last_attempt_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists garmin_workout_schedules_workout_idx
    on garmin_workout_schedules(user_id, workout_id, created_at desc);
create unique index if not exists garmin_workout_schedules_provider_idx
    on garmin_workout_schedules(user_id, provider_schedule_id)
    where provider_schedule_id <> '' and status <> 'removed';
create unique index if not exists garmin_workout_schedules_active_idx
    on garmin_workout_schedules(user_id, workout_id)
    where status in ('pending', 'scheduled', 'removing', 'error');
