package app

import (
	"errors"
	"strings"
	"testing"
)

func TestLocalActivityNameOverride(t *testing.T) {
	localName, err := localActivityNameOverride("  Rainy tempo  ", "Morning Run")
	if err != nil {
		t.Fatal(err)
	}
	if localName != "Rainy tempo" {
		t.Fatalf("local name = %q, want trimmed override", localName)
	}
}

func TestLocalActivityNameOverrideClearsWhenSourceNameMatches(t *testing.T) {
	sourceName := strings.Repeat("a", 161)
	localName, err := localActivityNameOverride(" "+sourceName+" ", sourceName)
	if err != nil {
		t.Fatal(err)
	}
	if localName != "" {
		t.Fatalf("local name = %q, want cleared override", localName)
	}
}

func TestLocalActivityNameOverrideRejectsInvalidNames(t *testing.T) {
	for _, name := range []string{"", "   ", strings.Repeat("a", 161)} {
		if _, err := localActivityNameOverride(name, "Morning Run"); !errors.Is(err, ErrInvalidActivityName) {
			t.Fatalf("localActivityNameOverride(%q) error = %v, want ErrInvalidActivityName", name, err)
		}
	}
}
