-- Migration 028 introduced these objects for the abandoned native Android
-- client. Keep 028 immutable for upgrade history, then remove only its schema.
alter table oidc_auth_states
    drop column if exists client;

alter table google_oauth_states
    drop column if exists client;

drop table if exists mobile_auth_handoffs;
