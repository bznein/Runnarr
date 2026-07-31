package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthReportsBuildIdentity(t *testing.T) {
	previousVersion := BuildVersion
	previousCommit := BuildCommit
	previousMigrationHash := BuildMigrationHash
	t.Cleanup(func() {
		BuildVersion = previousVersion
		BuildCommit = previousCommit
		BuildMigrationHash = previousMigrationHash
	})

	BuildVersion = "1.2.3"
	BuildCommit = "0123456789abcdef"
	BuildMigrationHash = "migration-set"

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	(&Server{}).Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"status":        "ok",
		"version":       "1.2.3",
		"commit":        "0123456789abcdef",
		"migrationHash": "migration-set",
	} {
		if got := payload[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}
