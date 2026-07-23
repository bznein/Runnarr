\set ON_ERROR_STOP on

insert into daily_health_metrics(
    user_id, provider, metric_date, steps, total_calories_kcal,
    active_calories_kcal, resting_heart_rate_bpm, avg_heart_rate_bpm,
    max_heart_rate_bpm, sleep_duration_s, deep_sleep_s, light_sleep_s,
    rem_sleep_s, awake_sleep_s, sleep_score, stress_avg, stress_max,
    body_battery_avg, body_battery_min, body_battery_max, hrv_avg_ms,
    hrv_status, weight_kg, body_fat_pct
)
select id, 'garmin', current_date - 1, 12450, 2380, 780, 48, 71, 156,
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
