# Public deployment and testing plan

This runbook is for exposing a private Runnarr instance through HTTPS and a
reverse proxy. A DNS name is discoverable even if it is not advertised, so
the deployment must be treated as internet-facing. The reverse proxy and
firewall are important controls, but they are not the application’s only
security boundary.

The rollout is complete only when all of the following are true:

- Users can reach Runnarr through the HTTPS DNS name, and HTTP is redirected
  or rejected by the proxy.
- The application and PostgreSQL ports are not reachable from the internet.
- Only explicitly mapped Google identities can sign in to the public service.
- Mutating requests require the expected session, origin, and CSRF protections.
- A backup has been restored successfully in an isolated environment.
- The normal local-only Compose workflow still works without public-mode
  settings.

## 1. Preflight

Record the deployment commit or image digest, DNS name, proxy host, database
backup location, and an owner for the maintenance window. Do not deploy while
a Garmin sync is running; first confirm that no job is in progress:

```sh
docker compose exec db psql -U runnarr -d runnarr -tAc "select count(*) from sync_jobs where status = 'running';"
```

The expected result is `0`. Take a fresh PostgreSQL backup and a consistent
copy of `/app/data`, then verify that both can be read. The backup must include
Garmin token files and encrypted Google Sheets refresh tokens; restoring only
the database is not sufficient.

Confirm the host and network boundary before starting:

- DNS points to the reverse proxy’s public address.
- The firewall exposes only the proxy’s required 80/443 endpoints. SSH and
  administrative interfaces are restricted to the management network.
- No router, firewall, or cloud security group forwards ports `37617` or
  `5432` to the host.
- The Docker network shared with the proxy exists, and the proxy container is
  attached to it. Attach only the Runnarr app to that shared network; keep
  PostgreSQL on the private application network.
- Nginx Proxy Manager or the chosen proxy is patched and has its own admin
  interface protected separately.

Prepare deployment secrets outside the repository. Use long random values,
file-backed secrets with restrictive permissions where possible, and a
non-default bcrypt hash for the admin password. URL-encode special characters
in the database password when constructing `DATABASE_URL`. Never copy values
from a development `.env` into the public deployment; rotate any credential
that has appeared in a repository, log, terminal capture, or support bundle.

## 2. Google and Runnarr configuration

Create or select a Google OAuth web client dedicated to this deployment. Set
the exact HTTPS redirect URI; do not use a wildcard:

```text
https://runnarr.example.com/api/auth/google/callback
```

Check the Google consent-screen state and test-user restrictions. Use the
minimum requested identity scopes (`openid`, `email`, and `profile`) and
do not assume that an unadvertised DNS name provides access control.

Before enabling public mode, create the intended Runnarr users and map only
their verified Google email addresses to those existing usernames. Test at
least one allowed account and one Google account that is not in the mapping.
Do not map an address to an account that should not be able to administer the
instance.

The public environment should contain the equivalent of:

```dotenv
RUNNARR_BASE_URL=https://runnarr.example.com
RUNNARR_PUBLIC_MODE=true
RUNNARR_LOCAL_AUTH_ENABLED=false
RUNNARR_PROXY_NETWORK=proxy
RUNNARR_TRUST_PROXY=true
RUNNARR_OIDC_GOOGLE_CLIENT_ID=...
RUNNARR_OIDC_GOOGLE_CLIENT_SECRET=...
RUNNARR_OIDC_ALLOWED_EMAILS=you@example.com=admin
RUNNARR_ADMIN_PASSWORD_HASH=...
RUNNARR_SECRET_KEY=...
POSTGRES_PASSWORD=...
DATABASE_URL=postgres://runnarr:<same-password>@db:5432/runnarr?sslmode=disable
```

Use `_FILE` variants for secrets when the deployment’s secret manager
supports them. Keep the existing Google Sheets OAuth settings separate from
the Google OIDC settings; their callback paths and credentials are different.

## 3. Staged deployment

Run the following from the checked-out release commit:

```sh
docker compose -f docker-compose.yml -f docker-compose.public.yml config --quiet
docker compose -f docker-compose.yml -f docker-compose.public.yml up --build -d
docker compose -f docker-compose.yml -f docker-compose.public.yml ps
docker compose -f docker-compose.yml -f docker-compose.public.yml logs --tail=200 app
```

