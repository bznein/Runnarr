update activity_intervals
set avg_pace_s_per_km = 1000 / ((raw ->> 'averageSpeed')::double precision)
where jsonb_typeof(raw -> 'averageSpeed') = 'number'
  and (raw ->> 'averageSpeed')::double precision > 0
  and exists (
    select 1 from activities
    where activities.id = activity_intervals.activity_id
      and activities.source = 'garmin'
  )
  and avg_pace_s_per_km is distinct from 1000 / ((raw ->> 'averageSpeed')::double precision);
