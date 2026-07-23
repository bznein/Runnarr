# Runnarr

Runnarr is a self-hosted, Dockerized activity hub. It imports activities from Garmin Connect and local activity files, then presents a private dashboard with activity history, maps, and charts. Multiple local accounts can use one deployment while keeping activity, health, provider, gear, and planning data private to each account.

The v1 scope is intentionally focused: import, normalize, browse, map, and chart. Deep training analysis and race planning remain future scope; local account management is included for trusted household deployments.

## Quick Start

1. Copy `.env.example` to `.env`.
2. Change `RUNNARR_ADMIN_USERNAME`, `RUNNARR_ADMIN_PASSWORD`, and `RUNNARR_SECRET_KEY`.
3. Start the stack:

```sh
docker compose up --build
```

The app listens on `http://localhost:37617` by default.

The configured admin account is created automatically on first startup. Additional accounts are created from Settings by an administrator. Administrators can temporarily enter a read-only support view for another account; account data remains private and provider credentials are stored per user.

## Mobile web and PWA

The responsive web client is the mobile client. It can be installed as a PWA
from a supported HTTPS deployment or localhost. The service worker caches only
the application shell and static assets; authenticated API responses, activity
media, maps, and provider data remain network-only.

The Google Pixel 8 Pro in Chrome is the primary mobile acceptance profile, but
the layout adapts to smaller phones, tablets, and desktop browsers. See the
[web and PWA smoke-test checklist](docs/mobile-pwa-smoke-test.md) for browser,
installability, responsive-layout, and cache checks.

If that port is already used on your host, change `RUNNARR_PORT` and `RUNNARR_BASE_URL` in `.env`.

For an HTTPS deployment behind Nginx Proxy Manager, see
[docs/internet-deployment.md](docs/internet-deployment.md). Public mode is an
explicit Compose override and does not change the local `localhost` path.

## Local Development

Backend:

```sh
source .env
go run ./cmd/runnarr
```

The example environment binds a directly-run backend to loopback. Docker
overrides the container listen address internally, while its host port remains
loopback-only.

Frontend:

```sh
cd web
npm install
npm run dev
```

For local non-docker full-stack development with Vite hot-reload, use:

```sh
scripts/dev.sh
```

`scripts/dev.sh` will create `.env` from `.env.example` on first run, generate a `RUNNARR_SECRET_KEY` if missing,
and run backend+frontend. `RUNNARR_ADMIN_PASSWORD` is preserved unless missing.
If `RUNNARR_HTTP_ADDR` is unset or `:8080`, it will be replaced with a random high port.
`RUNNARR_FRONTEND_PORT` (default `5173`) sets the preferred Vite port.
Point `DATABASE_URL` in `.env` at your local PostgreSQL before running if you want a
non-default database URL.
When using `docker compose up -d db` for local DB, keep `RUNNARR_DB_HOST_PORT` in `.env` aligned to the host-mapped postgres port (default `5432`).

See [docs/development.md](docs/development.md) for full non-dockerized setup notes.

Set `DATABASE_URL` to a running Postgres instance before starting the backend outside Docker.

## Garmin Connect Setup

Garmin Connect sync is configured from Settings after login. Enter your Garmin email/password, and enter an MFA code if Garmin asks for one. Runnarr stores Garmin Connect tokens in the Docker `app-data` volume and does not store your Garmin password.

The Garmin integration uses an unofficial Garmin Connect client because Garmin's official Activity API requires approval. If Garmin changes their private endpoints, reconnecting or updating the image dependency may be required.

## Repository

The intended upstream repository is:

```text
https://github.com/bznein/Runnarr
```