Before putting traffic through the proxy, inspect the rendered Compose
configuration. The public app must have no host `ports` entry, PostgreSQL must
have no host `ports` entry, and the app must be attached to both its private
network and the external proxy network. The proxy must be able to resolve
`app` on port `8080`.

Confirm that startup migrations complete and that the app logs do not contain
missing-secret, default-secret, invalid-public-URL, or invalid-OIDC warnings.
If a public-mode preflight check fails, fix the configuration before routing
traffic; do not bypass it by enabling local login on the public hostname.

Configure the reverse proxy with:

- the exact DNS hostname;
- a valid certificate and automatic renewal;
- upstream hostname `app` and port `8080` on the shared Docker network;
- preservation of the original `Host` header;
- correct `X-Real-IP` and `X-Forwarded-Proto` handling;
- request and upload timeouts/limits compatible with the intended import and
  media workflows;
- no proxy access rule that blocks the Google callback.

Start with a narrow allowlist at the proxy if one is practical, then remove it
only after the application-level OIDC tests below pass. Do not use a proxy
allowlist as a replacement for Runnarr’s identity mapping.

## 4. Public acceptance tests

Run these from a machine outside the Docker host where possible. Substitute
the real hostname for `runnarr.example.com`.

### Transport and exposure

- `curl -I https://runnarr.example.com/healthz` returns the expected health
  response over HTTPS and includes the security headers. Verify HSTS is
  present only on HTTPS.
- An HTTP request is redirected to HTTPS or rejected by the proxy.
- `/api/session` is not cached and reports public mode with local login
  disabled.
- From an external network, direct connections to the host’s old app and
  PostgreSQL ports fail. From the proxy network, the proxy can reach
  `app:8080`.
- The certificate name, renewal process, DNS record, and proxy error pages are
  all correct.

### Authentication and sessions

- An unauthenticated request can reach only the intended health, session, and
  authentication endpoints; protected API routes return `401`.
- `POST /api/session/login` is unavailable in public mode when local auth is
  disabled.
- The mapped Google account completes the callback and receives a session.
  Inspect the browser cookie: it must be host-only, `Secure`, `HttpOnly`, and
  use the expected `SameSite` policy.
- Logout invalidates the session. Reusing an old session cookie after logout,
  password reset, or user disablement does not restore access.
- A Google account absent from `RUNNARR_OIDC_ALLOWED_EMAILS` is denied and no
  Runnarr account is silently created. Also test a mapping to a disabled or
  missing local user.
- OIDC state and nonce values cannot be replayed; a callback with a reused,
  altered, or expired state is rejected.
- Google Sheets connection still uses its separate authenticated flow and
  callback; a Sheets callback cannot be used to create an application login.

### Request integrity and authorization

- A mutating request without a valid session and CSRF token is rejected.
- A mutating request with a valid session but a foreign `Origin` or
  `Referer` is rejected. A normal same-origin browser mutation succeeds.
- With two test users, replace IDs in activity, health, media, gear, planned
  training, provider, export, and sync requests. Every cross-user read and
  write must be denied, including access through download and delete routes.
- Verify admin support tools are read-only and cannot be used to mutate or
  impersonate a user.
- Repeated failed logins trigger the rate limit and return to normal only
  after the expected window. Confirm that a missing-user login is not
  noticeably cheaper than an existing-user login.

### Resource and input limits

- Oversized JSON, import, media, image, GPX, Garmin, and planned-training
  requests are rejected without an unbounded memory or disk increase.
- Malformed JSON, invalid image dimensions, path traversal attempts, and
  unexpected content types fail cleanly with no 5xx response.
- Concurrent sync-start requests create at most one active job for the same
  user/provider. A failed or interrupted job can be retried without leaving
  an unbounded queue.
- Test normal activity import, media upload/download, Garmin sync, calendar,
  maps, and provider connection flows at realistic sizes. Confirm that the
  OSM tile privacy notice is visible and that a blocked tile provider does not
  expose sensitive data or break the rest of the app.

