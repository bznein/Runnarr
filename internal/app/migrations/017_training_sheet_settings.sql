alter table if exists app_settings
    add column if not exists training_sheet_enabled boolean not null default false,
    add column if not exists training_sheet_sheet_url text not null default '',
    add column if not exists training_sheet_check_every_hours integer not null default 24,
    add column if not exists training_sheet_last_synced_at timestamptz,
    add column if not exists plan_year integer not null default 0;
