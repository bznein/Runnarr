package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	fit "github.com/tormoder/fit"
)

type ActivityParser interface {
	Name() string
	Parse(ctx context.Context, filename string, data []byte) (ImportedActivity, error)
}

type ImportService struct {
	store   *Store
	parsers map[string]ActivityParser
}

const maxImportUploadBytes = 80 << 20

var ErrImportFileTooLarge = errors.New("activity file must be 80 MiB or smaller")

func NewImportService(store *Store) *ImportService {
	parsers := []ActivityParser{
		GPXParser{},
		TCXParser{},
		FITParser{},
	}
	byName := make(map[string]ActivityParser, len(parsers))
	for _, parser := range parsers {
		byName[parser.Name()] = parser
	}
	return &ImportService{store: store, parsers: byName}
}

func (s *ImportService) ImportFile(ctx context.Context, filename, contentType string, reader io.Reader) (Activity, ImportFile, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxImportUploadBytes+1))
	if err != nil {
		return Activity{}, ImportFile{}, err
	}
	if len(data) == 0 {
		return Activity{}, ImportFile{}, errors.New("empty file")
	}
	if len(data) > maxImportUploadBytes {
		return Activity{}, ImportFile{}, ErrImportFileTooLarge
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	parser, err := s.parserFor(filename, data)
	if err != nil {
		return Activity{}, ImportFile{}, err
	}

	file, err := s.store.UpsertImportFile(ctx, ImportFile{
		Filename:    filename,
		ContentType: contentType,
		SHA256:      hash,
		SizeBytes:   int64(len(data)),
		Parser:      parser.Name(),
		Status:      "processing",
	})
	if err != nil {
		return Activity{}, ImportFile{}, err
	}

	imported, err := parser.Parse(ctx, filename, data)
	if err != nil {
		_ = s.store.UpdateImportStatus(ctx, file.ID, "failed", err.Error())
		return Activity{}, file, err
	}
	normalizeImported(&imported)

	sourceID := "file:" + hash
	activityID, err := s.store.SaveImportedActivity(ctx, "file", sourceID, &file.ID, imported)
	if err != nil {
		_ = s.store.UpdateImportStatus(ctx, file.ID, "failed", err.Error())
		return Activity{}, file, err
	}
	if err := s.store.UpdateImportStatus(ctx, file.ID, "imported", ""); err != nil {
		return Activity{}, file, err
	}
	file.Status = "imported"
	file.Error = ""

	activity, err := s.store.GetActivity(ctx, activityID)
	return activity, file, err
}

func (s *ImportService) parserFor(filename string, data []byte) (ActivityParser, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".gpx":
		return s.parsers["gpx"], nil
	case ".tcx":
		return s.parsers["tcx"], nil
	case ".fit":
		return s.parsers["fit"], nil
	}

	contentType := http.DetectContentType(data)
	lower := strings.ToLower(string(bytes.TrimSpace(data[:min(len(data), 128)])))
	if strings.Contains(lower, "<gpx") {
		return s.parsers["gpx"], nil
	}
	if strings.Contains(lower, "<trainingcenterdatabase") || strings.Contains(lower, "<activities") {
		return s.parsers["tcx"], nil
	}
	if strings.Contains(contentType, "xml") {
		return nil, fmt.Errorf("unsupported XML activity file %q", filename)
	}
	return nil, fmt.Errorf("unsupported activity file type %q", filename)
}

type GPXParser struct{}

func (GPXParser) Name() string { return "gpx" }