### Browser and usability

Use a clean browser profile and test the complete user journey: load the
login page, sign in, refresh, navigate to each data area, upload/import a
small fixture, export data, connect/disconnect providers, and log out. Repeat
the smoke test from a second device or browser. Check that browser console
errors, mixed-content warnings, CSP violations, and proxy 4xx/5xx responses
are investigated rather than ignored.

## 5. Local-mode regression test

The public override must never be required for a local installation. In a
separate local checkout or after stopping the public stack, use only the base
Compose file:

```sh
docker compose up --build -d
curl -fsS http://127.0.0.1:37617/api/session
```

The response must report `publicMode: false` and `localLoginEnabled: true`.
Password login, logout, imports, media, provider connections, and normal UI
navigation must work over `http://localhost:37617`. Confirm that the app and
PostgreSQL ports bind only to loopback and that no public DNS/proxy setting is
needed. Stop the local stack before switching back to the public override.

If a break-glass local login is ever enabled for a public deployment, restrict
access at the network layer first, use it only from a trusted management
network, and disable it followed by an app restart as soon as the recovery is
complete. Verify that previously issued sessions were revoked as intended.

## 6. Failure, backup, and rollback drills

Perform these in staging before the first production exposure and repeat the
most important drills after major upgrades:

1. Restore the PostgreSQL backup and `/app/data` into an isolated Compose
   project. Start the restored version, run the local smoke test, and verify
   activities, media, Garmin tokens, and encrypted provider credentials.
2. Stop and restart the app while no sync is running. Verify migrations,
   session behavior, and that no data was lost. Exercise an interrupted sync
   and confirm the job is reconciled or safely retryable.
3. Make the proxy unavailable, restart the app, and make PostgreSQL
   temporarily unavailable. Confirm the proxy exposes only a controlled error,
   the app recovers after dependencies return, and credentials are not leaked
   in logs.
4. Deploy a previously built image using the documented Compose command and
   verify that the application and database schema are compatible. Keep a
   database backup before migrations; do not assume every schema migration is
   automatically reversible.
5. Test secret rotation as a planned operation. In particular, changing
   `RUNNARR_SECRET_KEY` can make existing encrypted provider tokens
   unreadable; follow a migration procedure and re-authorize providers if
   required.

Record the exact rollback command and the image digest in the change ticket.
If rollback crosses a schema migration, restore the matching database backup
instead of downgrading blindly.

## 7. Ongoing operations

- Daily: check proxy certificate renewal, external HTTPS health, container
  health, recent app errors, disk usage, and backup completion.
- Weekly: review authentication failures, OIDC denials, rate-limit events,
  unexpected 4xx/5xx spikes, sync failures, and new users or allowlist
  changes. Confirm that no service has acquired an unintended host port.
- Monthly: perform a restore sample, review firewall and proxy rules, patch
  the host/proxy/database images, rebuild Runnarr, and rerun the public
  acceptance and local regression tests.
- On every release: record the commit/image digest, review migrations and
  dependency/container vulnerability reports, back up before deployment, and
  preserve a known-good image for rollback.
- Rotate OAuth credentials, database credentials, admin credentials, and
  proxy credentials on a defined schedule or immediately after exposure.
  Review the email allowlist whenever a user is added, removed, or disabled.

Keep backups encrypted, access-controlled, and subject to a retention policy.
Keep logs free of OAuth codes, cookies, tokens, passwords, and raw health
payloads. Treat a DNS, certificate, proxy, or secret-management change as a
security change and rerun the transport and authentication tests.

## 8. Sign-off record

Before declaring the service live, record:

```text
Hostname:
Deployment date/time:
Git commit or image digest:
Proxy and certificate owner:
Database/data backup identifier:
Backup restore verified by/date:
External exposure test verified by/date:
Allowed Google identities reviewed by/date:
Public acceptance tests: pass / fail
Local-mode regression: pass / fail
Rollback drill: pass / fail
Open risks and follow-up owners:
```

Do not mark the deployment complete while a failed acceptance test, unverified
restore, directly exposed service port, default secret, or unreviewed
allowlist entry remains unresolved.
