alter table daily_health_metrics
	add column if not exists body_battery_start double precision,
	add column if not exists body_battery_end double precision;
