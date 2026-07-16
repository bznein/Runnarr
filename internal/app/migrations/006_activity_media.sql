create table if not exists activity_media (
	id uuid primary key default gen_random_uuid(),
	activity_id uuid not null references activities(id) on delete cascade,
	original_filename text not null,
	content_type text not null,
	size_bytes bigint not null,
	sha256 text not null,
	original_path text not null,
	thumbnail_path text not null,
	width integer not null default 0,
	height integer not null default 0,
	capture_time timestamptz,
	latitude double precision,
	longitude double precision,
	created_at timestamptz not null default now(),
	unique(activity_id, sha256)
);

create index if not exists activity_media_activity_idx on activity_media(activity_id, created_at desc);
