alter table activity_weather
    add column if not exists selection_method text not null default '',
    add column if not exists model text not null default '';
