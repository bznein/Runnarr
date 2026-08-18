\set ON_ERROR_STOP on

-- Candidate images can reach a host before its deployment helper is refreshed.
-- Keep the seed compatible with older helpers while allowing E2E callers to
-- inject a deterministic fixture clock.
\if :{?e2e_date}
\else
select to_char(current_timestamp at time zone 'Europe/Dublin', 'YYYY-MM-DD') as e2e_date \gset
\endif
\if :{?e2e_now}
\else
select :'e2e_date' || 'T12:00:00Z' as e2e_now \gset
\endif

update user_settings
set training_sheet_sheet_url = 'https://docs.google.com/spreadsheets/d/e2e-workbook/edit',
    training_sheet_enabled = false,
    default_experience = 'full'
where user_id = (select id from users where username = :'e2e_username');

-- Garmin mutations in browser journeys are routed to the offline bridge.
insert into provider_connections(user_id, provider, provider_account_id, display_name, scopes, metadata)
select id, 'garmin', 'testbed-garmin', 'Offline Garmin Testbed', array['garmin-connect'],
    '{"fixture":"e2e"}'::jsonb
from users
where username = :'e2e_username'
on conflict(user_id, provider) do update set
    provider_account_id = excluded.provider_account_id,
    display_name = excluded.display_name,
    scopes = excluded.scopes,
    metadata = excluded.metadata,
    updated_at = :'e2e_now'::timestamptz;

insert into daily_health_metrics(
    user_id, provider, metric_date, steps, total_calories_kcal,
    active_calories_kcal, resting_heart_rate_bpm, avg_heart_rate_bpm,
    max_heart_rate_bpm, sleep_duration_s, deep_sleep_s, light_sleep_s,
    rem_sleep_s, awake_sleep_s, sleep_score, stress_avg, stress_max,
    body_battery_avg, body_battery_min, body_battery_max, hrv_avg_ms,
    hrv_status, weight_kg, body_fat_pct
)
select id, 'garmin', :'e2e_date'::date, 12450, 2380, 780, 48, 71, 156,
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

-- Keep the seeded activities outside the imported E2E GPX times so the
-- navigation journey has a deterministic Cycling -> imported -> Pool -> Strength
-- -> calendar-match order.
insert into activities(
    user_id, source, source_id, name, sport_type, start_time,
    distance_m, moving_time_s, elapsed_time_s, raw
)
select id, 'e2e', 'e2e-pool-swim', 'E2E Pool Swim', 'Swimming',
    :'e2e_date'::date + time '05:00', 1500, 1800, 1900, '{}'::jsonb
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

-- Keep a lap-only fixture for the Intervals fallback journey.
insert into activity_laps(
    activity_id, lap_index, start_time, elapsed_time_s, moving_time_s,
    distance_m, avg_pace_s_per_km, raw
)
select activity.id, 0, activity.start_time, 1900, 1800, 1500, 1200, '{}'::jsonb
from activities activity
join users on users.id = activity.user_id
where users.username = :'e2e_username'
  and activity.source = 'e2e'
  and activity.source_id = 'e2e-pool-swim'
on conflict (activity_id, lap_index) do update set
    start_time = excluded.start_time,
    elapsed_time_s = excluded.elapsed_time_s,
    moving_time_s = excluded.moving_time_s,
    distance_m = excluded.distance_m,
    avg_pace_s_per_km = excluded.avg_pace_s_per_km,
    raw = excluded.raw;

-- Deliberately include provider-style distance on a strength activity so the
-- browser journey verifies that distance and derived pace remain hidden.
insert into activities(
    user_id, source, source_id, name, sport_type, start_time,
    distance_m, moving_time_s, elapsed_time_s, raw
)
select id, 'e2e', 'e2e-strength-activity', 'E2E Strength Training', 'Strength Training',
    :'e2e_date'::date + time '04:00', 1000, 600, 660, '{}'::jsonb
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
    'Test Trainer', false, 423000, 800000, :'e2e_now'::timestamptz - interval '90 days', :'e2e_now'::timestamptz - interval '1 day',
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
    :'e2e_date'::date + time '07:00', 25000, 3600, 3750, '{}'::jsonb
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
select id, 'e2e', 'e2e-calendar-matched-run', 'E2E Calendar Matched Run', 'Run',
    :'e2e_date'::date + time '03:00', 8000, 2880, 3000, '{}'::jsonb
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

