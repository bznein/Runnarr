package app

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	BaseURL           string
	AdminPassword     string
	AdminPasswordHash string
	SecretKey         string
	MapTileURL        string
	StaticDir         string
}

func LoadConfig() (Config, error) {
	cfg := Config{
		HTTPAddr:          env("RUNNARR_HTTP_ADDR", ":8080"),
		DatabaseURL:       env("DATABASE_URL", ""),
		BaseURL:           strings.TrimRight(env("RUNNARR_BASE_URL", "http://localhost:8080"), "/"),
		AdminPassword:     env("RUNNARR_ADMIN_PASSWORD", ""),
		AdminPasswordHash: env("RUNNARR_ADMIN_PASSWORD_HASH", ""),
		SecretKey:         env("RUNNARR_SECRET_KEY", ""),
		MapTileURL:        env("MAP_TILE_URL", "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"),
		StaticDir:         env("RUNNARR_STATIC_DIR", "web/dist"),
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
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
