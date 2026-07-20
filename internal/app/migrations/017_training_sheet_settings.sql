alter table if exists app_settings
    add column if not exists training_sheet_enabled boolean not null default false,
    add column if not exists training_sheet_sheet_url text not null default '',
    add column if not exists training_sheet_check_every_hours integer not null default 24,
    add column if not exists training_sheet_last_synced_at timestamptz;

insert into app_settings(
    id,
    climb_smoothing_radius_m,
    min_climb_distance_m,
    min_climb_elevation_gain_m,
    min_climb_average_grade_pct,
    max_climb_merge_dip_distance_m,
    max_climb_merge_elevation_loss_m,
    climb_start_gain_m,
    climb_detection_preset
) values(
    'default',
    75,
    300,
    15,
    2.5,
    150,
    8,
    3,
    'balanced'
) on conflict(id) do nothing;
