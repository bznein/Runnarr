alter table activities
	add column if not exists local_name text not null default '',
	add column if not exists original_provider_url text not null default '';

update activities
set original_provider_url = 'https://connect.garmin.com/modern/activity/' || source_id
where source = 'garmin'
	and source_id <> ''
	and original_provider_url = '';
