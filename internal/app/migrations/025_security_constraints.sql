with ranked_running_jobs as (
    select id,
           row_number() over (partition by user_id, provider order by created_at, id) as job_rank
    from sync_jobs
    where status = 'running'
)
update sync_jobs
set status = 'failed',
    error = 'Superseded while installing the active sync-job constraint',
    finished_at = now()
from ranked_running_jobs
where sync_jobs.id = ranked_running_jobs.id
  and ranked_running_jobs.job_rank > 1;

create unique index if not exists sync_jobs_active_user_provider_idx
    on sync_jobs(user_id, provider)
    where status = 'running';
