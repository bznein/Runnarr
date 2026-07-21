alter table activities
    add column if not exists local_feedback text not null default '',
    add column if not exists rpe smallint check (rpe between 1 and 10);

alter table planned_activities
    add column if not exists feedback_cell text not null default '';

alter table google_sheets_tokens
    add column if not exists scopes text[] not null default '{}';

create table if not exists training_sheet_writebacks (
    planned_activity_id uuid primary key references planned_activities(id) on delete cascade,
    activity_id uuid not null references activities(id) on delete cascade,
    summary_status text not null default 'pending',
    summary_error text not null default '',
    summary_written_at timestamptz,
    feedback_status text not null default 'waiting_for_feedback',
    feedback_error text not null default '',
    feedback_written_at timestamptz,
    last_attempt_at timestamptz,
    updated_at timestamptz not null default now()
);
