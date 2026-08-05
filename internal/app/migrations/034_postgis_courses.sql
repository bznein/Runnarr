create extension if not exists postgis;

create table courses (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    name text not null check (char_length(btrim(name)) between 1 and 160),
    sport_type text not null check (sport_type in ('Run', 'Walk', 'Hike', 'Cycling')),
    notes text not null default '' check (char_length(notes) <= 5000),
    favorite boolean not null default false,
    revision integer not null default 1 check (revision > 0),
    geometry_hash text not null,
    distance_m double precision not null check (distance_m >= 0),
    elevation_gain_m double precision,
    elevation_loss_m double precision,
    elevation_coverage double precision not null default 0 check (elevation_coverage between 0 and 1),
    point_count integer not null check (point_count between 2 and 100000),
    leg_count integer not null check (leg_count between 1 and 500),
    direct_leg_count integer not null default 0 check (direct_leg_count >= 0),
    diagnostics jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index courses_user_updated_idx on courses(user_id, updated_at desc, id);
create index courses_user_favorite_idx on courses(user_id, favorite, updated_at desc);
create index courses_user_geometry_hash_idx on courses(user_id, geometry_hash);

create table course_waypoints (
    id uuid primary key default gen_random_uuid(),
    course_id uuid not null references courses(id) on delete cascade,
    waypoint_index integer not null check (waypoint_index >= 0),
    location geometry(Point, 4326) not null,
    unique(course_id, waypoint_index)
);

create index course_waypoints_course_idx on course_waypoints(course_id, waypoint_index);
create index course_waypoints_location_gist_idx on course_waypoints using gist(location);

create table course_legs (
    id uuid primary key default gen_random_uuid(),
    course_id uuid not null references courses(id) on delete cascade,
    leg_index integer not null check (leg_index >= 0),
    mode text not null check (mode in ('preserved', 'routed', 'direct')),
    geometry geometry(LineString, 4326) not null,
    elevations double precision[] not null,
    unique(course_id, leg_index),
    check (st_npoints(geometry) >= 2),
    check (cardinality(elevations) = st_npoints(geometry))
);

create index course_legs_course_idx on course_legs(course_id, leg_index);
create index course_legs_geometry_gist_idx on course_legs using gist(geometry);

create table course_imports (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    filename text not null,
    file_sha256 text not null,
    diagnostics jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now()
);

create index course_imports_user_created_idx on course_imports(user_id, created_at desc);

create table course_import_items (
    import_id uuid not null references course_imports(id) on delete cascade,
    course_id uuid not null references courses(id) on delete cascade,
    candidate_key text not null,
    primary key(import_id, course_id),
    unique(import_id, candidate_key)
);

create index course_import_items_course_idx on course_import_items(course_id);
