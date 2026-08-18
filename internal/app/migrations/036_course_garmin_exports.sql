alter table user_settings
    add column garmin_course_owner_token uuid not null default gen_random_uuid();

create table course_garmin_exports (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    course_id uuid not null references courses(id) on delete cascade,
    course_revision integer not null check (course_revision > 0),
    status text not null check (status in ('sending', 'sent', 'attention')),
    ownership_marker text not null,
    provider_course_id text not null default '',
    provider_url text not null default '',
    provider_response jsonb not null default '{}'::jsonb,
    error text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique(user_id, course_id, course_revision)
);

create index course_garmin_exports_user_course_idx
    on course_garmin_exports(user_id, course_id, created_at desc);
