# Course routing

Runnarr's course planner routes waypoints and generates closed-loop alternatives
through a self-hosted GraphHopper 11 service. Browser planner coordinates are
sent only to Runnarr; the backend calls GraphHopper. A failed manually planned
leg falls back independently to an inspectable dashed direct line.

Loop generation requires one draft waypoint and accepts 1–100 km for Run,
Walk, and Hike or 5–300 km for Cycling. GraphHopper's native round-trip
algorithm generates a bounded set of alternatives. Runnarr prefers routes
within 10% of the requested distance, permits a closest-result fallback within
20%, rejects excessive retracing and overlap, and returns at most three.

The generator offers Flat, Balanced, and Hilly biases. Flat penalizes sustained
gradients, Balanced uses the normal sport profile, and Hilly favors meaningful
gradients. Candidates show ascent and ascent per kilometre. Hilliness is a
generation hint only: it is not saved on the course, and later waypoint edits
use normal foot or bike routing.

## Bundled optional service

The normal stack does not start GraphHopper because a regional graph can take
minutes to import and several gigabytes of disk and memory. Select the smallest
Geofabrik extract that covers the courses you plan. The example defaults to
Ireland and Northern Ireland.

Set `.env`:

```dotenv
RUNNARR_ROUTING_ENABLED=true
RUNNARR_ROUTING_URL=http://graphhopper:8989
GRAPHHOPPER_PBF_URL=https://download.geofabrik.de/europe/ireland-and-northern-ireland-latest.osm.pbf
GRAPHHOPPER_JAVA_OPTS=-Xms1g -Xmx4g
```

Start the application and optional routing profile:

```sh
docker compose --profile routing up --build -d
docker compose --profile routing logs -f graphhopper
```

The first start downloads the PBF, imports foot and bike landmark profiles, and
downloads SRTM elevation tiles into `graphhopper-data`. Subsequent starts reuse
that volume. Check readiness and available profiles:

```sh
docker compose --profile routing ps
docker compose --profile routing exec graphhopper \
  curl -fsS http://127.0.0.1:8989/info
```

Stop routing while retaining its reusable graph:

```sh
docker compose --profile routing stop graphhopper
docker compose --profile routing rm -f graphhopper
```

To remove the local graph permanently, first run the two commands above, then
inspect and remove only the exact project volume reported by Compose:

```sh
docker compose --profile routing config --volumes
docker volume ls --filter label=com.docker.compose.project --filter name=graphhopper-data
# Replace PROJECT with the verified Compose project name from your host.
docker volume rm PROJECT_graphhopper-data
```

Changing `GRAPHHOPPER_PBF_URL` or the repository routing configuration does not
silently overwrite an existing graph. Startup fails with an actionable message
so an operator can deliberately create a fresh volume. Saved courses remain in
PostgreSQL and are unaffected. Legacy `runnarr-*-valhalla-data` volumes are
also left untouched during migration and may be removed manually after the new
service has been validated.

## Existing GraphHopper service

Set `RUNNARR_ROUTING_URL` to a GraphHopper 11-compatible HTTP(S) origin, enable
routing, and leave the bundled profile off. It must provide `foot` and `bike`
profiles, unencoded GeoJSON route responses, flexible round-trip routing,
custom models using `average_slope`, and elevation data. Run, Walk, and Hike
use `foot`; Cycling uses `bike`.

The configured URL is trusted deployment configuration. Keep external services
on a private network when route privacy matters because they receive waypoint
coordinates. Map tiles remain a separate browser-side boundary controlled by
`MAP_TILE_URL`.

## Managed production and previews

Production uses the pinned GraphHopper 11 image and its own persistent
`runnarr-production-graphhopper-data` volume. Put `GRAPHHOPPER_PBF_URL` and any
resource overrides in the root-owned production `base.env`. Promotion starts
and smoke-tests GraphHopper before cutting the app over. If cutover fails, a
legacy app is restored with routing disabled instead of being pointed at an
incompatible provider.

Automated previews share one host-managed `runnarr-nonprod-graphhopper`
container rather than importing a graph per PR. From a reviewed checkout:

```sh
sudo deploy/configure-preview-routing.sh \
  https://download.geofabrik.de/europe/ireland-and-northern-ireland-latest.osm.pbf
```

For another region, append covered `FROM_LAT FROM_LON TO_LAT TO_LON` smoke
coordinates. The helper backs up installed assets, imports and verifies the
graph, tests 3D foot routing, and activates routing only for future preview
deployments. It does not restart existing previews or production.

## Limitations

- The planner stores route geometry, not turn-by-turn directions.
- Existing courses are never silently recalculated when the graph changes.
- Graph coverage ends at the configured extract boundary; uncovered manual
  legs fall back to direct geometry.
- Native round trips remain best-effort. Sparse networks can return fewer than
  three alternatives or no route inside the 20% distance limit.