func (GPXParser) Parse(_ context.Context, filename string, data []byte) (ImportedActivity, error) {
	var doc gpxDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return ImportedActivity{}, err
	}
	if len(doc.Tracks) == 0 {
		return ImportedActivity{}, errors.New("GPX contains no tracks")
	}

	name := strings.TrimSpace(doc.Tracks[0].Name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}

	var samples []ActivitySample
	var firstTime *time.Time
	var lastTime *time.Time
	var lastLat, lastLon *float64
	var distance float64
	var elevationGain float64
	var lastElevation *float64
	var heartRates []int

	for _, track := range doc.Tracks {
		for _, segment := range track.Segments {
			for _, point := range segment.Points {
				idx := len(samples)
				lat := point.Lat
				lon := point.Lon
				if lastLat != nil && lastLon != nil {
					distance += haversine(*lastLat, *lastLon, lat, lon)
				}
				lastLat = &lat
				lastLon = &lon

				var ts *time.Time
				if point.Time != "" {
					if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(point.Time)); err == nil {
						ts = &parsed
						if firstTime == nil {
							firstTime = &parsed
						}
						lastTime = &parsed
					}
				}

				var elapsed *int
				if firstTime != nil && ts != nil {
					value := int(ts.Sub(*firstTime).Seconds())
					elapsed = &value
				}

				var elevation *float64
				if point.Elevation != nil {
					elevation = point.Elevation
					if lastElevation != nil {
						delta := *point.Elevation - *lastElevation
						if delta > 0 {
							elevationGain += delta
						}
					}
					lastElevation = point.Elevation
				}

				hr, cad := parseExtensions(point.Extensions.InnerXML)
				if hr != nil {
					heartRates = append(heartRates, *hr)
				}
				distanceValue := distance
				samples = append(samples, ActivitySample{
					Index:      idx,
					Timestamp:  ts,
					ElapsedS:   elapsed,
					DistanceM:  &distanceValue,
					Latitude:   &lat,
					Longitude:  &lon,
					ElevationM: elevation,
					HeartRate:  hr,
					Cadence:    cad,
				})
			}
		}
	}

	if len(samples) == 0 {
		return ImportedActivity{}, errors.New("GPX contains no track points")
	}

	start := time.Now().UTC()
	elapsed := 0
	if firstTime != nil {
		start = *firstTime
	}
	if firstTime != nil && lastTime != nil {
		elapsed = int(lastTime.Sub(*firstTime).Seconds())
	}
	avgHR, maxHR := heartRateSummary(heartRates)

	return ImportedActivity{
		Name:           name,
		SportType:      "Run",
		StartTime:      start,
		DistanceM:      distance,
		MovingTimeS:    elapsed,
		ElapsedTimeS:   elapsed,
		ElevationGainM: elevationGain,
		AvgHeartRate:   avgHR,
		MaxHeartRate:   maxHR,
		Samples:        samples,
		Raw:            map[string]any{"format": "gpx", "track_count": len(doc.Tracks)},
	}, nil
}

type TCXParser struct{}

func (TCXParser) Name() string { return "tcx" }

