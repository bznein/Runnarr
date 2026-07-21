# Development: Run Without Docker

Runnarr can be developed and tested without running the Docker app image.

## Prerequisites

- Go 1.22+
- Node.js 22+
- PostgreSQL 16+ (running locally)

You still need a PostgreSQL database for the app, but you can run the server and UI directly:

1. Configure environment variables in `.env` (copy from `.env.example` first).
   `scripts/dev.sh` now performs this bootstrap automatically when `.env` does not yet exist.
2. Set `RUNNARR_STATIC_DIR=web/dist` (default) for the backend config.
3. Set `RUNNARR_HTTP_ADDR` for the backend (if unset, `scripts/dev.sh` now auto-chooses a random high port and binds it to loopback).

## Start backend + frontend (with Vite hot-reload)

```bash
scripts/dev.sh
```

On first run, the script creates `.env` from `.env.example`, fills missing defaults,
generates a `RUNNARR_SECRET_KEY` when missing, and leaves `RUNNARR_ADMIN_PASSWORD` untouched unless missing.

Then visit:

- Backend API: uses `RUNNARR_HTTP_ADDR` (or the auto-selected random high port when unset).
- Frontend (Vite): whatever is available, e.g. `http://localhost:5173`
  (set `RUNNARR_FRONTEND_PORT` in your environment to pin it).
The backend and Vite server bind to loopback by default. If you still see `/api` calls sent to `5173`, stop the old frontend process on that port and rerun:

```bash
pkill -f "vite --host 127.0.0.1 --port 5173"
```

Set `RUNNARR_KEEP_LEGACY_FRONTEND=1` to keep manually managed legacy frontend instances running.

The frontend uses Vite proxy rules for `/api` and `/healthz`, routed from `scripts/dev.sh` via `VITE_API_TARGET`.

### What the script does

- Validates required runtime variables
- Installs frontend dependencies if missing
- Starts `go run ./cmd/runnarr` and `npm run dev` together
- Keeps both processes on a single command

Process logs are written to `tmp/runnarr-backend.log` and `tmp/runnarr-frontend.log`.

## Quick DB alternative with Docker (optional)

If you want the quickest local database start, run only Postgres in Docker:

```bash
docker compose up -d db
```

`scripts/dev.sh` now auto-starts `docker compose up -d db` when `DATABASE_URL` points at localhost and PostgreSQL is not reachable.
Set `RUNNARR_SKIP_DB_START=1` to prevent this behavior if you want to keep startup fully manual.
`RUNNARR_DB_HOST_PORT` controls which host port Docker publishes for postgres (default `5432`) and is used by both `docker compose up -d db` and `scripts/dev.sh`.
