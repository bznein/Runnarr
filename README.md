# Runnarr

Runnarr is a self-hosted, Dockerized activity hub. It imports local activity files, then presents a private dashboard with activity history, maps, and charts.

The v1 scope is intentionally focused: import, normalize, browse, map, and chart. Deep training analysis, race planning, and multi-user deployments are documented as future roadmap in [docs/PRD.md](docs/PRD.md).

## Quick Start

1. Copy `.env.example` to `.env`.
2. Change `RUNNARR_ADMIN_PASSWORD` and `RUNNARR_SECRET_KEY`.
3. Start the stack:

```sh
docker compose up --build
```

The app listens on `http://localhost:37617` by default.

If that port is already used on your host, change `RUNNARR_PORT` and `RUNNARR_BASE_URL` in `.env`.

## Local Development

Backend:

```sh
go run ./cmd/runnarr
```

Frontend:

```sh
cd web
npm install
npm run dev
```

Set `DATABASE_URL` to a running Postgres instance before starting the backend outside Docker.

## Repository

The intended upstream repository is:

```text
https://github.com/bznein/Runnarr
```
