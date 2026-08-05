alter table user_settings
    add column if not exists course_start_latitude double precision,
    add column if not exists course_start_longitude double precision;

alter table user_settings
    drop constraint if exists user_settings_course_start_location_check;

alter table user_settings
    add constraint user_settings_course_start_location_check
    check (
        (course_start_latitude is null and course_start_longitude is null)
        or (
            course_start_latitude is not null
            and course_start_longitude is not null
            and
            course_start_latitude between -90 and 90
            and course_start_longitude between -180 and 180
        )
    );
