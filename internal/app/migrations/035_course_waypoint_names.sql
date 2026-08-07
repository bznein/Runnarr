alter table course_waypoints
    add column name text not null default ''
    check (char_length(name) <= 160);
