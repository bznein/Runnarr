alter table oidc_auth_states
    add column if not exists client text not null default 'web';

alter table google_oauth_states
    add column if not exists client text not null default 'web';

create table if not exists mobile_auth_handoffs (
    code_hash text primary key,
    session_id uuid not null references auth_sessions(id) on delete cascade,
    expires_at timestamptz not null,
    created_at timestamptz not null default now()
);

create index if not exists mobile_auth_handoffs_expiry_idx
    on mobile_auth_handoffs(expires_at);
