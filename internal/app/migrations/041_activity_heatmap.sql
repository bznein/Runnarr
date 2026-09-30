create table activity_heatmap_routes (
    activity_id uuid primary key references activities(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    version integer not null,
    geometry geometry(MultiLineString, 3857),
    medium_geometry geometry(MultiLineString, 3857),
    coarse_geometry geometry(MultiLineString, 3857),
    indexed_at timestamptz not null default now()
);
create index activity_heatmap_routes_user_idx on activity_heatmap_routes(user_id);
create index activity_heatmap_routes_geometry_idx on activity_heatmap_routes using gist(geometry);

create table heatmap_revisions (
    user_id uuid primary key references users(id) on delete cascade,
    revision bigint not null default 0
);

-- Keep revision invalidation transactional, including cascading deletions.
create function bump_heatmap_revision() returns trigger language plpgsql as $$
declare owner_id uuid;
begin
    if TG_OP = 'DELETE' then owner_id := OLD.user_id; else owner_id := NEW.user_id; end if;
    insert into heatmap_revisions(user_id, revision)
        select id, 1 from users where id = owner_id
        on conflict(user_id) do update set revision = heatmap_revisions.revision + 1;
    return null;
end;
$$;
create trigger heatmap_route_changed after insert or update or delete on activity_heatmap_routes
    for each row execute function bump_heatmap_revision();
