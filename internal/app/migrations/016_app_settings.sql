create table if not exists app_settings (
    id text primary key,
    climb_smoothing_radius_m double precision not null check (climb_smoothing_radius_m > 0),
    min_climb_distance_m double precision not null check (min_climb_distance_m > 0),
    min_climb_elevation_gain_m double precision not null check (min_climb_elevation_gain_m > 0),
    min_climb_average_grade_pct double precision not null check (min_climb_average_grade_pct > 0),
    max_climb_merge_dip_distance_m double precision not null check (max_climb_merge_dip_distance_m > 0),
    max_climb_merge_elevation_loss_m double precision not null check (max_climb_merge_elevation_loss_m > 0),
    climb_start_gain_m double precision not null check (climb_start_gain_m > 0),
    climb_detection_preset text not null default 'balanced',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

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
