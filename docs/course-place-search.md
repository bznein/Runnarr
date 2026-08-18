# Course place search

Runnarr can search for a town, landmark, or address while planning a course.
The browser sends a query to Runnarr only after the user presses **Search**.
Runnarr then forwards that text to the configured Nominatim-compatible service,
returns at most five results, and lets the user inspect a result on the map or
add it as a named waypoint.

Place search is disabled by default. Enable it in `.env`:

```dotenv
RUNNARR_GEOCODING_ENABLED=true
RUNNARR_GEOCODING_URL=https://nominatim.openstreetmap.org
```

Then apply the configuration during an operator-approved deployment or restart.
`RUNNARR_GEOCODING_URL` must be an HTTP(S) origin without a path query or
fragment. A standard Nominatim `/search` endpoint is required.

## Privacy and provider policy

Runnarr proxies searches so the browser does not contact the geocoder directly.
The configured service still receives the submitted search text and the
Runnarr server's network address. It does not receive course geometry,
waypoints, account identifiers, or browser location from this feature.

For route privacy and independence from public availability, operate a private
Nominatim instance and set its internal origin, for example:

```dotenv
RUNNARR_GEOCODING_ENABLED=true
RUNNARR_GEOCODING_URL=http://nominatim:8080
```

The default URL points to the public OpenStreetMap Nominatim service only as an
explicit opt-in convenience. Review and follow its current
[usage policy](https://operations.osmfoundation.org/policies/nominatim/) before
enabling it, and do not submit personal or confidential information. Runnarr
uses explicit searches rather than autocomplete, identifies its requests,
limits each response to five results, keeps up to 256 successful query results
in process memory for 24 hours, and spaces provider requests to no more than one
per second per Runnarr process. The cache is not persisted and stores only a
hash of the submitted query as its key. A public service may still throttle or
refuse requests; it is not an availability dependency when search remains
disabled.

Map tiles are configured separately with `MAP_TILE_URL`. Selecting a result can
therefore involve two different privacy boundaries: the backend geocoder sees
the submitted text, and the browser-side tile provider sees the viewed map area.
