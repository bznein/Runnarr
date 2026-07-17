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

func TestLocalActivityNotesValue(t *testing.T) {
	notes, err := localActivityNotesValue("  Felt smooth\nNeed colder drink  ")
	if err != nil {
		t.Fatal(err)
	}
	if notes != "Felt smooth\nNeed colder drink" {
		t.Fatalf("notes = %q, want trimmed note", notes)
	}
	if notes, err := localActivityNotesValue("   "); err != nil || notes != "" {
		t.Fatalf("blank notes = %q, err = %v; want cleared notes", notes, err)
	}
}

func TestLocalActivityNotesValueRejectsLongNotes(t *testing.T) {
	if _, err := localActivityNotesValue(strings.Repeat("a", 5001)); !errors.Is(err, ErrInvalidActivityNotes) {
		t.Fatalf("long notes error = %v, want ErrInvalidActivityNotes", err)
	}
}
