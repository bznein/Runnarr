package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

func saveHeatmapRouteTx(ctx context.Context, tx pgx.Tx, id, userID string, samples []ActivitySample, summary string) error {
	geometry := heatmapGeometry(samples, summary)
	var geoJSON any
	if geometry != nil {
		geoJSON = string(geometry)
	}
	_, err := tx.Exec(ctx, `
        insert into activity_heatmap_routes(activity_id,user_id,version,geometry,medium_geometry,coarse_geometry)
        select $1,$2,$3,g,st_multi(st_simplifypreservetopology(g,16)),st_multi(st_simplifypreservetopology(g,256))
        from (select case when $4::text is null then null else st_setsrid(st_geomfromgeojson($4),3857) end as g) source
        on conflict(activity_id) do update set version=excluded.version,geometry=excluded.geometry,
            medium_geometry=excluded.medium_geometry,coarse_geometry=excluded.coarse_geometry,indexed_at=now()
    `, id, userID, heatmapIndexVersion, geoJSON)
	return err
}

// Each row is locked before reading its samples. Reimports and deletes therefore
// cannot publish old geometry after the activity has changed.
func (s *Store) backfillHeatmapRoute(ctx context.Context) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id, userID, summary string
	err = tx.QueryRow(ctx, `select a.id::text,a.user_id::text,a.summary_polyline from activities a
        left join activity_heatmap_routes h on h.activity_id=a.id
        where a.source <> 'training_sheet' and (h.version is null or h.version <> $1)
        order by a.id for update of a skip locked limit 1`, heatmapIndexVersion).Scan(&id, &userID, &summary)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx, `select sample_index,timestamp,elapsed_s,latitude,longitude from activity_samples where activity_id=$1 order by sample_index`, id)
	if err != nil {
		return false, err
	}
	samples := []ActivitySample{}
	for rows.Next() {
		var sample ActivitySample
		if err = rows.Scan(&sample.Index, &sample.Timestamp, &sample.ElapsedS, &sample.Latitude, &sample.Longitude); err != nil {
			rows.Close()
			return false, err
		}
		samples = append(samples, sample)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return false, err
	}
	if err = saveHeatmapRouteTx(ctx, tx, id, userID, samples, summary); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (s *Server) runHeatmapBackfill(ctx context.Context) {
	for ctx.Err() == nil {
		for i := 0; i < 25; i++ {
			more, err := s.store.backfillHeatmapRoute(ctx)
			if err != nil && ctx.Err() == nil {
				s.logger.Error("prepare heatmap route", "error", err)
			}
			if err != nil || !more {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

type heatmapFilters struct {
	Sports             []string
	From, To, Timezone string
}

func (f heatmapFilters) where(user string) (string, []any) {
	where := `a.user_id=$1 and a.source <> 'training_sheet'`
	args := []any{user}
	add := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	if len(f.Sports) == 1 && f.Sports[0] == "running" {
		where += ` and a.sport_type in ('Run','Treadmill Run')`
	} else if !(len(f.Sports) == 1 && f.Sports[0] == "all") {
		where += ` and a.sport_type=any(` + add(f.Sports) + `::text[])`
	}
	if f.From != "" || f.To != "" {
		tz := add(f.Timezone)
		if f.From != "" {
			where += ` and date(a.start_time at time zone ` + tz + `) >= ` + add(f.From) + `::date`
		}
		if f.To != "" {
			where += ` and date(a.start_time at time zone ` + tz + `) <= ` + add(f.To) + `::date`
		}
	}
	return where, args
}

type heatmapMetadata struct {
	Revision  int64       `json:"revision"`
	Sports    []string    `json:"sports"`
	Mapped    int         `json:"mapped"`
	Excluded  int         `json:"excluded"`
	Pending   int         `json:"pending"`
	DistanceM float64     `json:"distanceM"`
	Bounds    *[4]float64 `json:"bounds,omitempty"` // west,south,east,north
}

func heatmapRevision(ctx context.Context, tx pgx.Tx, user string) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `select coalesce((select revision from heatmap_revisions where user_id=$1),0)`, user).Scan(&revision)
	return revision, err
}

func (s *Store) heatmapMetadata(ctx context.Context, filters heatmapFilters) (heatmapMetadata, error) {
	result := heatmapMetadata{Sports: []string{}}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	user := scopedUserID(ctx)
	result.Revision, err = heatmapRevision(ctx, tx, user)
	if err != nil {
		return result, err
	}
	where, args := filters.where(user)
	var west, south, east, north *float64
	err = tx.QueryRow(ctx, `select
        count(*) filter(where h.version=1 and h.geometry is not null),
        count(*) filter(where h.version=1 and h.geometry is null),
        count(*) filter(where h.version is null or h.version<>1),
        coalesce(sum(a.distance_m) filter(where h.version=1 and h.geometry is not null),0),
		st_xmin(st_extent(h.geometry)),st_ymin(st_extent(h.geometry)),
		st_xmax(st_extent(h.geometry)),st_ymax(st_extent(h.geometry))
        from activities a left join activity_heatmap_routes h on h.activity_id=a.id where `+where, args...).Scan(&result.Mapped, &result.Excluded, &result.Pending, &result.DistanceM, &west, &south, &east, &north)
	if err != nil {
		return result, err
	}
	if west != nil && south != nil && east != nil && north != nil {
		// Aggregate the stored bounding boxes before projecting, rather than
		// transforming every coordinate in a full activity history.
		latitude := func(y float64) float64 { return math.Atan(math.Sinh(y/heatmapWorld*2*math.Pi)) * 180 / math.Pi }
		result.Bounds = &[4]float64{*west / heatmapWorld * 360, latitude(*south), *east / heatmapWorld * 360, latitude(*north)}
	}
	rows, err := tx.Query(ctx, `select distinct sport_type from activities where user_id=$1 and source<>'training_sheet' order by sport_type`, user)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var sport string
		if err = rows.Scan(&sport); err != nil {
			rows.Close()
			return result, err
		}
		result.Sports = append(result.Sports, sport)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func renderHeatmapRows(ctx context.Context, tx pgx.Tx, filters heatmapFilters, user string, z, x, y, scale int) (*heatRaster, error) {
	raster := newHeatRaster(z, x, y, scale)
	west, north := raster.origin[0], raster.origin[1]
	span := float64(raster.size) / raster.pixelsPerMetre
	where, args := filters.where(user)
	start := len(args) + 1
	args = append(args, west, north-span, west+span, north)
	envelope := fmt.Sprintf("st_makeenvelope($%d,$%d,$%d,$%d,3857)", start, start+1, start+2, start+3)
	column := "h.geometry"
	// Precomputed levels keep full-history/world views off the raw samples.
	metresPerPixel := 1 / raster.pixelsPerMetre
	if metresPerPixel >= 512 {
		column = "h.coarse_geometry"
	} else if metresPerPixel >= 32 {
		column = "h.medium_geometry"
	}
	shifts := []int{0}
	if west < -heatmapWorld/2 {
		shifts = append(shifts, 1)
	}
	if west+span > heatmapWorld/2 {
		shifts = append(shifts, -1)
	}
	args = append(args, shifts, heatmapWorld, metresPerPixel*0.25)
	shiftParam, worldParam, toleranceParam := len(args)-2, len(args)-1, len(args)
	// Collect translated dateline pieces by activity before drawing its mask.
	query := fmt.Sprintf(`with bounds as (select %s as box), pieces as (
        select a.id,st_translate(st_intersection(st_simplifypreservetopology(%s,$%d),st_translate(bounds.box,shift*$%d,0)),-shift*$%d,0) as g
        from bounds cross join unnest($%d::int[]) shift
        join activity_heatmap_routes h on h.geometry && st_translate(bounds.box,shift*$%d,0)
        join activities a on a.id=h.activity_id where %s and h.version=1)
        select st_asgeojson(st_multi(st_collectionextract(st_collect(g),2)),6) from pieces group by id`, envelope, column, toleranceParam, worldParam, worldParam, shiftParam, worldParam, where)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var encoded string
		if err = rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var geometry heatGeometry
		if err = json.Unmarshal([]byte(encoded), &geometry); err != nil {
			return nil, err
		}
		if err = raster.addActivity(ctx, geometry); err != nil {
			return nil, err
		}
	}
	return raster, rows.Err()
}
