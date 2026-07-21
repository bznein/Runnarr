create table if not exists oidc_auth_states (
    state_hash text primary key,
    nonce_hash text not null,
    expires_at timestamptz not null
);

create index if not exists oidc_auth_states_expiry_idx
    on oidc_auth_states(expires_at);

create table if not exists oidc_identities (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    issuer text not null,
    subject text not null,
    email text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (issuer, subject),
    unique (user_id, issuer)
);

create index if not exists oidc_identities_user_idx
    on oidc_identities(user_id);
