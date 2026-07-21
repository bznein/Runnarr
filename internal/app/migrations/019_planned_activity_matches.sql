alter table if exists planned_activities
    add column if not exists matched_activity_id uuid references activities(id) on delete set null,
    add column if not exists matched_at timestamptz;

create unique index if not exists planned_activities_matched_activity_idx
    on planned_activities(matched_activity_id)
    where matched_activity_id is not null;
