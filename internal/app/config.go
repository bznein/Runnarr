package app

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	BaseURL            string
	PublicMode         bool
	LocalAuthEnabled   bool
	TrustProxy         bool
	AdminUsername      string
	AdminPassword      string
	AdminPasswordHash  string
	SecretKey          string
	MapTileURL         string
	StaticDir          string
	MediaDir           string
	GarminBridgePython string
	GarminBridgeScript string
	GarminTokenDir     string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	OIDCClientID       string
	OIDCClientSecret   string
	OIDCRedirectURL    string
	OIDCIssuerURL      string
	OIDCAllowedEmails  map[string]string
}

func LoadConfig() (Config, error) {
	publicMode := envBool("RUNNARR_PUBLIC_MODE", false)
	cfg := Config{
		HTTPAddr:           env("RUNNARR_HTTP_ADDR", ":8080"),
		DatabaseURL:        env("DATABASE_URL", ""),
		BaseURL:            strings.TrimRight(env("RUNNARR_BASE_URL", "http://localhost:8080"), "/"),
		PublicMode:         publicMode,
		LocalAuthEnabled:   envBool("RUNNARR_LOCAL_AUTH_ENABLED", !publicMode),
		TrustProxy:         envBool("RUNNARR_TRUST_PROXY", false),
		AdminUsername:      env("RUNNARR_ADMIN_USERNAME", "admin"),
		AdminPassword:      env("RUNNARR_ADMIN_PASSWORD", ""),
		AdminPasswordHash:  env("RUNNARR_ADMIN_PASSWORD_HASH", ""),
		SecretKey:          env("RUNNARR_SECRET_KEY", ""),
		MapTileURL:         env("MAP_TILE_URL", "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"),
		StaticDir:          env("RUNNARR_STATIC_DIR", "web/dist"),
		MediaDir:           env("RUNNARR_MEDIA_DIR", "data/media"),
		GarminBridgePython: env("RUNNARR_GARMIN_BRIDGE_PYTHON", "python3"),
		GarminBridgeScript: env("RUNNARR_GARMIN_BRIDGE_SCRIPT", "internal/app/garmin_bridge.py"),
		GarminTokenDir:     env("RUNNARR_GARMIN_TOKEN_DIR", "data/garmin_tokens"),
		GoogleClientID:     env("RUNNARR_GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: env("RUNNARR_GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  env("RUNNARR_GOOGLE_REDIRECT_URL", ""),
		OIDCClientID:       env("RUNNARR_OIDC_GOOGLE_CLIENT_ID", ""),
		OIDCClientSecret:   env("RUNNARR_OIDC_GOOGLE_CLIENT_SECRET", ""),
		OIDCRedirectURL:    env("RUNNARR_OIDC_GOOGLE_REDIRECT_URL", ""),
		OIDCIssuerURL:      env("RUNNARR_OIDC_ISSUER_URL", "https://accounts.google.com"),
		OIDCAllowedEmails:  parseEmailAllowlist(env("RUNNARR_OIDC_ALLOWED_EMAILS", "")),
	}
	if cfg.GoogleRedirectURL == "" {
		cfg.GoogleRedirectURL = cfg.BaseURL + "/api/providers/google/callback"
	}
	if cfg.OIDCRedirectURL == "" {
		cfg.OIDCRedirectURL = cfg.BaseURL + "/api/auth/google/callback"
	}

	if cfg.DatabaseURL == "" {
		return cfg, errors.New("DATABASE_URL is required")
	}
	if cfg.AdminPassword == "" && cfg.AdminPasswordHash == "" {
		return cfg, errors.New("RUNNARR_ADMIN_PASSWORD or RUNNARR_ADMIN_PASSWORD_HASH is required")
	}
	if cfg.SecretKey == "" {
		return cfg, errors.New("RUNNARR_SECRET_KEY is required")
	}
	if cfg.PublicMode {
		baseURL, err := url.Parse(cfg.BaseURL)
		if err != nil || !strings.EqualFold(baseURL.Scheme, "https") || baseURL.Host == "" || baseURL.User != nil {
			return cfg, errors.New("RUNNARR_BASE_URL must use https in public mode")
		}
		if cfg.OIDCClientID == "" || cfg.OIDCClientSecret == "" {
			return cfg, errors.New("Google OIDC credentials are required in public mode")
		}
		if !strings.EqualFold(strings.TrimRight(cfg.OIDCIssuerURL, "/"), "https://accounts.google.com") {
			return cfg, errors.New("public mode only supports the HTTPS Google OIDC issuer")
		}
		if cfg.OIDCRedirectURL != cfg.BaseURL+"/api/auth/google/callback" {
			return cfg, errors.New("Google OIDC redirect URL must match RUNNARR_BASE_URL")
		}
		if len(cfg.OIDCAllowedEmails) == 0 {
			return cfg, errors.New("RUNNARR_OIDC_ALLOWED_EMAILS is required in public mode")
		}
		if cfg.AdminPasswordHash == "" && cfg.AdminPassword == "change-me" {
			return cfg, errors.New("the default administrator password cannot be used in public mode")
		}
		if cfg.SecretKey == "change-this-to-a-long-random-secret-with-at-least-32-bytes" {
			return cfg, errors.New("the default secret key cannot be used in public mode")
		}
		if strings.Contains(cfg.DatabaseURL, "runnarr:runnarr@") {
			return cfg, errors.New("the default database password cannot be used in public mode")
		}
	}
	return cfg, nil
}

func (c Config) UsingPlainAdminPassword() bool {
	return c.AdminPassword != "" && c.AdminPasswordHash == ""
}

func (c Config) EncryptionKey() []byte {
	if decoded, err := base64.StdEncoding.DecodeString(c.SecretKey); err == nil && len(decoded) == 32 {
		return decoded
	}
	if len(c.SecretKey) == 32 {
		return []byte(c.SecretKey)
	}
	sum := sha256.Sum256([]byte(c.SecretKey))
	return sum[:]
}

func env(key, fallback string) string {
	if file := strings.TrimSpace(os.Getenv(key + "_FILE")); file != "" {
		if data, err := os.ReadFile(file); err == nil {
			if value := strings.TrimSpace(string(data)); value != "" {
				return value
			}
		}
	}
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseEmailAllowlist(value string) map[string]string {
	allowed := make(map[string]string)
	for _, item := range strings.Split(value, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) != 2 {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(parts[0]))
		username := normalizeUsername(parts[1])
		if email != "" && username != "" {
			allowed[email] = username
		}
	}
	return allowed
}