func (TCXParser) Parse(_ context.Context, filename string, data []byte) (ImportedActivity, error) {
	var doc tcxDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return ImportedActivity{}, err
	}
	if len(doc.Activities.Activities) == 0 {
		return ImportedActivity{}, errors.New("TCX contains no activities")
	}

	source := doc.Activities.Activities[0]
	name := strings.TrimSpace(source.ID)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	sport := normalizeSport(source.Sport)

	var samples []ActivitySample
	var laps []ActivityLap
	var firstTime *time.Time
	var lastTime *time.Time
	var distance float64
	var elevationGain float64
	var lastElevation *float64
	var heartRates []int
	var caloriesKcal int
	var hasCalories bool

	for lapIndex, lap := range source.Laps {
		if lap.Calories != nil {
			caloriesKcal += *lap.Calories
			hasCalories = true
		}
		var lapStart *time.Time
		if lap.StartTime != "" {
			if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(lap.StartTime)); err == nil {
				lapStart = &parsed
			}
		}
		laps = append(laps, ActivityLap{
			Index:        lapIndex,
			StartTime:    lapStart,
			ElapsedTimeS: int(lap.TotalTimeSeconds),
			MovingTimeS:  int(lap.TotalTimeSeconds),
			DistanceM:    lap.DistanceMeters,
		})

		for _, track := range lap.Tracks {
			for _, point := range track.Trackpoints {
				idx := len(samples)
				var ts *time.Time
				if point.Time != "" {
					if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(point.Time)); err == nil {
						ts = &parsed
						if firstTime == nil {
							firstTime = &parsed
						}
						lastTime = &parsed
					}
				}

				var elapsed *int
				if firstTime != nil && ts != nil {
					value := int(ts.Sub(*firstTime).Seconds())
					elapsed = &value
				}

				var lat, lon *float64
				if point.Position != nil {
					latValue := point.Position.LatitudeDegrees
					lonValue := point.Position.LongitudeDegrees
					lat = &latValue
					lon = &lonValue
				}

				var elevation *float64
				if point.AltitudeMeters != nil {
					elevation = point.AltitudeMeters
					if lastElevation != nil {
						delta := *point.AltitudeMeters - *lastElevation
						if delta > 0 {
							elevationGain += delta
						}
					}
					lastElevation = point.AltitudeMeters
				}

				var pointDistance *float64
				if point.DistanceMeters != nil {
					value := *point.DistanceMeters
					pointDistance = &value
					if value > distance {
						distance = value
					}
				}

				var hr *int
				if point.HeartRateBPM != nil {
					hrValue := point.HeartRateBPM.Value
					hr = &hrValue
					heartRates = append(heartRates, hrValue)
				}
				var cadence *int
				if point.Cadence != nil {
					cadenceValue := *point.Cadence
					cadenceValue = normalizeCadence(cadenceValue, sport)
					cadence = &cadenceValue
				}

				samples = append(samples, ActivitySample{
					Index:      idx,
					Timestamp:  ts,
					ElapsedS:   elapsed,
					DistanceM:  pointDistance,
					Latitude:   lat,
					Longitude:  lon,
					ElevationM: elevation,
					HeartRate:  hr,
					Cadence:    cadence,
				})
			}
		}
		if lap.DistanceMeters > distance {
			distance = lap.DistanceMeters
		}
	}

	if len(samples) == 0 {
		return ImportedActivity{}, errors.New("TCX contains no trackpoints")
	}
	start := time.Now().UTC()
	if firstTime != nil {
		start = *firstTime
	}
	elapsed := 0
	if firstTime != nil && lastTime != nil {
		elapsed = int(lastTime.Sub(*firstTime).Seconds())
	}
	moving := elapsed
	if len(laps) > 0 {
		total := 0
		for _, lap := range laps {
			total += lap.ElapsedTimeS
		}
		if total > 0 {
			moving = total
		}
	}
	avgHR, maxHR := heartRateSummary(heartRates)
	var calories *int
	if hasCalories {
		calories = &caloriesKcal
	}

	return ImportedActivity{
		Name:           name,
		SportType:      sport,
		StartTime:      start,
		DistanceM:      distance,
		MovingTimeS:    moving,
		ElapsedTimeS:   elapsed,
		ElevationGainM: elevationGain,
		AvgHeartRate:   avgHR,
		MaxHeartRate:   maxHR,
		CaloriesKcal:   calories,
		Samples:        samples,
		Laps:           laps,
		Raw:            map[string]any{"format": "tcx", "lap_count": len(source.Laps)},
	}, nil
}

type FITParser struct{}

func (FITParser) Name() string { return "fit" }

func fitSessionCaloriesKcal(session *fit.SessionMsg) *int {
	if session == nil {
		return nil
	}
	if session.TotalCalories == 0xFFFF {
		return nil
	}
	value := int(session.TotalCalories)
	return &value
}

type fitPauseInterval struct {
	Start time.Time
	End   time.Time
}

func fitPauseIntervals(events []*fit.EventMsg) []fitPauseInterval {
	if len(events) == 0 {
		return nil
	}

	sortedEvents := append([]*fit.EventMsg(nil), events...)
	sort.SliceStable(sortedEvents, func(left, right int) bool {
		if sortedEvents[left] == nil {
			return false
		}
		if sortedEvents[right] == nil {
			return true
		}
		return sortedEvents[left].Timestamp.Before(sortedEvents[right].Timestamp)
	})

	var pausedAt time.Time
	intervals := make([]fitPauseInterval, 0)
	for _, event := range sortedEvents {
		if event == nil || event.Event != fit.EventTimer || event.Timestamp.IsZero() {
			continue
		}
		switch event.EventType {
		case fit.EventTypeStop, fit.EventTypeStopAll, fit.EventTypeStopDisable:
			if pausedAt.IsZero() {
				pausedAt = event.Timestamp
			}
		case fit.EventTypeStart:
			if !pausedAt.IsZero() && !event.Timestamp.Before(pausedAt) {
				intervals = append(intervals, fitPauseInterval{Start: pausedAt, End: event.Timestamp})
			}
			pausedAt = time.Time{}
		}
	}
	if !pausedAt.IsZero() {
		intervals = append(intervals, fitPauseInterval{Start: pausedAt})
	}
	return intervals
}

