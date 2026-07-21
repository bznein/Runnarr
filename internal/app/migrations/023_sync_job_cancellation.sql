alter table sync_jobs
    add column if not exists cancel_requested_at timestamptz;

create index if not exists sync_jobs_cancel_requested_idx
    on sync_jobs(status, cancel_requested_at)
    where status = 'running' and cancel_requested_at is not null;
