create table if not exists sync_excluded_activities (
	id uuid primary key default gen_random_uuid(),
	source text not null,
	source_id text not null,
	name text not null default '',
	sport_type text not null default '',
	start_time timestamptz,
	reason text not null default 'deleted_from_runnarr',
	created_at timestamptz not null default now(),
	unique(source, source_id)
);

create index if not exists sync_excluded_activities_source_idx on sync_excluded_activities(source, source_id);