func isFitPausedAt(timestamp time.Time, intervals []fitPauseInterval) bool {
	if timestamp.IsZero() {
		return false
	}
	for _, interval := range intervals {
		if timestamp.Before(interval.Start) {
			continue
		}
		if interval.End.IsZero() || timestamp.Before(interval.End) {
			return true
		}
	}
	return false
}

func fitRecordSpeed(record *fit.RecordMsg, paused bool) *float64 {
	if record == nil || paused {
		return nil
	}
	if value := record.GetEnhancedSpeedScaled(); !math.IsNaN(value) {
		return &value
	}
	return nil
}

func fitMovingDuration(start, end time.Time, intervals []fitPauseInterval) int {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return 0
	}

	moving := end.Sub(start)
	for _, interval := range intervals {
		pauseStart := interval.Start
		if pauseStart.Before(start) {
			pauseStart = start
		}
		pauseEnd := interval.End
		if pauseEnd.IsZero() || pauseEnd.After(end) {
			pauseEnd = end
		}
		if pauseEnd.After(pauseStart) {
			moving -= pauseEnd.Sub(pauseStart)
		}
	}
	if moving <= 0 {
		return 0
	}
	return int(moving.Seconds())
}

func (FITParser) Parse(_ context.Context, filename string, data []byte) (ImportedActivity, error) {
	decoded, err := fit.Decode(bytes.NewReader(data))
	if err != nil {
		return ImportedActivity{}, err
	}
	activityFile, err := decoded.Activity()
	if err != nil {
		return ImportedActivity{}, err
	}
	if len(activityFile.Records) == 0 {
		return ImportedActivity{}, errors.New("FIT contains no activity records")
	}

	var sport string
	var distance float64
	var moving int
	var elapsed int
	var avgPace *float64
	var sessionElevationGain *float64
	var calories *int
	if len(activityFile.Sessions) > 0 {
		session := activityFile.Sessions[0]
		sport = normalizeSport(session.Sport.String())
		if value := session.GetTotalDistanceScaled(); !math.IsNaN(value) {
			distance = value
		}
		if value := session.GetTotalTimerTimeScaled(); !math.IsNaN(value) && value > 0 {
			moving = int(value)
		} else if value := session.GetTotalMovingTimeScaled(); !math.IsNaN(value) && value > 0 {
			moving = int(value)
		}
		if value := session.GetTotalElapsedTimeScaled(); !math.IsNaN(value) {
			elapsed = int(value)
		}
		if value := session.GetEnhancedAvgSpeedScaled(); !math.IsNaN(value) && value > 0 {
			avgPace = paceFromSpeedMPS(value)
		} else if value := session.GetAvgSpeedScaled(); !math.IsNaN(value) && value > 0 {
			avgPace = paceFromSpeedMPS(value)
		}
		if session.TotalAscent != 0xFFFF {
			value := float64(session.TotalAscent)
			sessionElevationGain = &value
		}
		calories = fitSessionCaloriesKcal(session)
	}
	if sport == "" {
		sport = "Run"
	}
	pauseIntervals := fitPauseIntervals(activityFile.Events)

	var samples []ActivitySample
	var firstTime *time.Time
	var lastTime *time.Time
	var elevationGain float64
	var lastElevation *float64
	var heartRates []int
	for _, record := range activityFile.Records {
		idx := len(samples)
		ts := record.Timestamp
		if !ts.IsZero() {
			if firstTime == nil {
				firstTime = &ts
			}
			lastTime = &ts
		}
		var tsPtr *time.Time
		if !ts.IsZero() {
			tsPtr = &ts
		}
		var elapsedPtr *int
		if firstTime != nil && tsPtr != nil {
			value := int(ts.Sub(*firstTime).Seconds())
			elapsedPtr = &value
		}

		var lat, lon *float64
		if !record.PositionLat.Invalid() && !record.PositionLong.Invalid() {
			latValue := record.PositionLat.Degrees()
			lonValue := record.PositionLong.Degrees()
			if !math.IsNaN(latValue) && !math.IsNaN(lonValue) {
				lat = &latValue
				lon = &lonValue
			}
		}

		var elevation *float64
		if value := record.GetEnhancedAltitudeScaled(); !math.IsNaN(value) {
			elevation = &value
		} else if value := record.GetAltitudeScaled(); !math.IsNaN(value) {
			elevation = &value
		}
		if elevation != nil {
			if lastElevation != nil {
				delta := *elevation - *lastElevation
				if delta > 0 {
					elevationGain += delta
				}
			}
			lastElevation = elevation
		}

		var pointDistance *float64
		if value := record.GetDistanceScaled(); !math.IsNaN(value) {
			pointDistance = &value
			if value > distance {
				distance = value
			}
		}

		var hr *int
		if record.HeartRate > 0 && record.HeartRate < 255 {
			value := int(record.HeartRate)
			hr = &value
			heartRates = append(heartRates, value)
		}

		var cadence *int
		if record.Cadence > 0 && record.Cadence < 255 {
			value := normalizeCadence(int(record.Cadence), sport)
			cadence = &value
		}

		var power *int
		if record.Power > 0 {
			value := int(record.Power)
			power = &value
		}

		speed := fitRecordSpeed(record, isFitPausedAt(ts, pauseIntervals))

		samples = append(samples, ActivitySample{
			Index:      idx,
			Timestamp:  tsPtr,
			ElapsedS:   elapsedPtr,
			DistanceM:  pointDistance,
			Latitude:   lat,
			Longitude:  lon,
			ElevationM: elevation,
			HeartRate:  hr,
			Cadence:    cadence,
			Power:      power,
			SpeedMPS:   speed,
		})
	}

	if firstTime != nil && lastTime != nil && elapsed == 0 {
		elapsed = int(lastTime.Sub(*firstTime).Seconds())
	}
	if moving == 0 {
		if firstTime != nil && lastTime != nil {
			moving = fitMovingDuration(*firstTime, *lastTime, pauseIntervals)
		}
		if moving == 0 {
			moving = elapsed
		}
	}
	start := time.Now().UTC()
	if firstTime != nil {
		start = *firstTime
	}
	avgHR, maxHR := heartRateSummary(heartRates)
	sampleElevationGain := elevationGain
	if sessionElevationGain != nil {
		elevationGain = *sessionElevationGain
	}

	laps := make([]ActivityLap, 0, len(activityFile.Laps))
	for lapIndex, sourceLap := range activityFile.Laps {
		var lapStart *time.Time
		if !sourceLap.StartTime.IsZero() {
			start := sourceLap.StartTime
			lapStart = &start
		}
		lapElapsed := 0
		if value := sourceLap.GetTotalElapsedTimeScaled(); !math.IsNaN(value) {
			lapElapsed = int(value)
		}
		lapMoving := 0
		if value := sourceLap.GetTotalTimerTimeScaled(); !math.IsNaN(value) && value > 0 {
			lapMoving = int(value)
		} else if value := sourceLap.GetTotalMovingTimeScaled(); !math.IsNaN(value) && value > 0 {
			lapMoving = int(value)
		}
		if lapMoving <= 0 {
			if lapStart != nil && lapElapsed > 0 {
				lapEnd := sourceLap.Timestamp
				if lapEnd.IsZero() {
					lapEnd = lapStart.Add(time.Duration(lapElapsed) * time.Second)
				}
				lapMoving = fitMovingDuration(*lapStart, lapEnd, pauseIntervals)
			}
			if lapMoving <= 0 {
				lapMoving = lapElapsed
			}
		}
		lapDistance := 0.0
		if value := sourceLap.GetTotalDistanceScaled(); !math.IsNaN(value) {
			lapDistance = value
		}
		var lapPace *float64
		if value := sourceLap.GetEnhancedAvgSpeedScaled(); !math.IsNaN(value) && value > 0 {
			lapPace = paceFromSpeedMPS(value)
		} else if value := sourceLap.GetAvgSpeedScaled(); !math.IsNaN(value) && value > 0 {
			lapPace = paceFromSpeedMPS(value)
		}
		var lapGain *float64
		if sourceLap.TotalAscent != 0xFFFF {
			value := float64(sourceLap.TotalAscent)
			lapGain = &value
		}
		var lapLoss *float64
		if sourceLap.TotalDescent != 0xFFFF {
			value := float64(sourceLap.TotalDescent)
			lapLoss = &value
		}
		laps = append(laps, ActivityLap{
			Index:          lapIndex,
			StartTime:      lapStart,
			ElapsedTimeS:   lapElapsed,
			MovingTimeS:    lapMoving,
			DistanceM:      lapDistance,
			AvgPaceSPKM:    lapPace,
			ElevationGainM: lapGain,
			ElevationLossM: lapLoss,
		})
	}

	return ImportedActivity{
		Name:           strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)),
		SportType:      sport,
		StartTime:      start,
		DistanceM:      distance,
		MovingTimeS:    moving,
		ElapsedTimeS:   elapsed,
		ElevationGainM: elevationGain,
		AvgHeartRate:   avgHR,
		MaxHeartRate:   maxHR,
		AvgPaceSPKM:    avgPace,
		CaloriesKcal:   calories,
		Samples:        samples,
		Laps:           laps,
		Raw: map[string]any{
			"format":                  "fit",
			"record_count":            len(activityFile.Records),
			"lap_count":               len(activityFile.Laps),
			"sample_elevation_gain_m": sampleElevationGain,
		},
	}, nil
}

