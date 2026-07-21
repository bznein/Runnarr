create table if not exists users (
    id uuid primary key default gen_random_uuid(),
    username text not null,
    display_name text not null default '',
    role text not null default 'user' check (role in ('admin', 'user')),
    password_hash text not null,
    disabled boolean not null default false,
    last_login_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create unique index if not exists users_username_lower_idx on users(lower(username));

create table if not exists user_settings (
    user_id uuid primary key references users(id) on delete cascade,
    climb_smoothing_radius_m double precision not null check (climb_smoothing_radius_m > 0),
    min_climb_distance_m double precision not null check (min_climb_distance_m > 0),
    min_climb_elevation_gain_m double precision not null check (min_climb_elevation_gain_m > 0),
    min_climb_average_grade_pct double precision not null check (min_climb_average_grade_pct > 0),
    max_climb_merge_dip_distance_m double precision not null check (max_climb_merge_dip_distance_m > 0),
    max_climb_merge_elevation_loss_m double precision not null check (max_climb_merge_elevation_loss_m > 0),
    climb_start_gain_m double precision not null check (climb_start_gain_m > 0),
    climb_detection_preset text not null default 'balanced',
    training_sheet_enabled boolean not null default false,
    training_sheet_sheet_url text not null default '',
    training_sheet_check_every_hours integer not null default 24 check (training_sheet_check_every_hours between 1 and 720),
    training_sheet_last_synced_at timestamptz,
    plan_year integer not null default 0,
    theme_preference text not null default 'system' check (theme_preference in ('system', 'light', 'dark')),
    activity_table_columns text[] not null default '{}',
    gear_sort_by text not null default 'distance_percent',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

alter table auth_sessions add column if not exists user_id uuid;
alter table auth_sessions add column if not exists support_user_id uuid;
alter table import_files add column if not exists user_id uuid;
alter table provider_connections add column if not exists user_id uuid;
alter table activities add column if not exists user_id uuid;
alter table sync_jobs add column if not exists user_id uuid;
alter table sync_excluded_activities add column if not exists user_id uuid;
alter table daily_health_metrics add column if not exists user_id uuid;
alter table gears add column if not exists user_id uuid;
alter table planned_activities add column if not exists user_id uuid;
alter table google_sheets_tokens add column if not exists user_id uuid;
alter table google_oauth_states add column if not exists user_id uuid;

alter table import_files drop constraint if exists import_files_sha256_key;
alter table provider_connections drop constraint if exists provider_connections_provider_key;
alter table activities drop constraint if exists activities_source_source_id_key;
alter table sync_excluded_activities drop constraint if exists sync_excluded_activities_source_source_id_key;
alter table daily_health_metrics drop constraint if exists daily_health_metrics_provider_metric_date_key;
alter table gears drop constraint if exists gears_provider_provider_gear_id_key;
alter table planned_activities drop constraint if exists planned_activities_source_source_id_key;

create index if not exists auth_sessions_user_idx on auth_sessions(user_id);
create index if not exists auth_sessions_support_user_idx on auth_sessions(support_user_id);
create index if not exists import_files_user_idx on import_files(user_id, created_at desc);
create index if not exists provider_connections_user_idx on provider_connections(user_id, provider);
create index if not exists activities_user_idx on activities(user_id, start_time desc);
create index if not exists sync_jobs_user_idx on sync_jobs(user_id, created_at desc);
create index if not exists sync_excluded_activities_user_idx on sync_excluded_activities(user_id, source, source_id);
create index if not exists daily_health_metrics_user_idx on daily_health_metrics(user_id, metric_date desc);
create index if not exists gears_user_idx on gears(user_id, provider, retired, name);
create index if not exists planned_activities_user_idx on planned_activities(user_id, planned_date, status);
create index if not exists google_oauth_states_user_idx on google_oauth_states(user_id, expires_at);
