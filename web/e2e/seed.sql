\set ON_ERROR_STOP on

insert into daily_health_metrics(
    user_id, provider, metric_date, steps, total_calories_kcal,
    active_calories_kcal, resting_heart_rate_bpm, avg_heart_rate_bpm,
    max_heart_rate_bpm, sleep_duration_s, deep_sleep_s, light_sleep_s,
    rem_sleep_s, awake_sleep_s, sleep_score, stress_avg, stress_max,
    body_battery_avg, body_battery_min, body_battery_max, hrv_avg_ms,
    hrv_status, weight_kg, body_fat_pct
)
select id, 'garmin', current_date, 12450, 2380, 780, 48, 71, 156,
    27900, 7200, 14400, 6300, 900, 86, 23, 61, 72, 41, 98, 58,
    'balanced', 68.4, 14.2
from users
where username = :'e2e_username'
on conflict (user_id, provider, metric_date) do update set
    steps = excluded.steps,
    total_calories_kcal = excluded.total_calories_kcal,
    active_calories_kcal = excluded.active_calories_kcal,
    resting_heart_rate_bpm = excluded.resting_heart_rate_bpm,
    avg_heart_rate_bpm = excluded.avg_heart_rate_bpm,
    max_heart_rate_bpm = excluded.max_heart_rate_bpm,
    sleep_duration_s = excluded.sleep_duration_s,
    deep_sleep_s = excluded.deep_sleep_s,
    light_sleep_s = excluded.light_sleep_s,
    rem_sleep_s = excluded.rem_sleep_s,
    awake_sleep_s = excluded.awake_sleep_s,
    sleep_score = excluded.sleep_score,
    stress_avg = excluded.stress_avg,
    stress_max = excluded.stress_max,
    body_battery_avg = excluded.body_battery_avg,
    body_battery_min = excluded.body_battery_min,
    body_battery_max = excluded.body_battery_max,
    hrv_avg_ms = excluded.hrv_avg_ms,
    hrv_status = excluded.hrv_status,
    weight_kg = excluded.weight_kg,
    body_fat_pct = excluded.body_fat_pct;

insert into activities(
    user_id, source, source_id, name, sport_type, start_time,
    distance_m, moving_time_s, elapsed_time_s, raw
)
select id, 'e2e', 'e2e-pool-swim', 'E2E Pool Swim', 'Swimming',
    current_date + time '07:00', 1500, 1800, 1900, '{}'::jsonb
from users
where username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    name = excluded.name,
    sport_type = excluded.sport_type,
    start_time = excluded.start_time,
    distance_m = excluded.distance_m,
    moving_time_s = excluded.moving_time_s,
    elapsed_time_s = excluded.elapsed_time_s,
    raw = excluded.raw;

insert into gears(
    user_id, provider, provider_gear_id, name, gear_type, brand, model,
    retired, total_distance_m, max_distance_m, first_used_at, last_used_at,
    default_activity_types
)
select id, 'garmin', 'e2e-shoes', 'E2E Daily Trainers', 'shoes', 'Runnarr',
    'Test Trainer', false, 423000, 800000, now() - interval '90 days', now() - interval '1 day',
    array['running']::text[]
from users
where username = :'e2e_username'
on conflict (user_id, provider, provider_gear_id) do update set
    name = excluded.name,
    gear_type = excluded.gear_type,
    brand = excluded.brand,
    model = excluded.model,
    retired = excluded.retired,
    total_distance_m = excluded.total_distance_m,
    max_distance_m = excluded.max_distance_m,
    first_used_at = excluded.first_used_at,
    last_used_at = excluded.last_used_at,
    default_activity_types = excluded.default_activity_types;

insert into activities(
    user_id, source, source_id, name, sport_type, start_time,
    distance_m, moving_time_s, elapsed_time_s, raw
)
select id, 'e2e', 'e2e-cycling-activity', 'E2E Cycling Activity', 'Cycling',
    current_date + time '06:00', 25000, 3600, 3750, '{}'::jsonb
from users
where username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    name = excluded.name,
    sport_type = excluded.sport_type,
    start_time = excluded.start_time,
    distance_m = excluded.distance_m,
    moving_time_s = excluded.moving_time_s,
    elapsed_time_s = excluded.elapsed_time_s,
    raw = excluded.raw;

insert into planned_activities(
    user_id, source, source_id, workbook_id, sheet_id, sheet_title,
    plan_cell, planned_date, name, sport_type, status, raw
)
select id, 'training_sheet', 'e2e-planned-run', 'e2e-workbook', 'e2e-sheet',
    'E2E Plan', 'A1', current_date - 1, 'E2E Planned Run', 'Run', 'pending', '{}'::jsonb
from users
where username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    planned_date = excluded.planned_date,
    name = excluded.name,
    sport_type = excluded.sport_type,
    status = excluded.status,
    matched_activity_id = null,
    matched_at = null,
    raw = excluded.raw;

insert into planned_activities(
    user_id, source, source_id, workbook_id, sheet_id, sheet_title,
    plan_cell, planned_date, name, sport_type, status, raw
)
select id, 'training_sheet', 'e2e-planned-recovery', 'e2e-workbook', 'e2e-sheet',
    'E2E Plan', 'A2', current_date - 2, 'E2E Planned Recovery Run', 'Run', 'pending', '{}'::jsonb
from users
where username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    planned_date = excluded.planned_date,
    name = excluded.name,
    sport_type = excluded.sport_type,
    status = excluded.status,
    matched_activity_id = null,
    matched_at = null,
    raw = excluded.raw;

insert into planned_activities(
    user_id, source, source_id, workbook_id, sheet_id, sheet_title,
    plan_cell, planned_date, name, sport_type, status, raw
)
select id, 'training_sheet', 'e2e-planned-speed', 'e2e-workbook', 'e2e-sheet',
    'E2E Plan', 'A3', current_date - 1, 'E2E Planned Speed Work', 'Run', 'pending', '{}'::jsonb
from users
where username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    planned_date = excluded.planned_date,
    name = excluded.name,
    sport_type = excluded.sport_type,
    status = excluded.status,
    matched_activity_id = null,
    matched_at = null,
    raw = excluded.raw;

insert into planned_activities(
    user_id, source, source_id, workbook_id, sheet_id, sheet_title,
    plan_cell, planned_date, name, sport_type, status, raw
)
select id, 'training_sheet', 'e2e-planned-long', 'e2e-workbook', 'e2e-sheet',
    'E2E Plan', 'A4', current_date + 3, 'E2E Planned Long Run', 'Run', 'pending', '{}'::jsonb
from users
where username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    planned_date = excluded.planned_date,
    name = excluded.name,
    sport_type = excluded.sport_type,
    status = excluded.status,
    matched_activity_id = null,
    matched_at = null,
    raw = excluded.raw;

insert into planned_activities(
    user_id, source, source_id, workbook_id, sheet_id, sheet_title,
    plan_cell, planned_date, name, sport_type, status, raw
)
select id, 'training_sheet', 'e2e-planned-far', 'e2e-workbook', 'e2e-sheet',
    'E2E Plan', 'A5', current_date + 14, 'E2E Planned Far Run', 'Run', 'pending', '{}'::jsonb
from users
where username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    planned_date = excluded.planned_date,
    name = excluded.name,
    sport_type = excluded.sport_type,
    status = excluded.status,
    matched_activity_id = null,
    matched_at = null,
    raw = excluded.raw;