type gpxDocument struct {
	Tracks []gpxTrack `xml:"trk"`
}

type gpxTrack struct {
	Name     string       `xml:"name"`
	Segments []gpxSegment `xml:"trkseg"`
}

type gpxSegment struct {
	Points []gpxPoint `xml:"trkpt"`
}

type gpxPoint struct {
	Lat        float64       `xml:"lat,attr"`
	Lon        float64       `xml:"lon,attr"`
	Elevation  *float64      `xml:"ele"`
	Time       string        `xml:"time"`
	Extensions gpxExtensions `xml:"extensions"`
}

type gpxExtensions struct {
	InnerXML string `xml:",innerxml"`
}

type tcxDocument struct {
	Activities tcxActivities `xml:"Activities"`
}

type tcxActivities struct {
	Activities []tcxActivity `xml:"Activity"`
}

type tcxActivity struct {
	Sport string   `xml:"Sport,attr"`
	ID    string   `xml:"Id"`
	Laps  []tcxLap `xml:"Lap"`
}

type tcxLap struct {
	StartTime        string     `xml:"StartTime,attr"`
	TotalTimeSeconds float64    `xml:"TotalTimeSeconds"`
	DistanceMeters   float64    `xml:"DistanceMeters"`
	Calories         *int       `xml:"Calories"`
	Tracks           []tcxTrack `xml:"Track"`
}

