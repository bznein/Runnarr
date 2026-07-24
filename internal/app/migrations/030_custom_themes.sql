alter table user_settings drop constraint if exists user_settings_theme_preference_check;

update user_settings
set theme_preference = case theme_preference
    when 'light' then 'runnarr'
    when 'dark' then 'midnight'
    else theme_preference
end;

alter table user_settings
    add constraint user_settings_theme_preference_check
    check (theme_preference in ('system', 'runnarr', 'ocean', 'sunset', 'midnight'));
