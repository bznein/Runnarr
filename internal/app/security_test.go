package app

import (
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestParseEmailAllowlist(t *testing.T) {
	got := parseEmailAllowlist(" Alice@Example.com = Admin,invalid, bob@example.com=user ")
	if got["alice@example.com"] != "admin" || got["bob@example.com"] != "user" {
		t.Fatalf("allowlist = %#v", got)
	}
	if _, ok := got["invalid"]; ok {
		t.Fatal("malformed allowlist entry should be ignored")
	}
}

func TestLoadConfigKeepsLocalModeUsableWithoutOIDC(t *testing.T) {
	for _, key := range []string{
		"RUNNARR_PUBLIC_MODE", "RUNNARR_LOCAL_AUTH_ENABLED", "RUNNARR_BASE_URL",
		"DATABASE_URL", "RUNNARR_ADMIN_PASSWORD", "RUNNARR_ADMIN_PASSWORD_HASH", "RUNNARR_SECRET_KEY",
		"RUNNARR_OIDC_GOOGLE_CLIENT_ID", "RUNNARR_OIDC_GOOGLE_CLIENT_SECRET", "RUNNARR_OIDC_ALLOWED_EMAILS",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/runnarr")
	t.Setenv("RUNNARR_ADMIN_PASSWORD", "local-password")
	t.Setenv("RUNNARR_SECRET_KEY", "local-secret")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicMode || !cfg.LocalAuthEnabled {
		t.Fatalf("local config = %#v", cfg)
	}
}

func TestLoadConfigRejectsInsecurePublicMode(t *testing.T) {
	t.Setenv("RUNNARR_PUBLIC_MODE", "true")
	t.Setenv("RUNNARR_BASE_URL", "http://localhost:37617")
	t.Setenv("DATABASE_URL", "postgres://localhost/runnarr")
	t.Setenv("RUNNARR_ADMIN_PASSWORD", "strong-password")
	t.Setenv("RUNNARR_SECRET_KEY", "strong-secret")
	t.Setenv("RUNNARR_OIDC_GOOGLE_CLIENT_ID", "client")
	t.Setenv("RUNNARR_OIDC_GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("RUNNARR_OIDC_ALLOWED_EMAILS", "person@example.com=admin")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("insecure public mode should be rejected")
	}
}

func TestSessionCookieMode(t *testing.T) {
	local := &Server{cfg: Config{BaseURL: "http://localhost:8080"}}
	cookie := local.sessionCookie("id", time.Hour)
	if cookie.Name != sessionCookieName || cookie.Secure {
		t.Fatalf("local cookie = %#v", cookie)
	}

	public := &Server{cfg: Config{PublicMode: true, BaseURL: "https://runnarr.example"}}
	cookie = public.sessionCookie("id", time.Hour)
	if cookie.Name != publicSessionCookieName || !cookie.Secure {
		t.Fatalf("public cookie = %#v", cookie)
	}
}

func TestPublicSameOriginRequest(t *testing.T) {
	server := &Server{cfg: Config{PublicMode: true, BaseURL: "https://runnarr.example"}}
	request := httptest.NewRequest("POST", "https://runnarr.example/api/session/logout", nil)
	request.Header.Set("Origin", "https://runnarr.example")
	if !server.sameOriginRequest(request) {
		t.Fatal("same-origin request was rejected")
	}

	request.Header.Set("Origin", "https://evil.example")
	if server.sameOriginRequest(request) {
		t.Fatal("cross-origin request was accepted")
	}

	request.Header.Del("Origin")
	request.Header.Set("Referer", "https://runnarr.example/settings")
	if !server.sameOriginRequest(request) {
		t.Fatal("same-origin referer was rejected")
	}
}

func TestLoginRateLimiter(t *testing.T) {
	limiter := newLoginRateLimiter()
	now := time.Unix(100, 0)
	for i := 0; i < 10; i++ {
		if !limiter.allow("127.0.0.1", now) {
			t.Fatalf("attempt %d was rejected too early", i)
		}
	}
	if limiter.allow("127.0.0.1", now) {
		t.Fatal("eleventh attempt should be rejected")
	}
	if !limiter.allow("127.0.0.1", now.Add(time.Minute+time.Second)) {
		t.Fatal("expired attempts should no longer count")
	}
}

func TestServeSPAFailsClosedForTraversal(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(staticDir+"/index.html", []byte("index"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: Config{StaticDir: staticDir}}
	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost/../../etc/passwd", nil)
	server.serveSPA(response, request)
	if response.Code != 404 {
		t.Fatalf("traversal status = %d, want 404", response.Code)
	}
}
