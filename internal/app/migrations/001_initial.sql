create extension if not exists pgcrypto;

create table if not exists auth_sessions (
	id uuid primary key default gen_random_uuid(),
	csrf_token text not null,
	created_at timestamptz not null default now(),
	last_seen_at timestamptz not null default now(),
	expires_at timestamptz not null
);

create table if not exists import_files (
	id uuid primary key default gen_random_uuid(),
	filename text not null,
	content_type text not null default '',
	sha256 text not null unique,
	size_bytes bigint not null,
	parser text not null,
	status text not null,
	error text not null default '',
	created_at timestamptz not null default now()
);

create table if not exists provider_connections (
	id uuid primary key default gen_random_uuid(),
	provider text not null unique,
	provider_account_id text not null default '',
	display_name text not null default '',
	access_token_ciphertext bytea,
	refresh_token_ciphertext bytea,
	token_expires_at timestamptz,
	scopes text[] not null default '{}',
	metadata jsonb not null default '{}'::jsonb,
	connected_at timestamptz not null default now(),
	updated_at timestamptz not null default now()
);

create table if not exists activities (
	id uuid primary key default gen_random_uuid(),
	source text not null,
	source_id text not null,
	source_file_id uuid references import_files(id) on delete set null,
	name text not null,
	sport_type text not null,
	start_time timestamptz not null,
	distance_m double precision not null default 0,
	moving_time_s integer not null default 0,
	elapsed_time_s integer not null default 0,
	elevation_gain_m double precision not null default 0,
	avg_heart_rate double precision,
	max_heart_rate double precision,
	avg_pace_s_per_km double precision,
	summary_polyline text not null default '',
	raw jsonb not null default '{}'::jsonb,
	created_at timestamptz not null default now(),
	updated_at timestamptz not null default now(),
	unique(source, source_id)
);

create index if not exists activities_start_time_idx on activities(start_time desc);
create index if not exists activities_sport_type_idx on activities(sport_type);

create table if not exists activity_samples (
	id bigserial primary key,
	activity_id uuid not null references activities(id) on delete cascade,
	sample_index integer not null,
	timestamp timestamptz,
	elapsed_s integer,
	distance_m double precision,
	latitude double precision,
	longitude double precision,
	elevation_m double precision,
	heart_rate integer,
	cadence integer,
	power integer,
	speed_mps double precision,
	unique(activity_id, sample_index)
);

create index if not exists activity_samples_activity_idx on activity_samples(activity_id, sample_index);

create table if not exists activity_laps (
	id bigserial primary key,
	activity_id uuid not null references activities(id) on delete cascade,
	lap_index integer not null,
	start_time timestamptz,
	elapsed_time_s integer not null default 0,
	distance_m double precision not null default 0,
	unique(activity_id, lap_index)
);

create table if not exists sync_jobs (
	id uuid primary key default gen_random_uuid(),
	provider text not null,
	kind text not null,
	status text not null,
	payload jsonb not null default '{}'::jsonb,
	error text not null default '',
	created_at timestamptz not null default now(),
	started_at timestamptz,
	finished_at timestamptz
);

create index if not exists sync_jobs_created_idx on sync_jobs(created_at desc);

