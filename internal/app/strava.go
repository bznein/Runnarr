package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const stravaProvider = "strava"

type StravaService struct {
	cfg    Config
	store  *Store
	cipher *TokenCipher
	client *http.Client
}

func NewStravaService(cfg Config, store *Store, cipher *TokenCipher) *StravaService {
	return &StravaService{
		cfg:    cfg,
		store:  store,
		cipher: cipher,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *StravaService) AuthorizationURL(state string) (string, error) {
	if !s.cfg.StravaConfigured() {
		return "", errors.New("Strava is not configured")
	}
	values := url.Values{}
	values.Set("client_id", s.cfg.StravaClientID)
	values.Set("redirect_uri", s.cfg.StravaRedirectURL())
	values.Set("response_type", "code")
	values.Set("approval_prompt", "auto")
	values.Set("scope", "activity:read")
	values.Set("state", state)
	return "https://www.strava.com/oauth/authorize?" + values.Encode(), nil
}

func (s *StravaService) ExchangeCode(ctx context.Context, code string) (ProviderConnection, error) {
	if !s.cfg.StravaConfigured() {
		return ProviderConnection{}, errors.New("Strava is not configured")
	}
	values := url.Values{}
	values.Set("client_id", s.cfg.StravaClientID)
	values.Set("client_secret", s.cfg.StravaClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")

	var token stravaTokenResponse
	if err := s.postForm(ctx, "https://www.strava.com/oauth/token", values, &token); err != nil {
		return ProviderConnection{}, err
	}
	return s.saveTokenResponse(ctx, token)
}

func (s *StravaService) Status(ctx context.Context) (ProviderConnection, bool, error) {
	conn, err := s.store.GetProviderConnection(ctx, stravaProvider)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderConnection{Provider: stravaProvider}, false, nil
	}
	if err != nil {
		return ProviderConnection{}, false, err
	}
	return conn.ProviderConnection, true, nil
}

func (s *StravaService) Sync(ctx context.Context) (map[string]any, error) {
	conn, err := s.store.GetProviderConnection(ctx, stravaProvider)
	if err != nil {
		return nil, err
	}
	accessToken, err := s.accessToken(ctx, conn)
	if err != nil {
		return nil, err
	}

	imported := 0
	pages := 0
	var rateLimit, rateUsage, readLimit, readUsage string
	for page := 1; page <= 20; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.strava.com/api/v3/athlete/activities?per_page=100&page="+strconv.Itoa(page), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		rateLimit = resp.Header.Get("X-RateLimit-Limit")
		rateUsage = resp.Header.Get("X-RateLimit-Usage")
		readLimit = resp.Header.Get("X-ReadRateLimit-Limit")
		readUsage = resp.Header.Get("X-ReadRateLimit-Usage")
		if resp.StatusCode >= 300 {
			defer resp.Body.Close()
			var apiErr map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&apiErr)
			return nil, fmt.Errorf("Strava activities request failed: %s", resp.Status)
		}

		var activities []stravaActivity
		if err := json.NewDecoder(resp.Body).Decode(&activities); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		if len(activities) == 0 {
			break
		}
		pages++
		for _, source := range activities {
			importedActivity := source.Imported()
			normalizeImported(&importedActivity)
			if _, err := s.store.SaveImportedActivity(ctx, stravaProvider, strconv.FormatInt(source.ID, 10), nil, importedActivity); err != nil {
				return nil, err
			}
			imported++
		}
	}

	return map[string]any{
		"imported":      imported,
		"pages":         pages,
		"rateLimit":     rateLimit,
		"rateUsage":     rateUsage,
		"readRateLimit": readLimit,
		"readRateUsage": readUsage,
	}, nil
}

