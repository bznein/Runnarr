# Course routing

Runnarr's course planner can connect adjacent waypoints through a self-hosted
Valhalla routing service. The browser sends planner waypoints only to Runnarr;
Runnarr calls Valhalla from the backend. If routing is disabled or a particular
leg cannot be routed, that leg remains visible as a dashed direct line and can
still be saved after confirmation.

## Bundled optional service

The normal Compose stack does not start Valhalla. Routing graphs are regional,
take time and memory to build, and can consume several gigabytes on disk. Pick
the smallest Geofabrik extract that covers the area where you plan courses.
The example environment defaults to the Ireland and Northern Ireland extract.

Set these values in `.env`:

```dotenv
RUNNARR_ROUTING_ENABLED=true
RUNNARR_ROUTING_URL=http://valhalla:8002
VALHALLA_TILE_URL=https://download.geofabrik.de/europe/ireland-and-northern-ireland-latest.osm.pbf
VALHALLA_BUILD_ELEVATION=False
VALHALLA_SERVER_THREADS=2
```

Then explicitly start the routing profile:

```sh
docker compose --profile routing up --build -d
```

The first start downloads the configured OpenStreetMap extract and builds a
graph in the `valhalla-data` volume. Watch progress with:

```sh
docker compose --profile routing logs -f valhalla
```

Changing `VALHALLA_TILE_URL` may trigger a graph rebuild. Back up or remove the
dedicated volume only when you intentionally want to replace its graph; course
records themselves remain in PostgreSQL.

## Automated pull-request previews

Do not start the `routing` profile inside every automated preview. That would
duplicate the regional graph, disk use, and Valhalla process for every open
pull request. The deployment host instead runs one immutable, resource-limited
`runnarr-nonprod-valhalla` container and attaches that trusted container to each
preview's isolated network with the alias `valhalla`.

From the reviewed revision that contains the course planner, enable the shared
service with the smallest regional extract needed for preview testing:

```sh
sudo deploy/configure-preview-routing.sh \
  https://download.geofabrik.de/europe/ireland-and-northern-ireland-latest.osm.pbf
```

The helper holds the deployment lock, backs up the installed non-production
assets, starts the shared service, and activates routing only for subsequent
preview deployments. It does not restart existing preview or production
containers. Watch the first graph build and wait for a healthy result:

The default smoke leg is in central Dublin. For another regional extract, pass
four covered coordinates after the PBF URL (`FROM_LAT FROM_LON TO_LAT TO_LON`)
so deployment acceptance tests that graph rather than Ireland.

```sh
docker compose \
  --project-name runnarr-preview-routing \
  --env-file /etc/runnarr/preview-routing.env \
  --file /opt/runnarr-deploy/docker-compose.routing.yml \
  logs --follow valhalla

docker inspect runnarr-nonprod-valhalla \
  --format '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}'
```

Rerun each candidate deployment after Valhalla is healthy. The deployer writes
`RUNNARR_ROUTING_ENABLED=true` and `RUNNARR_ROUTING_URL=http://valhalla:8002`,
attaches the shared service, and sends a real pedestrian route through the
preview app container before accepting the deployment. Preview teardown
detaches Valhalla before removing the isolated network. Production is not
connected to this service.

## Existing Valhalla service

To use an existing deployment, set `RUNNARR_ROUTING_URL` to its HTTP(S) origin,
enable routing, and leave the bundled profile off. Runnarr calls `/route` with
two coordinates at a time and uses Valhalla's `pedestrian` costing for Run,
Walk, and Hike courses and `bicycle` costing for Cycling.

The URL is trusted deployment configuration. Keep a self-hosted endpoint on a
private network when route privacy matters. An external endpoint receives each
waypoint pair and can infer the planned route. Map tiles remain a separate
browser-side privacy boundary controlled by `MAP_TILE_URL`.

## Limitations

- The planner stores route geometry, not turn-by-turn directions.
- Routed legs currently have no elevation unless their source geometry already
  supplied it. [Issue #245](https://github.com/bznein/Runnarr/issues/245)
  tracks reviewable elevation enrichment and whole-course recalculation.
- Valhalla coverage ends at the boundaries of the configured graph. Uncovered
  or disconnected legs fall back independently instead of discarding the
  course draft.
