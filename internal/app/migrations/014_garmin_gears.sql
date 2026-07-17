create table if not exists gears (
	id uuid primary key default gen_random_uuid(),
	provider text not null,
	provider_gear_id text not null,
	name text not null default '',
	gear_type text not null default '',
	brand text not null default '',
	model text not null default '',
	retired boolean not null default false,
	total_distance_m double precision,
	max_distance_m double precision,
	first_used_at timestamptz,
	last_used_at timestamptz,
	default_activity_types text[] not null default '{}',
	raw jsonb not null default '{}'::jsonb,
	stats_raw jsonb not null default '{}'::jsonb,
	created_at timestamptz not null default now(),
	updated_at timestamptz not null default now(),
	unique(provider, provider_gear_id)
);

create index if not exists gears_provider_retired_idx on gears(provider, retired, name);

create table if not exists activity_gears (
	activity_id uuid not null references activities(id) on delete cascade,
	gear_id uuid not null references gears(id) on delete cascade,
	raw jsonb not null default '{}'::jsonb,
	created_at timestamptz not null default now(),
	updated_at timestamptz not null default now(),
	primary key(activity_id, gear_id)
);

create index if not exists activity_gears_gear_idx on activity_gears(gear_id, activity_id);