-- Model the training-sheet activity row created by a real plan import. Once
-- matched, this future placeholder must stay hidden while its provenance is
-- attached to the completed activity on the actual completion date.
insert into activities(
    user_id, source, source_id, name, sport_type, start_time,
    distance_m, moving_time_s, elapsed_time_s, raw
)
select id, 'training_sheet', 'e2e-calendar-planned-run', 'E2E Calendar Planned Run', 'Run',
    :'e2e_date'::date + 1, 0, 0, 0, '{}'::jsonb
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
    plan_cell, planned_date, name, sport_type, status, source_url, raw,
    matched_activity_id, matched_at
)
select users.id, 'training_sheet', 'e2e-calendar-planned-run', 'e2e-workbook', 'e2e-sheet',
    'E2E Plan', 'A0', :'e2e_date'::date + 1, 'E2E Calendar Planned Run', 'Run', 'completed',
    'https://docs.google.com/spreadsheets/d/e2e-workbook/edit#gid=e2e-sheet', '{}'::jsonb,
    matched_activity.id, :'e2e_now'::timestamptz
from users
join activities matched_activity
    on matched_activity.user_id = users.id
    and matched_activity.source = 'e2e'
    and matched_activity.source_id = 'e2e-calendar-matched-run'
where users.username = :'e2e_username'
on conflict (user_id, source, source_id) do update set
    planned_date = excluded.planned_date,
    name = excluded.name,
    sport_type = excluded.sport_type,
    status = excluded.status,
    source_url = excluded.source_url,
    matched_activity_id = excluded.matched_activity_id,
    matched_at = excluded.matched_at,
    raw = excluded.raw;

insert into training_sheet_writebacks(
    planned_activity_id, activity_id, summary_status, summary_error,
    interval_status, feedback_status, last_attempt_at
)
select planned.id, planned.matched_activity_id, 'failed', 'E2E writeback failure',
    'not_applicable', 'not_provided', :'e2e_now'::timestamptz
from planned_activities planned
join users on users.id = planned.user_id
where users.username = :'e2e_username'
  and planned.source = 'training_sheet'
  and planned.source_id = 'e2e-calendar-planned-run'
  and planned.matched_activity_id is not null
on conflict (planned_activity_id) do update set
    activity_id = excluded.activity_id,
    summary_status = excluded.summary_status,
    summary_error = excluded.summary_error,
    interval_status = excluded.interval_status,
    feedback_status = excluded.feedback_status,
    last_attempt_at = excluded.last_attempt_at;

insert into workouts(
    id, user_id, source, planned_activity_id, name, sport_type, source_text, source_hash,
    definition, parse_status, parse_messages, scheduled_date, garmin_excluded, revision
)
select '00000000-0000-4000-8000-000000000173'::uuid, planned.user_id, 'training_sheet', planned.id,
    'E2E Calendar Planned Run', 'Run', '48mins easy', 'e2e-completed-plan-workout',
    '{"version":1,"sportType":"Run","estimatedDurationS":2880,"steps":[
      {"order":1,"kind":"work","endCondition":{"type":"time","value":2880,"unit":"seconds"},"target":{"type":"none"}}
    ]}'::jsonb,
    'ready', '[]'::jsonb, planned.planned_date, true, 1
from planned_activities planned
join users on users.id = planned.user_id
where users.username = :'e2e_username' and planned.source_id = 'e2e-calendar-planned-run'
on conflict (id) do update set
    planned_activity_id = excluded.planned_activity_id,
    name = excluded.name,
    source_text = excluded.source_text,
    source_hash = excluded.source_hash,
    definition = excluded.definition,
    scheduled_date = excluded.scheduled_date,
    garmin_excluded = true,
    archived_at = null,
    updated_at = :'e2e_now'::timestamptz;

-- A deterministic spatial fixture keeps the course library and detail views
-- inspectable before the browser journey creates user-scoped courses itself.
insert into courses(
    id, user_id, name, sport_type, notes, favorite, revision, geometry_hash,
    distance_m, elevation_gain_m, elevation_loss_m, elevation_coverage,
    point_count, leg_count, direct_leg_count, diagnostics, created_at, updated_at
)
select '00000000-0000-4000-8000-000000000180'::uuid, id,
    'E2E Riverside Loop', 'Run', 'Synthetic course for library and map inspection.',
    true, 1, 'e2e-riverside-loop-v1', 4210, 38, 35, 1, 5, 1, 0,
    '{"fixture":"e2e"}'::jsonb, :'e2e_now'::timestamptz - interval '2 days', :'e2e_now'::timestamptz - interval '1 day'
