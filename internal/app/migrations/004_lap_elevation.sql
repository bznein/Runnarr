alter table activity_laps
	add column if not exists elevation_gain_m double precision,
	add column if not exists elevation_loss_m double precision;
