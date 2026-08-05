# PostGIS database requirement

Course geometry uses PostGIS. Runnarr's bundled database remains on PostgreSQL
16 and now uses `postgis/postgis:16-3.5-alpine`, which keeps the existing
`/var/lib/postgresql/data` volume layout.

## Existing Compose deployments

1. Confirm that no Garmin or training-sheet sync is running.
2. Record the currently deployed application and database images.
3. Take and verify a PostgreSQL backup before starting the new application.
4. Pull/build the release and allow Compose to recreate only the database
   container with the new PostgreSQL 16 PostGIS image, retaining the existing
   named database volume.
5. Start the application. Migration `034_postgis_courses.sql` enables the
   `postgis` extension and creates the empty course schema.
6. Verify `/healthz`, `/api/session`, existing activity data, and the new course
   endpoints before removing the backup.

Do not switch PostgreSQL major versions as part of this change. If startup
reports that the `postgis` extension is unavailable, stop and restore the
previous application/database images; do not delete or recreate the volume.

## Automated preview and staging host

The deployment host uses root-owned Compose assets installed under
`/opt/runnarr-deploy`; it does not check out pull-request files. Before testing
this change in a preview, install the reviewed non-production override while
holding the deployment lock:

```sh
sudo flock /srv/runnarr/deploy.lock \
  install -o root -g root -m 0644 \
  docker-compose.nonprod.yml \
  /opt/runnarr-deploy/docker-compose.nonprod.yml
```

This only updates the configuration used by the next preview or staging
deployment. It does not recreate a running container and it leaves the
production Compose path unchanged. Verify the installed asset without starting
services:

```sh
sudo env RUNNARR_INGRESS_ALIAS=runnarr-config-check \
  docker compose \
  -f /opt/runnarr-deploy/docker-compose.yml \
  -f /opt/runnarr-deploy/docker-compose.nonprod.yml \
  config --format json |
  jq -e '.services.db.image == "postgis/postgis:16-3.5-alpine"'
```

Rerun the candidate workflow after this check passes. Install the reviewed base
Compose asset only as part of the separately backed-up production upgrade
described above.

## External PostgreSQL

The target database must have PostGIS 3.5 or newer installed. The application
migration executes `create extension if not exists postgis`; on managed or
least-privilege databases, a database administrator must enable the extension
before deploying the application migration.

Backups and restores must include the PostGIS extension and course tables. A
plain PostgreSQL server without the extension is no longer a supported Runnarr
database.