from users
where username = :'e2e_username'
on conflict (id) do update set
    user_id = excluded.user_id,
    name = excluded.name,
    sport_type = excluded.sport_type,
    notes = excluded.notes,
    favorite = excluded.favorite,
    geometry_hash = excluded.geometry_hash,
    distance_m = excluded.distance_m,
    elevation_gain_m = excluded.elevation_gain_m,
    elevation_loss_m = excluded.elevation_loss_m,
    elevation_coverage = excluded.elevation_coverage,
    point_count = excluded.point_count,
    leg_count = excluded.leg_count,
    direct_leg_count = excluded.direct_leg_count,
    diagnostics = excluded.diagnostics,
    updated_at = excluded.updated_at;

insert into course_legs(id, course_id, leg_index, mode, geometry, elevations)
values (
    '00000000-0000-4000-8000-000000000181'::uuid,
    '00000000-0000-4000-8000-000000000180'::uuid,
    0,
    'preserved',
    st_geomfromtext('LINESTRING(-6.2603 53.3498,-6.2450 53.3560,-6.2350 53.3450,-6.2500 53.3380,-6.2603 53.3498)', 4326),
    array[22,37,31,25,22]::double precision[]
)
on conflict (id) do update set
    mode = excluded.mode,
    geometry = excluded.geometry,
    elevations = excluded.elevations;

-- Keep a structured-interval fixture for the Activity Detail tab journey.
insert into activity_workouts(
    activity_id, provider, provider_workout_id, name, sport_type, steps, raw
)
select activity.id, 'e2e', 'e2e-structured-ride', 'E2E Structured Ride',
    'Cycling', '[]'::jsonb, '{}'::jsonb
from activities activity
join users on users.id = activity.user_id
where users.username = :'e2e_username'
  and activity.source = 'e2e'
  and activity.source_id = 'e2e-cycling-activity'
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
    elapsed_time_s, moving_time_s, distance_m, avg_pace_s_per_km, raw
)
select activity.id, 0, 'active', 'ride', 900, 900, 6000, 150, '{}'::jsonb
from activities activity
join users on users.id = activity.user_id
where users.username = :'e2e_username'
  and activity.source = 'e2e'
  and activity.source_id = 'e2e-cycling-activity'
on conflict (activity_id, interval_index) do update set
    category = excluded.category,
    provider_type = excluded.provider_type,
    elapsed_time_s = excluded.elapsed_time_s,
    moving_time_s = excluded.moving_time_s,
    distance_m = excluded.distance_m,
    avg_pace_s_per_km = excluded.avg_pace_s_per_km,
    raw = excluded.raw;

insert into planned_activities(
    user_id, source, source_id, workbook_id, sheet_id, sheet_title,
    plan_cell, planned_date, name, sport_type, status, raw
)
select id, 'training_sheet', 'e2e-planned-run', 'e2e-workbook', 'e2e-sheet',
    'E2E Plan', 'A1', :'e2e_date'::date, '2mins E2E Planned Run', 'Run', 'pending',
    '{"planCellBackgroundColor":"#ffffff"}'::jsonb
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
    'E2E Plan', 'A2', :'e2e_date'::date - 2, 'E2E Planned Recovery Run', 'Run', 'pending', '{}'::jsonb
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
    'E2E Plan', 'A3', :'e2e_date'::date, 'E2E Planned Speed Work', 'Run', 'pending',
    '{"planCellBackgroundColor":"#3d85c6","workoutTable":{"rows":[{"label":"5min rep 1"}]}}'::jsonb
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
    'E2E Plan', 'A4', :'e2e_date'::date + 3, '2 hours E2E Planned Long Run', 'Run', 'pending',
    '{"planCellBackgroundColor":"#674ea7"}'::jsonb
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
    'E2E Plan', 'A5', :'e2e_date'::date + 14, 'E2E Planned Far Run', 'Run', 'pending', '{}'::jsonb
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
select users.id, 'training_sheet', fixture.source_id, 'e2e-workbook', 'e2e-sheet',
    'E2E Plan', fixture.plan_cell, :'e2e_date'::date + 3, fixture.name, 'Run', 'pending', '{}'::jsonb
