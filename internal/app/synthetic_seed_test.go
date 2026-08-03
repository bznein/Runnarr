package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareSyntheticSeedSQL(t *testing.T) {
	prepared, err := prepareSyntheticSeedSQL("fixture.sql", "\\set ON_ERROR_STOP on\nselect :'e2e_username';\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prepared, `\set`) {
		t.Fatalf("prepared SQL retained psql command: %q", prepared)
	}
	if !strings.Contains(prepared, `current_setting('runnarr.synthetic_seed_username')`) {
		t.Fatalf("prepared SQL did not use the transaction-local username: %q", prepared)
	}
}

func TestPrepareSyntheticSeedSQLRejectsUnsupportedPSQL(t *testing.T) {
	_, err := prepareSyntheticSeedSQL("fixture.sql", "\\set ON_ERROR_STOP on\n\\quit 0\nselect :'e2e_username';\n")
	if err == nil || !strings.Contains(err.Error(), "unsupported psql command") {
		t.Fatalf("prepare error = %v", err)
	}
}

func TestPrepareSyntheticSeedSQLRequiresUsernameVariable(t *testing.T) {
	_, err := prepareSyntheticSeedSQL("fixture.sql", "\\set ON_ERROR_STOP on\nselect 1;\n")
	if err == nil || !strings.Contains(err.Error(), "missing username variable") {
		t.Fatalf("prepare error = %v", err)
	}
}

func TestSyntheticSeedFilesAreApplicationCompatible(t *testing.T) {
	for _, name := range syntheticSeedFiles {
		t.Run(name, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join("..", "..", "web", "e2e", name))
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := prepareSyntheticSeedSQL(name, string(contents))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(prepared, psqlUsernameVariable) {
				t.Fatalf("prepared SQL retained the psql username variable")
			}
		})
	}
}

func TestSyntheticSeedSkipsNonPreviewEnvironments(t *testing.T) {
	err := SeedSyntheticPreview(context.Background(), nil, Config{DeployEnvironment: "production"})
	if err != nil {
		t.Fatalf("non-preview seed = %v", err)
	}
}

func TestSyntheticPreviewUsesImmutablePRBuildIdentityAsCompatibilityFallback(t *testing.T) {
	previousVersion := BuildVersion
	t.Cleanup(func() { BuildVersion = previousVersion })

	BuildVersion = "pr-235-0123456789ab"
	if !syntheticPreviewEnabled(Config{}) {
		t.Fatal("PR candidate build identity did not enable preview seeding")
	}
	BuildVersion = "main-0123456789ab"
	if syntheticPreviewEnabled(Config{DeployEnvironment: "production"}) {
		t.Fatal("main production build identity enabled preview seeding")
	}
}
