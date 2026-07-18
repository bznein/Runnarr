package app

import (
	"testing"
)

func TestLoadConfigEnableProfiling(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("RUNNARR_SECRET_KEY", "test-secret-key-12345678901234567890")
	t.Setenv("RUNNARR_ADMIN_PASSWORD", "admin")
	t.Setenv("RUNNARR_ENABLE_PROFILING", "true")
	t.Setenv("RUNNARR_PROFILING_ADDR", ":7000")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if !cfg.EnableProfiling {
		t.Fatalf("enable profiling = %v, want true", cfg.EnableProfiling)
	}
	if cfg.ProfilingAddr != ":7000" {
		t.Fatalf("profiling addr = %q, want %q", cfg.ProfilingAddr, ":7000")
	}
}

func TestLoadConfigInvalidProfilingValueFallsBackToFalse(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("RUNNARR_SECRET_KEY", "test-secret-key-12345678901234567890")
	t.Setenv("RUNNARR_ADMIN_PASSWORD", "admin")
	t.Setenv("RUNNARR_ENABLE_PROFILING", "not-bool")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.EnableProfiling {
		t.Fatalf("enable profiling = %v, want false for invalid value", cfg.EnableProfiling)
	}
}
