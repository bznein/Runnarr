create table if not exists activity_workouts (
    activity_id uuid primary key references activities(id) on delete cascade,
    provider text not null,
    provider_workout_id text not null default '',
    name text not null default '',
    sport_type text not null default '',
    steps jsonb not null default '[]'::jsonb,
    raw jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists activity_intervals (
    id bigserial primary key,
    activity_id uuid not null references activities(id) on delete cascade,
    interval_index integer not null,
    category text not null default '',
    provider_type text not null default '',
    workout_step_index integer,
    workout_repeat_index integer,
    start_time timestamptz,
    end_time timestamptz,
    elapsed_time_s integer not null default 0,
    moving_time_s integer not null default 0,
    distance_m double precision not null default 0,
    avg_pace_s_per_km double precision,
    avg_grade_adjusted_pace_s_per_km double precision,
    avg_heart_rate double precision,
    max_heart_rate double precision,
    avg_power double precision,
    max_power double precision,
    normalized_power double precision,
    avg_run_cadence double precision,
    avg_ground_contact_time_ms double precision,
    avg_respiration_rate double precision,
    avg_temperature_c double precision,
    elevation_gain_m double precision,
    elevation_loss_m double precision,
    calories_kcal integer,
    lap_indexes integer[] not null default '{}',
    raw jsonb not null default '{}'::jsonb,
    unique(activity_id, interval_index)
);

create index if not exists activity_intervals_activity_idx on activity_intervals(activity_id, interval_index);

alter table activity_laps
    add column if not exists avg_heart_rate double precision,
    add column if not exists max_heart_rate double precision,
    add column if not exists avg_power double precision,
    add column if not exists max_power double precision,
    add column if not exists normalized_power double precision,
    add column if not exists avg_run_cadence double precision,
    add column if not exists avg_ground_contact_time_ms double precision,
    add column if not exists avg_respiration_rate double precision,
    add column if not exists avg_temperature_c double precision,
    add column if not exists intensity_type text,
    add column if not exists workout_step_index integer,
    add column if not exists workout_repeat_index integer,
    add column if not exists raw jsonb not null default '{}'::jsonb;
