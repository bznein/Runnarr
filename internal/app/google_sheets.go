package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const googleSheetsReadonlyScope = "https://www.googleapis.com/auth/spreadsheets.readonly"

type GoogleSheetsAuthService struct {
	store  *Store
	cfg    Config
	client *http.Client
}

func NewGoogleSheetsAuthService(store *Store, cfg Config) *GoogleSheetsAuthService {
	return &GoogleSheetsAuthService{store: store, cfg: cfg, client: &http.Client{Timeout: 45 * time.Second}}
}

func (s *GoogleSheetsAuthService) Configured() bool {
	return strings.TrimSpace(s.cfg.GoogleClientID) != "" && strings.TrimSpace(s.cfg.GoogleClientSecret) != ""
}

func (s *GoogleSheetsAuthService) Status(ctx context.Context) (GoogleSheetsStatus, error) {
	status := GoogleSheetsStatus{Configured: s.Configured(), Provider: "google_sheets"}
	if !status.Configured {
		return status, nil
	}
	_, err := s.store.LoadGoogleSheetsTokens(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Connected = true
	return status, nil
}

func (s *GoogleSheetsAuthService) AuthorizationURL(ctx context.Context, sessionID string) (string, error) {
	if !s.Configured() {
		return "", errors.New("Google OAuth is not configured")
	}
	state, err := randomGoogleState()
	if err != nil {
		return "", err
	}
	if err := s.store.CreateGoogleOAuthState(ctx, state, sessionID, time.Now().UTC().Add(10*time.Minute)); err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("client_id", s.cfg.GoogleClientID)
	query.Set("redirect_uri", s.cfg.GoogleRedirectURL)
	query.Set("response_type", "code")
	query.Set("scope", googleSheetsReadonlyScope)
	query.Set("access_type", "offline")
	query.Set("prompt", "consent")
	query.Set("state", state)
	return "https://accounts.google.com/o/oauth2/v2/auth?" + query.Encode(), nil
}

func (s *GoogleSheetsAuthService) Exchange(ctx context.Context, code string) error {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", s.cfg.GoogleClientID)
	form.Set("client_secret", s.cfg.GoogleClientSecret)
	form.Set("redirect_uri", s.cfg.GoogleRedirectURL)
	form.Set("grant_type", "authorization_code")
	response, err := postGoogleForm(ctx, s.client, "https://oauth2.googleapis.com/token", form)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var token googleTokenResponse
	if err := decodeGoogleResponse(response, &token); err != nil {
		return err
	}
	if token.RefreshToken == "" {
		return errors.New("Google OAuth did not return a refresh token")
	}
	return s.saveTokenResponse(refreshTokenOrEmpty(token.RefreshToken), token)
}

func refreshTokenOrEmpty(value string) string { return value }

func (s *GoogleSheetsAuthService) AccessToken(ctx context.Context) (string, error) {
	record, err := s.store.LoadGoogleSheetsTokens(ctx)
	if err != nil {
		return "", err
	}
	if record.TokenExpiresAt != nil && record.TokenExpiresAt.After(time.Now().UTC().Add(60*time.Second)) {
		accessToken, err := decryptGoogleSecret(s.cfg, record.AccessTokenCiphertext)
		if err == nil && accessToken != "" {
			return accessToken, nil
		}
	}
	refreshToken, err := decryptGoogleSecret(s.cfg, record.RefreshTokenCiphertext)
	if err != nil || refreshToken == "" {
		return "", errors.New("Google refresh token is unavailable")
	}
	form := url.Values{}
	form.Set("client_id", s.cfg.GoogleClientID)
	form.Set("client_secret", s.cfg.GoogleClientSecret)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")
	response, err := postGoogleForm(ctx, s.client, "https://oauth2.googleapis.com/token", form)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var token googleTokenResponse
	if err := decodeGoogleResponse(response, &token); err != nil {
		return "", err
	}
	if token.AccessToken == "" {
		return "", errors.New("Google OAuth refresh returned no access token")
	}
	if err := s.saveTokenResponse(refreshToken, token); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func (s *GoogleSheetsAuthService) saveTokenResponse(refreshToken string, token googleTokenResponse) error {
	accessCiphertext, err := encryptGoogleSecret(s.cfg, token.AccessToken)
	if err != nil {
		return err
	}
	refreshCiphertext, err := encryptGoogleSecret(s.cfg, refreshToken)
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
	return s.store.SaveGoogleSheetsTokens(context.Background(), accessCiphertext, refreshCiphertext, &expiresAt)
}

type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type googleSheetTab struct {
	ID          string
	Title       string
	RowCount    int
	ColumnCount int
	Values      [][]string
}

type googleSpreadsheetResponse struct {
	Sheets []struct {
		Properties struct {
			SheetID        int    `json:"sheetId"`
			Title          string `json:"title"`
			GridProperties struct {
				RowCount    int `json:"rowCount"`
				ColumnCount int `json:"columnCount"`
			} `json:"gridProperties"`
		} `json:"properties"`
	} `json:"sheets"`
}

type googleValuesResponse struct {
	ValueRanges []struct {
		Values [][]string `json:"values"`
	} `json:"valueRanges"`
}

func (s *GoogleSheetsAuthService) ReadWorkbook(ctx context.Context, sheetURL string) (string, []googleSheetTab, error) {
	sheetID, _, err := parseTrainingSheetID(sheetURL)
	if err != nil {
		return "", nil, err
	}
	accessToken, err := s.AccessToken(ctx)
	if err != nil {
		return "", nil, err
	}
	metadataURL := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s?fields=sheets(properties(sheetId,title,gridProperties))", url.PathEscape(sheetID))
	metadata, err := googleGET[googleSpreadsheetResponse](ctx, s.client, metadataURL, accessToken)
	if err != nil {
		return "", nil, err
	}
	tabs := make([]googleSheetTab, 0, len(metadata.Sheets))
	ranges := make([]string, 0, len(metadata.Sheets))
	for _, sheet := range metadata.Sheets {
		props := sheet.Properties
		rows := props.GridProperties.RowCount
		if rows <= 0 { rows = 1000 }
		if rows > 10000 { rows = 10000 }
		rangeName := fmt.Sprintf("'%s'!A1:AD%d", strings.ReplaceAll(props.Title, "'", "''"), rows)
		ranges = append(ranges, rangeName)
		tabs = append(tabs, googleSheetTab{ID: strconv.Itoa(props.SheetID), Title: props.Title, RowCount: rows, ColumnCount: props.GridProperties.ColumnCount})
	}
	if len(ranges) == 0 {
		return sheetID, tabs, nil
	}
	query := url.Values{}
	for _, rangeName := range ranges { query.Add("ranges", rangeName) }
	query.Set("valueRenderOption", "FORMATTED_VALUE")
	valuesURL := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s/values:batchGet?%s", url.PathEscape(sheetID), query.Encode())
	values, err := googleGET[googleValuesResponse](ctx, s.client, valuesURL, accessToken)
	if err != nil { return "", nil, fmt.Errorf("read workbook values: %w", err) }
	for index := range tabs {
		if index < len(values.ValueRanges) { tabs[index].Values = values.ValueRanges[index].Values }
	}
	return sheetID, tabs, nil
}

func googleGET[T any](ctx context.Context, client *http.Client, endpoint, accessToken string) (T, error) {
	var result T
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil { return result, err }
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil { return result, err }
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return result, fmt.Errorf("Google API returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil { return result, err }
	return result, nil
}

func decodeGoogleResponse(response *http.Response, target any) error {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Google OAuth returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func postGoogleForm(ctx context.Context, client *http.Client, endpoint string, form url.Values) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return client.Do(request)
}

func randomGoogleState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil { return "", err }
	return fmt.Sprintf("%x", buf), nil
}

func encryptGoogleSecret(cfg Config, value string) ([]byte, error) {
	block, err := aes.NewCipher(cfg.EncryptionKey())
	if err != nil { return nil, err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return nil, err }
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil { return nil, err }
	return gcm.Seal(nonce, nonce, []byte(value), nil), nil
}

func decryptGoogleSecret(cfg Config, ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(cfg.EncryptionKey())
	if err != nil { return "", err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return "", err }
	if len(ciphertext) < gcm.NonceSize() { return "", errors.New("encrypted Google token is too short") }
	nonce, payload := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, payload, nil)
	return string(plaintext), err
}
