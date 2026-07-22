alter table training_sheet_writebacks
    add column if not exists interval_status text not null default 'not_applicable',
    add column if not exists interval_error text not null default '',
    add column if not exists interval_written_at timestamptz;