type tcxTrack struct {
	Trackpoints []tcxTrackpoint `xml:"Trackpoint"`
}

type tcxTrackpoint struct {
	Time           string           `xml:"Time"`
	Position       *tcxPosition     `xml:"Position"`
	AltitudeMeters *float64         `xml:"AltitudeMeters"`
	DistanceMeters *float64         `xml:"DistanceMeters"`
	HeartRateBPM   *tcxHeartRateBPM `xml:"HeartRateBpm"`
	Cadence        *int             `xml:"Cadence"`
}

type tcxPosition struct {
	LatitudeDegrees  float64 `xml:"LatitudeDegrees"`
	LongitudeDegrees float64 `xml:"LongitudeDegrees"`
}

type tcxHeartRateBPM struct {
	Value int `xml:"Value"`
}

var (
	hrTagRegexp  = regexp.MustCompile(`(?i)<(?:[a-z0-9_]+:)?hr>\s*([0-9]+)\s*</(?:[a-z0-9_]+:)?hr>`)
	cadTagRegexp = regexp.MustCompile(`(?i)<(?:[a-z0-9_]+:)?cad(?:ence)?>\s*([0-9]+)\s*</(?:[a-z0-9_]+:)?cad(?:ence)?>`)
	// normalizeSport uses these regexes to turn provider type values into canonical UI labels.
	alphaNumericRegexp  = regexp.MustCompile(`[^a-z0-9]+`)
	versionSuffixRegexp = regexp.MustCompile(`(?i)^v[0-9]+$`)
)

func parseExtensions(value string) (*int, *int) {
	var hr *int
	if match := hrTagRegexp.FindStringSubmatch(value); len(match) == 2 {
		if parsed, err := strconv.Atoi(match[1]); err == nil {
			hr = &parsed
		}
	}
	var cadence *int
	if match := cadTagRegexp.FindStringSubmatch(value); len(match) == 2 {
		if parsed, err := strconv.Atoi(match[1]); err == nil {
			cadence = &parsed
		}
	}
	return hr, cadence
}

