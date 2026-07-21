alter table activity_laps
    add column if not exists moving_time_s integer not null default 0,
    add column if not exists avg_pace_s_per_km double precision;
