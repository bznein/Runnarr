update activities
set sport_type = 'Strength'
where lower(replace(sport_type, ' ', '')) in (
	'strength',
	'strengthtraining',
	'weighttraining',
	'weightlifting',
	'workout'
);
