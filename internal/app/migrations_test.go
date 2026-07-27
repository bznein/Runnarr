package app

import (
	"regexp"
	"sort"
	"testing"
)

func TestMigrationNumericPrefixes(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}

	prefixPattern := regexp.MustCompile(`^(\d+)_`)
	byPrefix := make(map[string][]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := prefixPattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 {
			t.Fatalf("migration %q does not start with a numeric prefix", entry.Name())
		}
		byPrefix[match[1]] = append(byPrefix[match[1]], entry.Name())
	}

	allowedHistoricalDuplicate := map[string]map[string]bool{
		"028": {
			"028_mobile_auth_handoffs.sql":             true,
			"028_training_sheet_preview_overrides.sql": true,
		},
	}
	for prefix, names := range byPrefix {
		if len(names) <= 1 {
			continue
		}
		sort.Strings(names)
		allowed := allowedHistoricalDuplicate[prefix]
		if len(allowed) != len(names) {
			t.Fatalf("duplicate migration prefix %s: %v", prefix, names)
		}
		for _, name := range names {
			if !allowed[name] {
				t.Fatalf("unexpected migration using historical duplicate prefix %s: %q", prefix, name)
			}
		}
	}
}
