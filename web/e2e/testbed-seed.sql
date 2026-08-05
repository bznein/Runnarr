\set ON_ERROR_STOP on

-- This file runs in a separate psql process from seed.sql, so it needs the
-- same compatibility defaults when the host deployment helper is older than
-- the candidate image.
\if :{?e2e_date}
\else
select to_char(current_timestamp at time zone 'Europe/Dublin', 'YYYY-MM-DD') as e2e_date \gset
\endif
\if :{?e2e_now}
\else
select :'e2e_date' || 'T12:00:00Z' as e2e_now \gset
\endif

update courses
set name = 'Testbed Riverside Loop',
    notes = 'Synthetic course. Safe to favorite, duplicate, export, or delete.'
where id = '00000000-0000-4000-8000-000000000180'::uuid
  and user_id = (select id from users where username = :'e2e_username');

-- Give the deterministic browser-test fixtures product-neutral names when
-- they are displayed in the general-purpose testbed.
update activities
set name = case source_id
    when 'e2e-pool-swim' then 'Testbed Pool Swim'
    when 'e2e-strength-activity' then 'Testbed Strength Session'
    when 'e2e-cycling-activity' then 'Testbed Interval Ride'
    else name
end
where user_id = (select id from users where username = :'e2e_username')
  and source = 'e2e';

update gears
set name = 'Testbed Daily Trainers',
    brand = 'Northstar',
    model = 'Daily One'
where user_id = (select id from users where username = :'e2e_username')
  and provider = 'garmin'
  and provider_gear_id = 'e2e-shoes';

update planned_activities
set sheet_title = 'Testbed Plan',
    name = case source_id
        when 'e2e-planned-run' then '2mins Easy Run'
        when 'e2e-planned-recovery' then 'Recovery Run'
        when 'e2e-planned-speed' then 'Speed Session'
        when 'e2e-planned-long' then '2 hours Long Run'
        when 'e2e-planned-far' then 'Future Easy Run'
        when 'e2e-workbook:e2e-sheet:A6' then 'Steady Run A'
        when 'e2e-workbook:e2e-sheet:A7' then 'Steady Run B'
        else name
    end
where user_id = (select id from users where username = :'e2e_username')
  and source = 'training_sheet';

insert into activities(
    user_id, source, source_id, name, sport_type, start_time,
    distance_m, moving_time_s, elapsed_time_s, elevation_gain_m,
    avg_heart_rate, max_heart_rate, avg_pace_s_per_km, calories_kcal,
    local_notes, raw
)
select users.id, 'testbed', fixture.source_id, fixture.name, fixture.sport_type,
    fixture.start_time, fixture.distance_m, fixture.moving_time_s,
    fixture.elapsed_time_s, fixture.elevation_gain_m, fixture.avg_heart_rate,
    fixture.max_heart_rate,
    case when fixture.distance_m > 0 and fixture.sport_type in ('Run', 'Walking', 'Hiking')
        then fixture.moving_time_s::double precision / fixture.distance_m * 1000
        else null
    end,
    fixture.calories_kcal,
    'Synthetic testbed activity. Safe to rename, match, or otherwise edit.',
    jsonb_build_object('fixture', 'testbed', 'sequence', fixture.sequence)
