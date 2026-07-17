alter table daily_health_metrics
	add column if not exists body_battery_gained double precision,
	add column if not exists body_battery_drained double precision;

update daily_health_metrics
set
	body_battery_gained = coalesce(
		body_battery_gained,
		nullif(coalesce(raw #>> '{bodyBattery,0,charged}', raw #>> '{bodyBattery,charged}'), '')::double precision
	),
	body_battery_drained = coalesce(
		body_battery_drained,
		nullif(coalesce(raw #>> '{bodyBattery,0,drained}', raw #>> '{bodyBattery,drained}'), '')::double precision
	)
where body_battery_gained is null
	or body_battery_drained is null;
