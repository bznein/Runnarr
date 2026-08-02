alter table app_settings
    add column if not exists web_push_private_key_ciphertext bytea not null default decode('', 'hex'),
    add column if not exists web_push_public_key text not null default '';

create table if not exists notification_threads (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    thread_key text not null,
    category text not null,
    kind text not null,
    severity text not null check (severity in ('info', 'success', 'warning', 'error')),
    title text not null,
    body text not null default '',
    action_path text not null default '/notifications',
    read_at timestamptz,
    created_at timestamptz not null default now(),
    last_event_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique(user_id, thread_key)
);

create index if not exists notification_threads_user_order_idx
    on notification_threads(user_id, last_event_at desc, id desc);
create index if not exists notification_threads_user_unread_idx
    on notification_threads(user_id, last_event_at desc)
    where read_at is null;

create table if not exists notification_events (
    id uuid primary key default gen_random_uuid(),
    thread_id uuid not null references notification_threads(id) on delete cascade,
    event_key text not null,
    category text not null,
    kind text not null,
    severity text not null check (severity in ('info', 'success', 'warning', 'error')),
    title text not null,
    body text not null default '',
    action_path text not null default '/notifications',
    created_at timestamptz not null default now(),
    unique(thread_id, event_key)
);

create index if not exists notification_events_thread_order_idx
    on notification_events(thread_id, created_at, id);

create table if not exists notification_preferences (
    user_id uuid not null references users(id) on delete cascade,
    category text not null check (category in ('workout_changes', 'garmin_calendar', 'activity_matching', 'sheet_writeback')),
    mode text not null check (mode in ('off', 'in_app', 'in_app_push')),
    updated_at timestamptz not null default now(),
    primary key(user_id, category)
);

create table if not exists web_push_subscriptions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    endpoint_hash bytea not null unique,
    subscription_ciphertext bytea not null,
    device_name text not null,
    user_agent text not null default '',
    last_seen_at timestamptz not null default now(),
    last_success_at timestamptz,
    last_error text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists web_push_subscriptions_user_idx
    on web_push_subscriptions(user_id, updated_at desc);

create table if not exists web_push_outbox (
    id uuid primary key default gen_random_uuid(),
    thread_id uuid not null references notification_threads(id) on delete cascade,
    event_id uuid not null references notification_events(id) on delete cascade,
    subscription_id uuid not null references web_push_subscriptions(id) on delete cascade,
    payload jsonb not null,
    attempts integer not null default 0,
    available_at timestamptz not null default now(),
    locked_at timestamptz,
    last_error text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique(thread_id, subscription_id)
);

create index if not exists web_push_outbox_due_idx
    on web_push_outbox(available_at, created_at);