func normalizeCadence(value int, sport string) int {
	if value <= 0 {
		return value
	}
	if !isRunningSport(sport) {
		return value
	}
	return value * 2
}

func isRunningSport(value string) bool {
	switch normalizeSport(value) {
	case "Run", "Treadmill Run":
		return true
	default:
		return false
	}
}

func normalizeImported(activity *ImportedActivity) {
	if activity.SportType == "" {
		activity.SportType = "Run"
	}
	activity.SportType = normalizeSport(activity.SportType)
	if activity.Name == "" {
		activity.Name = fallbackName(*activity)
	}
	if activity.ElapsedTimeS == 0 && len(activity.Samples) > 1 {
		first := activity.Samples[0].Timestamp
		last := activity.Samples[len(activity.Samples)-1].Timestamp
		if first != nil && last != nil {
			activity.ElapsedTimeS = int(last.Sub(*first).Seconds())
		}
	}
	if activity.MovingTimeS == 0 {
		activity.MovingTimeS = activity.ElapsedTimeS
	}
	for index := range activity.Laps {
		if activity.Laps[index].MovingTimeS <= 0 {
			activity.Laps[index].MovingTimeS = activity.Laps[index].ElapsedTimeS
		}
	}
}

func paceFromSpeedMPS(speed float64) *float64 {
	if math.IsNaN(speed) || math.IsInf(speed, 0) || speed <= 0 {
		return nil
	}
	pace := 1000 / speed
	return &pace
}

func normalizeSport(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "Run"
	}

	tokens := strings.Fields(alphaNumericRegexp.ReplaceAllString(value, " "))
	clean := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token == "" || versionSuffixRegexp.MatchString(token) {
			continue
		}
		clean = append(clean, token)
	}
	if len(clean) == 0 {
		return "Run"
	}

	joined := strings.Join(clean, "")
	switch joined {
	case "run", "running":
		return "Run"
	case "ride", "virtualride", "cycling", "biking", "bike":
		return "Cycling"
	case "walk", "walking":
		return "Walk"
	case "hike", "hiking":
		return "Hike"
	case "kayaking", "kayakingv2":
		return "Kayaking"
	case "swim", "swimming":
		return "Swimming"
	case "strength", "strengthtraining", "weighttraining", "weightlifting", "workout":
		return "Strength Training"
	case "treadmill", "treadmillrunning", "treadmillrun", "runningtreadmill":
		return "Treadmill Run"
	}

	if containsAll(clean, "lap", "swimming") || containsAll(clean, "lap", "swim") {
		return "Swimming"
	}
	if containsAny(clean, "treadmill") && containsAny(clean, "run", "running") {
		return "Treadmill Run"
	}
	if containsAny(clean, "virtual", "ride") && !containsAny(clean, "run", "running") {
		return "Cycling"
	}
	if containsAny(clean, "strength", "weight", "lifting", "workout") {
		return "Strength Training"
	}
	if containsAny(clean, "ride", "cycling", "biking", "bike") {
		return "Cycling"
	}
	if containsAny(clean, "swim", "swimming") {
		return "Swimming"
	}
	if containsAny(clean, "hike", "hiking") {
		return "Hike"
	}
	if containsAny(clean, "walk", "walking") {
		return "Walk"
	}
	if containsAny(clean, "run", "running") {
		return "Run"
	}
	if containsAny(clean, "kayak") {
		return "Kayaking"
	}

	parts := make([]string, 0, len(clean))
	for _, token := range clean {
		parts = append(parts, strings.ToUpper(token[:1])+token[1:])
	}
	return strings.Join(parts, " ")
}

func containsAny(values []string, wanted ...string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, want := range wanted {
		if _, ok := seen[want]; ok {
			return true
		}
	}
	return false
}

func containsAll(values []string, wanted ...string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, want := range wanted {
		if _, ok := seen[want]; !ok {
			return false
		}
	}
	return true
}

func heartRateSummary(values []int) (*float64, *float64) {
	if len(values) == 0 {
		return nil, nil
	}
	var sum int
	var max int
	for _, value := range values {
		sum += value
		if value > max {
			max = value
		}
	}
	avg := float64(sum) / float64(len(values))
	maxFloat := float64(max)
	return &avg, &maxFloat
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371000
	toRad := func(v float64) float64 { return v * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
