package app

import (
	"encoding/json"
	"math"
	"sort"
)

const heatmapIndexVersion = 1
const heatmapWorld = 2 * math.Pi * 6378137
const heatmapMaxLatitude = 85.0511287798066

type heatPoint [2]float64
type heatGeometry struct {
	Type        string        `json:"type"`
	Coordinates [][]heatPoint `json:"coordinates"`
}

func heatProject(lon, lat float64) heatPoint {
	return heatPoint{lon / 360 * heatmapWorld, math.Log(math.Tan(math.Pi/4+lat*math.Pi/360)) * heatmapWorld / (2 * math.Pi)}
}

// Missing samples break lines rather than inventing a route across a GPS gap.
// All coordinates are derived; provider payloads and original samples stay intact.
func heatmapGeometry(samples []ActivitySample, summary string) []byte {
	if !sort.SliceIsSorted(samples, func(i, j int) bool { return samples[i].Index < samples[j].Index }) {
		samples = append([]ActivitySample(nil), samples...)
		sort.SliceStable(samples, func(i, j int) bool { return samples[i].Index < samples[j].Index })
	}
	hasGPS := false
	for _, sample := range samples {
		if sample.Latitude != nil || sample.Longitude != nil {
			hasGPS = true
			break
		}
	}
	if !hasGPS && summary != "" {
		if points, err := decodeCoursePolyline(summary, 5); err == nil {
			samples = make([]ActivitySample, len(points))
			for i, point := range points {
				lat, lon := point.Latitude, point.Longitude
				samples[i] = ActivitySample{Latitude: &lat, Longitude: &lon}
			}
		}
	}
	geometry := heatGeometry{Type: "MultiLineString"}
	var line []heatPoint
	var previous *ActivitySample
	flush := func() {
		if len(line) > 1 {
			geometry.Coordinates = append(geometry.Coordinates, line)
		}
		line = nil
	}
	for i := range samples {
		sample := &samples[i]
		if sample.Latitude == nil || sample.Longitude == nil || !heatFinite(*sample.Latitude) || !heatFinite(*sample.Longitude) || math.Abs(*sample.Latitude) > heatmapMaxLatitude || math.Abs(*sample.Longitude) > 180 {
			flush()
			previous = nil
			continue
		}
		lon, lat := *sample.Longitude, *sample.Latitude
		if previous != nil {
			priorLon, priorLat := *previous.Longitude, *previous.Latitude
			distance := haversine(priorLat, priorLon, lat, lon)
			seconds, timed := 0.0, false
			if sample.Timestamp != nil && previous.Timestamp != nil {
				seconds, timed = sample.Timestamp.Sub(*previous.Timestamp).Seconds(), true
			} else if sample.ElapsedS != nil && previous.ElapsedS != nil {
				seconds, timed = float64(*sample.ElapsedS-*previous.ElapsedS), true
			}
			if (timed && ((seconds > 120 && distance > 200) || seconds < 0)) || (!timed && distance > 2000) {
				flush()
			} else if math.Abs(lon-priorLon) > 180 {
				// Interpolate at the dateline, then continue on the other edge.
				unwrapped, edge := lon, 180.0
				if lon < priorLon {
					unwrapped += 360
				} else {
					unwrapped -= 360
					edge = -180
				}
				crossingLat := priorLat + (lat-priorLat)*(edge-priorLon)/(unwrapped-priorLon)
				line = append(line, heatProject(edge, crossingLat))
				flush()
				line = append(line, heatProject(-edge, crossingLat))
			}
		}
		point := heatProject(lon, lat)
		if len(line) == 0 || line[len(line)-1] != point {
			line = append(line, point)
		}
		previous = sample
	}
	flush()
	if len(geometry.Coordinates) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(geometry)
	return encoded
}

func heatFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
