package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"testing"
)

func heatSample(lon, lat float64, elapsed int) ActivitySample {
	return ActivitySample{Index: elapsed, Longitude: &lon, Latitude: &lat, ElapsedS: &elapsed}
}
func decodeHeat(t *testing.T, samples []ActivitySample, summary string) heatGeometry {
	t.Helper()
	var g heatGeometry
	if data := heatmapGeometry(samples, summary); data != nil {
		if err := json.Unmarshal(data, &g); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

func TestHeatmapGeometryGapsAndFallback(t *testing.T) {
	samples := []ActivitySample{heatSample(-6.2, 53.3, 0), heatSample(-6.199, 53.3, 10), {Index: 11}, heatSample(-6.1, 53.3, 20), heatSample(-6.099, 53.3, 30)}
	if got := decodeHeat(t, samples, ""); len(got.Coordinates) != 2 {
		t.Fatalf("GPS gap joined: %#v", got)
	}
	shuffled := []ActivitySample{samples[4], samples[0], samples[2], samples[1], samples[3]}
	if !reflect.DeepEqual(decodeHeat(t, shuffled, ""), decodeHeat(t, samples, "")) || shuffled[0].Index != 30 {
		t.Fatal("sample ordering changed the route or mutated imported samples")
	}
	samples = []ActivitySample{heatSample(0, 0, 0), heatSample(0.001, 0, 10), heatSample(0.02, 0, 200), heatSample(0.021, 0, 210)}
	if got := decodeHeat(t, samples, ""); len(got.Coordinates) != 2 {
		t.Fatalf("long gap joined: %#v", got)
	}
	summary := encodeCoursePolyline([]CoursePoint{{Latitude: 53.3, Longitude: -6.2}, {Latitude: 53.3, Longitude: -6.199}}, 5)
	if got := decodeHeat(t, nil, summary); len(got.Coordinates) != 1 {
		t.Fatal("missing polyline fallback")
	}
	if got := decodeHeat(t, []ActivitySample{heatSample(math.NaN(), 0, 0)}, summary); len(got.Coordinates) != 0 {
		t.Fatal("invalid GPS replaced with summary")
	}
	if got := decodeHeat(t, []ActivitySample{heatSample(0, 86, 0), heatSample(0, 87, 1)}, ""); len(got.Coordinates) != 0 {
		t.Fatal("polar coordinates retained")
	}
}

func TestHeatmapDatelineAndStationary(t *testing.T) {
	g := decodeHeat(t, []ActivitySample{heatSample(179.999, 0, 0), heatSample(-179.999, 0, 1)}, "")
	if len(g.Coordinates) != 2 {
		t.Fatalf("dateline not split: %#v", g)
	}
	for _, line := range g.Coordinates {
		for i := 1; i < len(line); i++ {
			if math.Abs(line[i][0]-line[i-1][0]) > 1000 {
				t.Fatal("line crosses world")
			}
		}
	}
	if got := decodeHeat(t, []ActivitySample{heatSample(0, 0, 0), heatSample(0, 0, 1)}, ""); len(got.Coordinates) != 0 {
		t.Fatal("stationary activity has route")
	}
}

func testHeatLine(r *heatRaster, coordinates ...heatPoint) heatGeometry {
	line := make([]heatPoint, len(coordinates))
	for i, p := range coordinates {
		line[i] = heatPoint{r.origin[0] + p[0]/r.pixelsPerMetre, r.origin[1] - p[1]/r.pixelsPerMetre}
	}
	return heatGeometry{Type: "MultiLineString", Coordinates: [][]heatPoint{line}}
}

func TestHeatmapCountsActivitiesNotSamplesOrLaps(t *testing.T) {
	sparse, dense, lap := newHeatRaster(10, 500, 340, 1), newHeatRaster(10, 500, 340, 1), newHeatRaster(10, 500, 340, 1)
	ctx := context.Background()
	_ = sparse.addActivity(ctx, testHeatLine(sparse, heatPoint{20, 100}, heatPoint{220, 100}))
	points := []heatPoint{}
	for x := 20; x <= 220; x++ {
		points = append(points, heatPoint{float64(x), 100})
	}
	_ = dense.addActivity(ctx, testHeatLine(dense, points...))
	_ = lap.addActivity(ctx, testHeatLine(lap, heatPoint{20, 100}, heatPoint{220, 100}, heatPoint{20, 100}, heatPoint{220, 100}))
	for i, value := range sparse.sum {
		if math.Abs(float64(value-dense.sum[i])) > 0.0001 || value != lap.sum[i] {
			t.Fatalf("sample/lap bias at %d: %f/%f/%f", i, value, dense.sum[i], lap.sum[i])
		}
	}
	before := append([]float32(nil), sparse.sum...)
	_ = sparse.addActivity(ctx, testHeatLine(sparse, heatPoint{20, 100}, heatPoint{220, 100}))
	for i, value := range before {
		if sparse.sum[i] != 2*value {
			t.Fatalf("second activity not counted at %d", i)
		}
	}
}

func TestHeatmapTileSeamsAndCancellation(t *testing.T) {
	a, b := newHeatRaster(10, 500, 340, 1), newHeatRaster(10, 501, 340, 1)
	geometry := testHeatLine(a, heatPoint{250, 20}, heatPoint{265, 220})
	_ = a.addActivity(context.Background(), geometry)
	_ = b.addActivity(context.Background(), geometry)
	for y := 0; y < a.size; y++ {
		for offset := 0; offset < 8; offset++ {
			if a.sum[y*a.size+256+offset] != b.sum[y*b.size+offset] {
				t.Fatalf("seam at %d,%d", offset, y)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a.addActivity(ctx, geometry) == nil {
		t.Fatal("cancelled render succeeded")
	}
}

func TestHeatmapFilters(t *testing.T) {
	for _, query := range []string{"from=2026-02-30", "from=2026-02-01&to=2026-01-01", "timezone=Invalid/Zone", "sport=all&sport=Run"} {
		q, _ := url.ParseQuery(query)
		if _, err := parseHeatmapFilters(q); err == nil {
			t.Fatalf("accepted %q", query)
		}
	}
	q, _ := url.ParseQuery("sport=Run&sport=Cycling&from=2026-03-29&to=2026-03-29&timezone=Europe/Dublin")
	f, err := parseHeatmapFilters(q)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Sports, []string{"Cycling", "Run"}) {
		t.Fatal(f.Sports)
	}
	_, args := f.where("private-user")
	if args[0] != "private-user" || args[2] != "Europe/Dublin" {
		t.Fatal(args)
	}
}

func TestHeatmapCacheEvictsOldestTiles(t *testing.T) {
	cache := newHeatTileService()
	for i := 0; i < heatCacheEntries; i++ {
		cache.put(fmt.Sprint(i), []byte{1})
	}
	if cache.get("0") == nil {
		t.Fatal("missing cached tile")
	}
	cache.put("extra", []byte{2})
	if cache.get("1") != nil || cache.get("0") == nil || len(cache.entries) != heatCacheEntries {
		t.Fatal("cache did not evict the least recently used tile")
	}
	// Large images must respect the byte limit independently of the count limit.
	cache.put("large", make([]byte, heatCacheBytes))
	if cache.bytes > heatCacheBytes || cache.get("extra") != nil {
		t.Fatal("cache byte limit exceeded")
	}
}

func BenchmarkHeatmap10000Routes(b *testing.B) {
	// Ten thousand routes crossing a city tile; unique per-activity masks are
	// required even when the geometry is identical.
	for b.Loop() {
		r := newHeatRaster(13, 3953, 2661, 1)
		g := testHeatLine(r, heatPoint{20, 60}, heatPoint{80, 95}, heatPoint{130, 90}, heatPoint{200, 180})
		for i := 0; i < 10000; i++ {
			_ = r.addActivity(context.Background(), g)
		}
		_ = r.image(true)
	}
}