from users
cross join lateral (
    select sequence,
        format('testbed-activity-%s', lpad(sequence::text, 2, '0')) as source_id,
        case sequence % 7
            when 0 then format('Easy Riverside Run %s', lpad(sequence::text, 2, '0'))
            when 1 then format('Tempo Run %s', lpad(sequence::text, 2, '0'))
            when 2 then format('Rolling Roads Ride %s', lpad(sequence::text, 2, '0'))
            when 3 then format('Full Body Strength %s', lpad(sequence::text, 2, '0'))
            when 4 then format('Lunch Walk %s', lpad(sequence::text, 2, '0'))
            when 5 then format('Pool Endurance %s', lpad(sequence::text, 2, '0'))
            else format('Hill Hike %s', lpad(sequence::text, 2, '0'))
        end as name,
        case sequence % 7
            when 0 then 'Run'
            when 1 then 'Run'
            when 2 then 'Cycling'
            when 3 then 'Strength Training'
            when 4 then 'Walking'
            when 5 then 'Swimming'
            else 'Hiking'
        end as sport_type,
        (:'e2e_date'::date - sequence * 2) + make_time(6 + sequence % 4, sequence * 7 % 60, 0) as start_time,
        case sequence % 7
            when 0 then 6000 + sequence * 80
            when 1 then 8000 + sequence * 100
            when 2 then 28000 + sequence * 350
            when 3 then 0
            when 4 then 4500 + sequence * 40
            when 5 then 1500 + sequence % 4 * 250
            else 9000 + sequence * 120
        end::double precision as distance_m,
        case sequence % 7
            when 0 then 2100 + sequence * 8
            when 1 then 2500 + sequence * 8
            when 2 then 4200 + sequence * 20
            when 3 then 2700
            when 4 then 3600 + sequence * 15
            when 5 then 2000 + sequence * 20
            else 7200 + sequence * 30
        end as moving_time_s,
        case sequence % 7
            when 0 then 2160 + sequence * 8
            when 1 then 2620 + sequence * 8
            when 2 then 4380 + sequence * 20
            when 3 then 2850
            when 4 then 3720 + sequence * 15
            when 5 then 2120 + sequence * 20
            else 7800 + sequence * 30
        end as elapsed_time_s,
        case sequence % 7
            when 0 then 45 + sequence * 2
            when 1 then 70 + sequence * 3
            when 2 then 220 + sequence * 5
            when 3 then 0
            when 4 then 35 + sequence
            when 5 then 0
            else 520 + sequence * 8
        end::double precision as elevation_gain_m,
        case sequence % 7
            when 0 then 138 + sequence % 8
            when 1 then 154 + sequence % 8
            when 2 then 142 + sequence % 10
            when 3 then 118 + sequence % 12
            when 4 then 101 + sequence % 8
            when 5 then 132 + sequence % 10
            else 127 + sequence % 10
        end::double precision as avg_heart_rate,
        case sequence % 7
            when 0 then 161 + sequence % 8
            when 1 then 178 + sequence % 6
            when 2 then 169 + sequence % 8
            when 3 then 151 + sequence % 10
            when 4 then 122 + sequence % 8
            when 5 then 158 + sequence % 8
            else 154 + sequence % 8
        end::double precision as max_heart_rate,
        case sequence % 7
            when 0 then 360 + sequence * 3
            when 1 then 470 + sequence * 3
            when 2 then 760 + sequence * 6
            when 3 then 310 + sequence * 2
            when 4 then 290 + sequence * 2
            when 5 then 420 + sequence * 3
            else 980 + sequence * 5
        end as calories_kcal
    from generate_series(1, 42) as sequence
) as fixture
where users.username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    name = excluded.name,
    sport_type = excluded.sport_type,
    start_time = excluded.start_time,
    distance_m = excluded.distance_m,
    moving_time_s = excluded.moving_time_s,
    elapsed_time_s = excluded.elapsed_time_s,
    elevation_gain_m = excluded.elevation_gain_m,
    avg_heart_rate = excluded.avg_heart_rate,
    max_heart_rate = excluded.max_heart_rate,
    avg_pace_s_per_km = excluded.avg_pace_s_per_km,
    calories_kcal = excluded.calories_kcal,
    local_notes = excluded.local_notes,
    raw = excluded.raw;

insert into activity_samples(
    activity_id, sample_index, timestamp, elapsed_s, distance_m,
    latitude, longitude, elevation_m, heart_rate, cadence, power, speed_mps
)
select activity.id, sample_index,
    activity.start_time + (activity.moving_time_s * sample_index / 5) * interval '1 second',
    activity.moving_time_s * sample_index / 5,
    activity.distance_m * sample_index / 5,
    53.3400 + activity_offset * 0.002 + sample_index * 0.0005,
    -6.2600 + activity_offset * 0.002 + sample_index * 0.0007,
    25 + activity_offset * 2 + sample_index * 3,
    round(coalesce(activity.avg_heart_rate, 120) - 8 + sample_index * 3)::integer,
    case when activity.sport_type = 'Run' then 164 + sample_index * 2
         when activity.sport_type = 'Cycling' then 78 + sample_index
         else null end,
    case when activity.sport_type = 'Cycling' then 165 + sample_index * 12 else null end,
    case when activity.moving_time_s > 0 then activity.distance_m / activity.moving_time_s else null end
from activities activity
cross join generate_series(0, 5) as sample_index
cross join lateral (
    select substring(activity.source_id from '(\d+)$')::integer as activity_offset
) offsets
where activity.user_id = (select id from users where username = :'e2e_username')
  and activity.source = 'testbed'
  and activity.distance_m > 0
  and activity.sport_type in ('Run', 'Cycling', 'Walking', 'Hiking')
