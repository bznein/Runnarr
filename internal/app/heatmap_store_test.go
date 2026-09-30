package app

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Opt-in integration test. Uses its own disposable database, never existing rows.
func TestHeatmapStoreIntegration(t *testing.T) {
	database := os.Getenv("RUNNARR_HEATMAP_TEST_DATABASE_URL")
	if database == "" {
		t.Skip("set RUNNARR_HEATMAP_TEST_DATABASE_URL to an isolated PostGIS database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("heatmap_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "create database "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "drop database "+schema)
	config, err := pgxpool.ParseConfig(database)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.Database = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool)
	if err = store.EnsureBootstrap(ctx, "bootstrap", "test"); err != nil {
		t.Fatal(err)
	}
	var user, other string
	for i, target := range []*string{&user, &other} {
		if err = pool.QueryRow(ctx, `insert into users(username,display_name,role,password_hash) values($1,$1,'user','test') returning id::text`, fmt.Sprintf("heatmap-%d", i)).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	userCtx := withUserID(ctx, user)
	otherCtx := withUserID(ctx, other)
	activity := ImportedActivity{Name: "Heatmap run", SportType: "Run", StartTime: time.Date(2026, 9, 29, 23, 30, 0, 0, time.UTC), DistanceM: 1000, Samples: []ActivitySample{heatSample(-6.26, 53.35, 0), heatSample(-6.25, 53.35, 60)}}
	id, err := store.SaveImportedActivity(userCtx, "test", "one", nil, activity)
	if err != nil {
		t.Fatal(err)
	}
	otherActivity := activity
	otherActivity.Samples = []ActivitySample{heatSample(0, 50, 0), heatSample(0.01, 50, 60)}
	if _, err = store.SaveImportedActivity(otherCtx, "test", "one", nil, otherActivity); err != nil {
		t.Fatal(err)
	}
	f := heatmapFilters{Sports: []string{"running"}, Timezone: "Europe/Dublin", From: "2026-09-30", To: "2026-09-30"}
	first, err := store.heatmapMetadata(userCtx, f)
	if err != nil {
		t.Fatal(err)
	}
	if first.Mapped != 1 || first.Pending != 0 || first.Bounds == nil || first.DistanceM != 1000 {
		t.Fatalf("metadata: %+v", first)
	}
	for i, want := range [4]float64{-6.26, 53.35, -6.25, 53.35} {
		if math.Abs(first.Bounds[i]-want) > 0.00001 {
			t.Fatalf("bounds: got %v, coordinate %d want %f", first.Bounds, i, want)
		}
	}
	utc := f
	utc.Timezone = "UTC"
	empty, err := store.heatmapMetadata(userCtx, utc)
	if err != nil || empty.Mapped != 0 {
		t.Fatalf("timezone: %+v %v", empty, err)
	}
	tx, err := pool.BeginTx(userCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	point := heatProject(-6.255, 53.35)
	tileX := int((point[0]/heatmapWorld + 0.5) * 8192)
	tileY := int((0.5 - point[1]/heatmapWorld) * 8192)
	raster, err := renderHeatmapRows(userCtx, tx, f, user, 13, tileX, tileY, 1)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	peak := float32(0)
	for _, v := range raster.sum {
		peak = max(peak, v)
	}
	if peak < 0.9 || peak > 1 {
		t.Fatalf("missing route or cross-account contribution: peak %f", peak)
	}
	server := &Server{store: store, logger: slog.Default(), heatTiles: newHeatTileService()}
	routes := server.Routes()
	userSession, err := store.CreateSession(ctx, user, "test-csrf", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := store.CreateSession(ctx, other, "test-csrf", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	getTile := func(session string, revision int64, scale int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest("GET", fmt.Sprintf("/api/heatmap/tiles/13/%d/%d.png?appearance=dark&revision=%d&scale=%d", tileX, tileY, revision, scale), nil)
		if session != "" {
			request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
		}
		response := httptest.NewRecorder()
		routes.ServeHTTP(response, request)
		return response
	}
	if response := getTile("", first.Revision, 1); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous tile: %d", response.Code)
	}
	firstTile := getTile(userSession, first.Revision, 1)
	if firstTile.Code != 200 {
		t.Fatalf("tile: %s", firstTile.Body.String())
	}
	warmTile := getTile(userSession, first.Revision, 1)
	if warmTile.Code != 200 || !bytes.Equal(firstTile.Body.Bytes(), warmTile.Body.Bytes()) {
		t.Fatal("cached tile changed")
	}
	privateTile := getTile(otherSession, first.Revision, 1)
	if privateTile.Code != 200 || bytes.Equal(firstTile.Body.Bytes(), privateTile.Body.Bytes()) {
		t.Fatal("cross-account tile cache collision")
	}
	for _, scale := range []int{1, 2} {
		response := getTile(userSession, first.Revision, scale)
		decoded, e := png.Decode(bytes.NewReader(response.Body.Bytes()))
		if e != nil || decoded.Bounds().Dx() != 256*scale || decoded.Bounds().Dy() != 256*scale {
			t.Fatalf("PNG scale %d: %v", scale, e)
		}
		if response.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("private tile permits browser caching")
		}
	}
	// Reimport replaces, never duplicates, and changes the revision.
	if _, err = store.SaveImportedActivity(userCtx, "test", "one", nil, activity); err != nil {
		t.Fatal(err)
	}
	updated, err := store.heatmapMetadata(userCtx, f)
	if err != nil || updated.Mapped != 1 || updated.Revision <= first.Revision {
		t.Fatalf("reimport: %+v %v", updated, err)
	}
	if response := getTile(userSession, first.Revision, 1); response.Code != http.StatusConflict {
		t.Fatalf("stale cached tile returned: %d", response.Code)
	}
	// Deleting the derived row simulates a pre-feature activity.
	if _, err = pool.Exec(ctx, `delete from activity_heatmap_routes where activity_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	pending, err := store.heatmapMetadata(userCtx, f)
	if err != nil || pending.Pending != 1 {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := store.backfillHeatmapRoute(ctx); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	ready, err := store.heatmapMetadata(userCtx, f)
	if err != nil || ready.Mapped != 1 || ready.Pending != 0 {
		t.Fatalf("backfill: %+v %v", ready, err)
	}
	if _, err = store.DeleteActivity(userCtx, id); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.heatmapMetadata(userCtx, f)
	if err != nil || deleted.Mapped != 0 || deleted.Revision <= ready.Revision {
		t.Fatalf("deletion: %+v %v", deleted, err)
	}
	untouched, err := store.heatmapMetadata(otherCtx, f)
	if err != nil || untouched.Mapped != 1 {
		t.Fatalf("other account affected: %+v %v", untouched, err)
	}
}
