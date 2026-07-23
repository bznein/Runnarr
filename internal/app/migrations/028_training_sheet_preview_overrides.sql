alter table training_sheet_writebacks
    add column if not exists manual_overrides jsonb not null default '{}'::jsonb;

alter table training_sheet_writebacks
    alter column feedback_status set default 'not_provided';

update training_sheet_writebacks
set feedback_status = 'not_provided'
where feedback_status = 'waiting_for_feedback';