on conflict (activity_id, sample_index) do update set
    timestamp = excluded.timestamp,
    elapsed_s = excluded.elapsed_s,
    distance_m = excluded.distance_m,
    latitude = excluded.latitude,
    longitude = excluded.longitude,
    elevation_m = excluded.elevation_m,
    heart_rate = excluded.heart_rate,
    cadence = excluded.cadence,
    power = excluded.power,
    speed_mps = excluded.speed_mps;

insert into activity_laps(
    activity_id, lap_index, start_time, elapsed_time_s, moving_time_s,
    distance_m, avg_pace_s_per_km, elevation_gain_m, elevation_loss_m, raw
)
select activity.id, lap_index,
    activity.start_time + (activity.elapsed_time_s * lap_index / 3) * interval '1 second',
    activity.elapsed_time_s / 3,
    activity.moving_time_s / 3,
    activity.distance_m / 3,
    case when activity.distance_m > 0 then activity.moving_time_s / activity.distance_m * 1000 else null end,
    activity.elevation_gain_m / 3,
    activity.elevation_gain_m / 4,
    jsonb_build_object('fixture', 'testbed')
from activities activity
cross join generate_series(0, 2) as lap_index
where activity.user_id = (select id from users where username = :'e2e_username')
  and activity.source = 'testbed'
  and activity.distance_m > 0
on conflict (activity_id, lap_index) do update set
    start_time = excluded.start_time,
    elapsed_time_s = excluded.elapsed_time_s,
    moving_time_s = excluded.moving_time_s,
    distance_m = excluded.distance_m,
    avg_pace_s_per_km = excluded.avg_pace_s_per_km,
    elevation_gain_m = excluded.elevation_gain_m,
    elevation_loss_m = excluded.elevation_loss_m,
    raw = excluded.raw;

insert into activity_workouts(
    activity_id, provider, provider_workout_id, name, sport_type, steps, raw
)
select activity.id, 'testbed', activity.source_id, 'Tempo intervals', 'Run',
    '[{"index":0,"order":0,"type":"warmup"},{"index":1,"order":1,"type":"repeat","repeatCount":4},{"index":2,"order":2,"type":"cooldown"}]'::jsonb,
    '{"fixture":"testbed"}'::jsonb
from activities activity
where activity.user_id = (select id from users where username = :'e2e_username')
  and activity.source = 'testbed'
  and activity.source_id ~ '(01|08|15|22|29|36)$'
on conflict (activity_id) do update set
    provider = excluded.provider,
    provider_workout_id = excluded.provider_workout_id,
    name = excluded.name,
    sport_type = excluded.sport_type,
    steps = excluded.steps,
    raw = excluded.raw,
    updated_at = :'e2e_now'::timestamptz;

insert into activity_intervals(
    activity_id, interval_index, category, provider_type,
    workout_step_index, workout_repeat_index, start_time, end_time,
    elapsed_time_s, moving_time_s, distance_m, avg_heart_rate,
    max_heart_rate, lap_indexes, raw
)
select activity.id, repeat_index, 'active', 'run', 1, repeat_index,
    activity.start_time + (300 + repeat_index * 420) * interval '1 second',
    activity.start_time + (600 + repeat_index * 420) * interval '1 second',
    300, 290, 1000,
    coalesce(activity.avg_heart_rate, 150) + repeat_index * 2,
    coalesce(activity.max_heart_rate, 175),
    array[least(repeat_index, 2)],
    '{"fixture":"testbed"}'::jsonb
from activities activity
cross join generate_series(0, 3) as repeat_index
where activity.user_id = (select id from users where username = :'e2e_username')
  and activity.source = 'testbed'
  and activity.source_id ~ '(01|08|15|22|29|36)$'
on conflict (activity_id, interval_index) do update set
    category = excluded.category,
    provider_type = excluded.provider_type,
    workout_step_index = excluded.workout_step_index,
    workout_repeat_index = excluded.workout_repeat_index,
    start_time = excluded.start_time,
    end_time = excluded.end_time,
    elapsed_time_s = excluded.elapsed_time_s,
    moving_time_s = excluded.moving_time_s,
    distance_m = excluded.distance_m,
    avg_heart_rate = excluded.avg_heart_rate,
    max_heart_rate = excluded.max_heart_rate,
    lap_indexes = excluded.lap_indexes,
    raw = excluded.raw;

