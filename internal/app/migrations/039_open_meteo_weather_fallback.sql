alter table user_settings
    add column if not exists open_meteo_weather_enabled boolean not null default false;

create table if not exists external_api_rate_limit_usage (
    provider text not null,
    window_kind text not null check (window_kind in ('minute', 'hour', 'day', 'month')),
    window_start timestamptz not null,
    request_count integer not null default 0 check (request_count >= 0),
    updated_at timestamptz not null default now(),
    primary key (provider, window_kind, window_start)
);
