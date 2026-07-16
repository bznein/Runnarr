alter table activity_laps
	add column if not exists avg_grade_adjusted_pace_s_per_km double precision;