insert into daily_health_metrics(
    user_id, provider, metric_date, steps, total_calories_kcal,
    active_calories_kcal, resting_heart_rate_bpm, avg_heart_rate_bpm,
    max_heart_rate_bpm, sleep_duration_s, deep_sleep_s, light_sleep_s,
    rem_sleep_s, awake_sleep_s, sleep_score, stress_avg, stress_max,
    body_battery_avg, body_battery_min, body_battery_max, hrv_avg_ms,
    hrv_status, weight_kg, body_fat_pct
)
select users.id, 'garmin', :'e2e_date'::date - day,
    6500 + day * 137 % 9000,
    2050 + day * 17 % 650,
    350 + day * 23 % 700,
    46 + day % 9,
    67 + day % 12,
    142 + day % 28,
    25200 + day * 180 % 7200,
    5400 + day * 90 % 2700,
    12600 + day * 120 % 3600,
    5400 + day * 75 % 2400,
    600 + day * 30 % 900,
    68 + day * 7 % 28,
    18 + day * 3 % 30,
    48 + day * 5 % 35,
    58 + day % 20,
    28 + day % 18,
    82 + day % 16,
    48 + day % 18,
    case when day % 5 = 0 then 'low' else 'balanced' end,
    68.0 + (day % 10)::double precision / 10,
    14.0 + (day % 8)::double precision / 10
from users
cross join generate_series(1, 60) as day
where users.username = :'e2e_username'
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
select id, 'testbed', 'testbed-current-run', 'Morning Shakeout', 'Run',
    :'e2e_date'::date + time '06:15', 400, 120, 120, '{"fixture":"testbed"}'::jsonb
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

insert into activities(
    user_id, source, source_id, name, sport_type, start_time,
    distance_m, moving_time_s, elapsed_time_s, raw
)
select id, 'testbed', 'testbed-historic-run', 'Canal Recovery Run', 'Run',
    (:'e2e_date'::date - 45) + time '06:15', 400, 120, 120, '{"fixture":"testbed"}'::jsonb
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
select users.id, 'training_sheet', fixture.source_id, 'testbed-workbook', 'testbed-sheet',
    'Testbed Plan', fixture.plan_cell, :'e2e_date'::date - 45, fixture.name, 'Run', 'pending',
    '{"planCellBackgroundColor":"#ffffff"}'::jsonb
from users
cross join (values
    ('testbed-historic-plan-a', 'B2', '2mins Base Run'),
    ('testbed-historic-plan-b', 'C2', '2mins Aerobic Run')
) as fixture(source_id, plan_cell, name)
where users.username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    planned_date = excluded.planned_date,
    name = excluded.name,
    sport_type = excluded.sport_type,
    status = excluded.status,
    matched_activity_id = null,
    matched_at = null,
    raw = excluded.raw;

update workouts
set name = case id
        when '00000000-0000-4000-8000-000000000170'::uuid then 'Threshold Repeats'
        when '00000000-0000-4000-8000-000000000172'::uuid then 'Canal Tempo'
        else name
    end,
    updated_at = :'e2e_now'::timestamptz
where user_id = (select id from users where username = :'e2e_username')
  and id in (
    '00000000-0000-4000-8000-000000000170'::uuid,
    '00000000-0000-4000-8000-000000000172'::uuid
  );

update planned_activities
set name = 'Canal Tempo', updated_at = :'e2e_now'::timestamptz
where user_id = (select id from users where username = :'e2e_username')
  and id = '00000000-0000-4000-8000-000000000171'::uuid;

insert into provider_connections(user_id, provider, provider_account_id, display_name, scopes, metadata)
select id, 'garmin', 'testbed-garmin', 'Offline Garmin Testbed', array['garmin-connect'],
    '{"fixture":"testbed"}'::jsonb
from users
where username = :'e2e_username'
on conflict(user_id, provider) do update set
    provider_account_id = excluded.provider_account_id,
    display_name = excluded.display_name,
    scopes = excluded.scopes,
    metadata = excluded.metadata,
    connected_at = :'e2e_now'::timestamptz,
    updated_at = :'e2e_now'::timestamptz;

update user_settings
set workout_sync_enabled = true,
    workout_default_pace_tolerance_s = 0,
    workout_timezone = 'UTC',
    updated_at = :'e2e_now'::timestamptz
where user_id = (select id from users where username = :'e2e_username');
