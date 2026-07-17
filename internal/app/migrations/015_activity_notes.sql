alter table activities
	add column if not exists local_notes text not null default '';
