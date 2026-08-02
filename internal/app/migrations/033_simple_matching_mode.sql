alter table user_settings
    add column if not exists default_experience text not null default 'full';

alter table user_settings
    drop constraint if exists user_settings_default_experience_check;

alter table user_settings
    add constraint user_settings_default_experience_check
    check (default_experience in ('full', 'simple'));