from users
cross join (values
    ('e2e-workbook:e2e-sheet:A6', 'A6', 'E2E chromium Match Candidate'),
    ('e2e-workbook:e2e-sheet:A7', 'A7', 'E2E mobile-chromium Match Candidate')
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

insert into workouts(
    id, user_id, source, planned_activity_id, name, sport_type, source_text, source_hash,
    definition, parse_status, parse_messages, scheduled_date, revision
)
select '00000000-0000-4000-8000-000000000170'::uuid, planned.user_id, 'training_sheet', planned.id,
    'E2E Planned Speed Work', 'Run', '10mins warm up//4x5mins@4:00(90secs)//10mins cool down',
    'e2e-sheet-workout',
    '{"version":1,"sportType":"Run","estimatedDurationS":2490,"steps":[
      {"order":1,"kind":"warmup","endCondition":{"type":"time","value":600,"unit":"seconds"},"target":{"type":"none"}},
      {"order":2,"kind":"repeat","repeatCount":4,"skipLastRecovery":true,"target":{"type":"none"},"children":[
        {"order":1,"kind":"work","endCondition":{"type":"time","value":300,"unit":"seconds"},"target":{"type":"pace","paceSecondsPerKM":240}},
        {"order":2,"kind":"recovery","endCondition":{"type":"time","value":90,"unit":"seconds"},"target":{"type":"none"}}
      ]},
      {"order":3,"kind":"cooldown","endCondition":{"type":"time","value":600,"unit":"seconds"},"target":{"type":"none"}}
    ]}'::jsonb,
    'ready', '[]'::jsonb, planned.planned_date, 1
from planned_activities planned
join users on users.id = planned.user_id
where users.username = :'e2e_username' and planned.source = 'training_sheet' and planned.source_id = 'e2e-planned-speed'
on conflict (id) do update set
    planned_activity_id = excluded.planned_activity_id,
    name = excluded.name,
    source_text = excluded.source_text,
    source_hash = excluded.source_hash,
    definition = excluded.definition,
    parse_status = excluded.parse_status,
    parse_messages = excluded.parse_messages,
    scheduled_date = excluded.scheduled_date,
    archived_at = null,
    updated_at = :'e2e_now'::timestamptz;

insert into planned_activities(
    id, user_id, source, source_id, workbook_id, sheet_id, sheet_title,
    plan_cell, planned_date, name, sport_type, notes, status, raw
)
select '00000000-0000-4000-8000-000000000171'::uuid, id, 'manual',
    'workout:00000000-0000-4000-8000-000000000172', '', '', '', '', :'e2e_date'::date + 40,
    'E2E Manual Surges', 'Run', '', 'pending', '{"fixture":"e2e-workout"}'::jsonb
from users
where username = :'e2e_username'
on conflict (id) do update set
    planned_date = excluded.planned_date,
    name = excluded.name,
    status = 'pending',
    matched_activity_id = null,
    matched_at = null,
    raw = excluded.raw;

insert into workouts(
    id, user_id, source, planned_activity_id, name, sport_type, source_text, source_hash,
    definition, parse_status, parse_messages, scheduled_date, revision
)
select '00000000-0000-4000-8000-000000000172'::uuid, id, 'manual',
    '00000000-0000-4000-8000-000000000171'::uuid, 'E2E Manual Surges', 'Run',
    '47mins with surges', 'e2e-manual-workout',
    '{"version":1,"sportType":"Run","estimatedDurationS":2820,"steps":[
      {"order":1,"kind":"repeat","repeatCount":9,"target":{"type":"none"},"children":[
        {"order":2,"kind":"work","description":"Steady run","endCondition":{"type":"time","value":270,"unit":"seconds"},"target":{"type":"none"}},
        {"order":3,"kind":"work","description":"Surge","endCondition":{"type":"time","value":30,"unit":"seconds"},"target":{"type":"none"}}
      ]},
      {"order":4,"kind":"work","description":"Final run","endCondition":{"type":"time","value":120,"unit":"seconds"},"target":{"type":"none"}}
    ]}'::jsonb,
    'ready', '[]'::jsonb, :'e2e_date'::date + 40, 1
from users
where username = :'e2e_username'
on conflict (id) do update set
    planned_activity_id = excluded.planned_activity_id,
    name = excluded.name,
    source_text = excluded.source_text,
    source_hash = excluded.source_hash,
    definition = excluded.definition,
    parse_status = excluded.parse_status,
    parse_messages = excluded.parse_messages,
    scheduled_date = excluded.scheduled_date,
    archived_at = null,
    updated_at = :'e2e_now'::timestamptz;
