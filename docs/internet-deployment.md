# Internet-facing deployment

Runnarr supports two deliberately different modes:

- The default local mode keeps password login enabled, binds the Compose host
  ports to loopback, uses HTTP on `localhost`, and does not require Google
  OIDC.
- Public mode requires an HTTPS `RUNNARR_BASE_URL`, Google OIDC, and explicit
  email-to-existing-username mappings. It does not provide public signup.

The reverse proxy is part of the deployment boundary, but the application
still authenticates requests, enforces CSRF, limits request sizes, and scopes
all user data. Do not publish the app or Postgres host ports in public mode.

For the staged rollout, acceptance tests, rollback drills, and ongoing
operational checks, see the [public deployment and testing plan](public-deployment-testing-plan.md).

## Nginx Proxy Manager topology

Create or reuse a Docker network shared by Nginx Proxy Manager and Runnarr:

```sh
docker network create proxy
```

If Docker reports that no default address pool is available, choose an unused
private subnet that does not overlap the host LAN or another Docker network:

```sh
docker network create --driver bridge --subnet 10.254.0.0/24 --gateway 10.254.0.1 proxy
```

Use a different unused subnet when `10.254.0.0/24` is already in use.

Set these values in the deployment `.env` (use long random values):

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

The `RUNNARR_OIDC_ALLOWED_EMAILS` value is a comma-separated list of
`verified-google-email=existing-runnarr-username` mappings. Runnarr will not
create an account for an unmapped Google account. Configure the Google OAuth
client's redirect URI as:

```text
https://runnarr.example.com/api/auth/google/callback
```

Start the public stack with the override file:

```sh
docker compose -f docker-compose.yml -f docker-compose.public.yml up --build -d
```

In Nginx Proxy Manager, create an HTTPS Proxy Host for the DNS name and
forward it to hostname `app`, port `8080`, on the shared `proxy` network.
Enable the proxy's WebSocket support if it is offered, preserve the original
Host header, and pass the usual `X-Real-IP` and `X-Forwarded-Proto` headers.
The app's public cookie is Secure and host-only, so HTTPS must terminate at
the proxy before any user logs in.

The public override removes both the app and Postgres host-port mappings.
Postgres remains reachable only on the private Compose network; the app is
the only service attached to the NPM network.

## Cloudflare configuration and strict CSP

Runnarr intentionally keeps a strict Content-Security-Policy. Its
`script-src` allows only scripts served by Runnarr itself; do not add
`unsafe-inline` to make third-party proxy behavior appear to work.

If Cloudflare proxies the public hostname, disable Cloudflare JavaScript
Detections for that hostname. JavaScript Detections injects an inline
bootstrap response under `/cdn-cgi/challenge-platform/`, which the Runnarr
CSP correctly blocks. A Cloudflare WAF or bot rule that depends on the
JavaScript Detection result must also be disabled or replaced with a control
that does not inject a script into Runnarr HTML responses.

After changing the Cloudflare setting, check the browser console while
visiting `/login`, `/calendar`, the normal SPA routes, and the Google OIDC
callback flow. There should be no CSP violations from Cloudflare, and the
Runnarr bundle must continue to load without any `unsafe-inline` exception.

## Secrets and operations

For Docker secret mounts or another file-backed secret store, set any of the
critical variables with a matching `_FILE` variable, for example
`RUNNARR_SECRET_KEY_FILE` or `RUNNARR_OIDC_GOOGLE_CLIENT_SECRET_FILE`.
File-backed values take precedence over environment values. Never reuse
credentials from a development `.env` in the public deployment; rotate any
Google client secret that has been exposed in a local file or log.

Back up Postgres and `/app/data` together. The data volume contains Garmin
token files and encrypted Google Sheets refresh tokens. Rotate the Runnarr
secret only as a planned migration because it changes the key used for
provider-token encryption. Keep the image updated and review dependency and
container vulnerability reports before upgrades.

Runnarr keeps the default OpenStreetMap tile URL. This is convenient but not
private: the browser sends tile requests, including approximate route
locations, to the tile provider. Use a different `MAP_TILE_URL` if that
metadata exposure is unacceptable.

To run locally, omit the public override and use the normal command:

```sh
docker compose up --build -d
```

It remains available at `http://localhost:37617`; the app and database ports
are bound to loopback, local password login is enabled, and OIDC is optional.
