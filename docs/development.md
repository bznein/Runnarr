# Development: Run Without Docker

Runnarr can be developed and tested without running the Docker app image.

## Prerequisites

- Go 1.22+
- Node.js 22+
- PostgreSQL 16+ (running locally)

You still need a PostgreSQL database for the app, but you can run the server and UI directly:

1. Configure environment variables in `.env` (copy from `.env.example` first).
2. Set `RUNNARR_STATIC_DIR=web/dist` (default) for the backend config.
3. Set `RUNNARR_HTTP_ADDR=:8080` (or another address for the backend).

## Start backend + frontend (with Vite hot-reload)

```bash
cp .env.example .env
# edit DATABASE_URL/credentials first
source .env
scripts/dev.sh
```

Then visit:

- Backend API: `http://localhost:8080`
- Frontend (Vite): `http://localhost:5173`

The frontend uses Vite proxy rules for `/api` and `/healthz`, so API calls are routed to the local backend.

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

Then run `scripts/dev.sh` in the same shell with the same environment variables.
