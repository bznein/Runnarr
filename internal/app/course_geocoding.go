package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxCoursePlaceQueryLength  = 200
	maxCoursePlaceResponse     = 1 << 20
	maxCoursePlaceResults      = 5
	maxCoursePlaceCacheEntries = 256
	coursePlaceCacheTTL        = 24 * time.Hour
)

var (
	errCourseGeocodingDisabled = errors.New("course place search is not configured")
	errCourseGeocodingRate     = errors.New("course place search is temporarily rate limited")
	errCourseGeocodingInput    = errors.New("invalid course place search")
)

type CourseGeocodingService struct {
	enabled     bool
	baseURL     string
	client      *http.Client
	minInterval time.Duration
	mu          sync.Mutex
	nextRequest time.Time
	cache       map[[sha256.Size]byte]coursePlaceCacheEntry
}

type coursePlaceCacheEntry struct {
	results   []CoursePlaceResult
	expiresAt time.Time
}

type CoursePlaceResult struct {
	Name        string  `json:"name"`
	DisplayName string  `json:"displayName"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

type nominatimPlace struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Latitude    string `json:"lat"`
	Longitude   string `json:"lon"`
}

func NewCourseGeocodingService(cfg Config) *CourseGeocodingService {
	return &CourseGeocodingService{
		enabled:     cfg.GeocodingEnabled,
		baseURL:     cfg.GeocodingURL,
		client:      &http.Client{Timeout: 8 * time.Second},
		minInterval: time.Second,
	}
}

func (service *CourseGeocodingService) Search(ctx context.Context, query string) ([]CoursePlaceResult, error) {
	if service == nil || !service.enabled {
		return nil, errCourseGeocodingDisabled
	}
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > maxCoursePlaceQueryLength {
		return nil, fmt.Errorf("%w: query must contain between 2 and %d characters", errCourseGeocodingInput, maxCoursePlaceQueryLength)
	}

	cacheKey := sha256.Sum256([]byte(strings.ToLower(query)))
	service.mu.Lock()
	now := time.Now()
	if cached, ok := service.cache[cacheKey]; ok && now.Before(cached.expiresAt) {
		results := append([]CoursePlaceResult(nil), cached.results...)
		service.mu.Unlock()
		return results, nil
	}
	if now.Before(service.nextRequest) {
		service.mu.Unlock()
		return nil, errCourseGeocodingRate
	}
	service.nextRequest = now.Add(service.minInterval)
	service.mu.Unlock()

	endpoint, err := url.Parse(service.baseURL + "/search")
	if err != nil {
		return nil, fmt.Errorf("build geocoding request: %w", err)
	}
	params := endpoint.Query()
	params.Set("q", query)
	params.Set("format", "jsonv2")
	params.Set("limit", strconv.Itoa(maxCoursePlaceResults))
	params.Set("addressdetails", "0")
	endpoint.RawQuery = params.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build geocoding request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Runnarr course planner (https://github.com/bznein/Runnarr)")
	response, err := service.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("search places: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geocoding service returned status %d", response.StatusCode)
	}

	var providerResults []nominatimPlace
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxCoursePlaceResponse+1))
	if err := decoder.Decode(&providerResults); err != nil {
		return nil, fmt.Errorf("decode geocoding response: %w", err)
	}
	results := make([]CoursePlaceResult, 0, min(len(providerResults), maxCoursePlaceResults))
	for _, providerResult := range providerResults {
		if len(results) == maxCoursePlaceResults {
			break
		}
		latitude, latitudeErr := strconv.ParseFloat(providerResult.Latitude, 64)
		longitude, longitudeErr := strconv.ParseFloat(providerResult.Longitude, 64)
		displayName := strings.TrimSpace(providerResult.DisplayName)
		if latitudeErr != nil || longitudeErr != nil || !validCourseCoordinate(latitude, longitude) || displayName == "" {
			continue
		}
		name := strings.TrimSpace(providerResult.Name)
		if name == "" {
			name = strings.TrimSpace(strings.SplitN(displayName, ",", 2)[0])
		}
		results = append(results, CoursePlaceResult{Name: name, DisplayName: displayName, Latitude: latitude, Longitude: longitude})
	}
	service.mu.Lock()
	if service.cache == nil {
		service.cache = make(map[[sha256.Size]byte]coursePlaceCacheEntry)
	}
	for key, cached := range service.cache {
		if !now.Before(cached.expiresAt) {
			delete(service.cache, key)
		}
	}
	if len(service.cache) >= maxCoursePlaceCacheEntries {
		var oldestKey [sha256.Size]byte
		oldestExpiry := time.Time{}
		for key, cached := range service.cache {
			if oldestExpiry.IsZero() || cached.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = key, cached.expiresAt
			}
		}
		delete(service.cache, oldestKey)
	}
	service.cache[cacheKey] = coursePlaceCacheEntry{results: append([]CoursePlaceResult(nil), results...), expiresAt: now.Add(coursePlaceCacheTTL)}
	service.mu.Unlock()
	return results, nil
}

func (s *Server) handleSearchCoursePlaces(w http.ResponseWriter, r *http.Request) {
	results, err := s.courseGeocoding.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		switch {
		case errors.Is(err, errCourseGeocodingDisabled):
			writeError(w, http.StatusNotFound, "course place search is not configured")
		case errors.Is(err, errCourseGeocodingInput):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, errCourseGeocodingRate):
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "wait a moment before searching again")
		default:
			s.logger.Error("search course places", "error", err)
			writeError(w, http.StatusBadGateway, "place search is unavailable")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