func (s *StravaService) accessToken(ctx context.Context, conn StoredProviderConnection) (string, error) {
	if conn.TokenExpiresAt.After(time.Now().Add(1 * time.Minute)) {
		return s.cipher.DecryptString(conn.AccessTokenCiphertext)
	}
	refreshToken, err := s.cipher.DecryptString(conn.RefreshTokenCiphertext)
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("client_id", s.cfg.StravaClientID)
	values.Set("client_secret", s.cfg.StravaClientSecret)
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)

	var token stravaTokenResponse
	if err := s.postForm(ctx, "https://www.strava.com/oauth/token", values, &token); err != nil {
		return "", err
	}
	if _, err := s.saveTokenResponse(ctx, token); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func (s *StravaService) saveTokenResponse(ctx context.Context, token stravaTokenResponse) (ProviderConnection, error) {
	accessCiphertext, err := s.cipher.EncryptString(token.AccessToken)
	if err != nil {
		return ProviderConnection{}, err
	}
	refreshCiphertext, err := s.cipher.EncryptString(token.RefreshToken)
	if err != nil {
		return ProviderConnection{}, err
	}

	displayName := strings.TrimSpace(token.Athlete.FirstName + " " + token.Athlete.LastName)
	if displayName == "" {
		displayName = token.Athlete.Username
	}
	if displayName == "" {
		displayName = strconv.FormatInt(token.Athlete.ID, 10)
	}

	conn := StoredProviderConnection{
		ProviderConnection: ProviderConnection{
			Provider:          stravaProvider,
			ProviderAccountID: strconv.FormatInt(token.Athlete.ID, 10),
			DisplayName:       displayName,
			Scopes:            strings.Fields(strings.ReplaceAll(token.Scope, ",", " ")),
			TokenExpiresAt:    time.Unix(token.ExpiresAt, 0).UTC(),
		},
		AccessTokenCiphertext:  accessCiphertext,
		RefreshTokenCiphertext: refreshCiphertext,
	}
	if err := s.store.UpsertProviderConnection(ctx, conn); err != nil {
		return ProviderConnection{}, err
	}
	saved, err := s.store.GetProviderConnection(ctx, stravaProvider)
	if err != nil {
		return ProviderConnection{}, err
	}
	return saved.ProviderConnection, nil
}

func (s *StravaService) postForm(ctx context.Context, endpoint string, values url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var apiErr map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return fmt.Errorf("Strava token request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type stravaTokenResponse struct {
	TokenType    string        `json:"token_type"`
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	ExpiresAt    int64         `json:"expires_at"`
	ExpiresIn    int           `json:"expires_in"`
	Scope        string        `json:"scope"`
	Athlete      stravaAthlete `json:"athlete"`
}

type stravaAthlete struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
}

type stravaActivity struct {
	ID                 int64          `json:"id"`
	Name               string         `json:"name"`
	Type               string         `json:"type"`
	SportType          string         `json:"sport_type"`
	StartDate          time.Time      `json:"start_date"`
	Distance           float64        `json:"distance"`
	MovingTime         int            `json:"moving_time"`
	ElapsedTime        int            `json:"elapsed_time"`
	TotalElevationGain float64        `json:"total_elevation_gain"`
	AverageHeartRate   *float64       `json:"average_heartrate"`
	MaxHeartRate       *float64       `json:"max_heartrate"`
	Map                stravaRouteMap `json:"map"`
}

type stravaRouteMap struct {
	SummaryPolyline string `json:"summary_polyline"`
}

func (a stravaActivity) Imported() ImportedActivity {
	sport := a.SportType
	if sport == "" {
		sport = a.Type
	}
	return ImportedActivity{
		Name:            a.Name,
		SportType:       sport,
		StartTime:       a.StartDate,
		DistanceM:       a.Distance,
		MovingTimeS:     a.MovingTime,
		ElapsedTimeS:    a.ElapsedTime,
		ElevationGainM:  a.TotalElevationGain,
		AvgHeartRate:    a.AverageHeartRate,
		MaxHeartRate:    a.MaxHeartRate,
		SummaryPolyline: a.Map.SummaryPolyline,
		Raw: map[string]any{
			"provider": "strava",
			"id":       a.ID,
		},
	}
}
