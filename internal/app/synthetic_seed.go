package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	previewDeployEnvironment = "preview"
	psqlUsernameVariable     = `:'e2e_username'`
	seedUsernameSetting      = "runnarr.synthetic_seed_username"
)

var syntheticSeedFiles = []string{"seed.sql", "testbed-seed.sql"}

// SeedSyntheticPreview applies the fixture bundle shipped in the exact
// candidate image. Preview databases are isolated and recreated for each PR
// revision, so running candidate-controlled fixture SQL does not grant the
// image access beyond the database it already owns.
func SeedSyntheticPreview(ctx context.Context, pool *pgxpool.Pool, cfg Config) error {
	if cfg.DeployEnvironment != previewDeployEnvironment {
		return nil
	}
	if strings.TrimSpace(cfg.SyntheticSeedDir) == "" {
		return fmt.Errorf("RUNNARR_SYNTHETIC_SEED_DIR is required for preview deployments")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin synthetic preview seed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `select set_config($1, $2, true)`, seedUsernameSetting, cfg.AdminUsername); err != nil {
		return fmt.Errorf("configure synthetic preview username: %w", err)
	}
	for _, name := range syntheticSeedFiles {
		path := filepath.Join(cfg.SyntheticSeedDir, name)
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read synthetic preview seed %s: %w", name, err)
		}
		sql, err := prepareSyntheticSeedSQL(name, string(contents))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, sql); err != nil {
			return fmt.Errorf("apply synthetic preview seed %s: %w", name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit synthetic preview seed: %w", err)
	}
	return nil
}

func prepareSyntheticSeedSQL(name, contents string) (string, error) {
	lines := strings.Split(contents, "\n")
	prepared := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, `\`) {
			prepared = append(prepared, line)
			continue
		}
		if trimmed != `\set ON_ERROR_STOP on` {
			return "", fmt.Errorf("prepare synthetic preview seed %s: unsupported psql command %q", name, trimmed)
		}
	}
	result := strings.Join(prepared, "\n")
	if !strings.Contains(result, psqlUsernameVariable) {
		return "", fmt.Errorf("prepare synthetic preview seed %s: missing username variable", name)
	}
	return strings.ReplaceAll(
		result,
		psqlUsernameVariable,
		`current_setting('`+seedUsernameSetting+`')`,
	), nil
}
