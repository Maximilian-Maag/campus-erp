package budget

import (
	"errors"
	"testing"
)

// A source is built from a code and a name, with the chapter optional: not every funder has a state
// chapter. Everything is trimmed, and a blank half is refused rather than stored empty.
func TestNewSource(t *testing.T) {
	src, err := NewSource("  LHH ", "  Landesmittel ", " 05 ")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	if src.Code != "LHH" || src.Name != "Landesmittel" || src.Chapter != "05" || !src.Active {
		t.Fatalf("source = %+v", src)
	}
	// The chapter is optional.
	plain, err := NewSource("S-1", "Stiftung", "")
	if err != nil {
		t.Fatalf("NewSource without a chapter: %v", err)
	}
	if plain.Chapter != "" || !plain.Active {
		t.Fatalf("source = %+v", plain)
	}

	for _, c := range []struct {
		name        string
		code, title string
		want        error
	}{
		{"no code", "  ", "Landesmittel", ErrSourceCodeRequired},
		{"no name", "LHH", "  ", ErrSourceNameRequired},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewSource(c.code, c.title, ""); !errors.Is(err, c.want) {
				t.Fatalf("NewSource error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestNewCentre(t *testing.T) {
	centre, err := NewCentre("  BW-1 ", "  Fakultät 1 ")
	if err != nil {
		t.Fatalf("NewCentre: %v", err)
	}
	if centre.Code != "BW-1" || centre.Name != "Fakultät 1" || !centre.Active {
		t.Fatalf("centre = %+v", centre)
	}
	for _, c := range []struct {
		name        string
		code, name2 string
		want        error
	}{
		{"no code", "", "Fakultät 1", ErrCentreCodeRequired},
		{"no name", "BW-1", "   ", ErrCentreNameRequired},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewCentre(c.code, c.name2); !errors.Is(err, c.want) {
				t.Fatalf("NewCentre error = %v, want %v", err, c.want)
			}
		})
	}
}

// A position is "named" as soon as either half is set, which is what an assignment is about.
func TestPositionNamed(t *testing.T) {
	if (Position{}).Named() {
		t.Fatal("a position with neither half is not named")
	}
	id := int64(7)
	if !(Position{SourceID: &id}).Named() || !(Position{CentreID: &id}).Named() {
		t.Fatal("either half names the position")
	}
	if !(Position{SourceID: &id, CentreID: &id}).Named() {
		t.Fatal("both halves name the position")
	}
}

// An audit detail states a cleared half rather than leaving it out, so "nothing" and "not mentioned"
// do not read alike.
func TestIDOrNone(t *testing.T) {
	if got := idOrNone(nil); got != "none" {
		t.Fatalf("idOrNone(nil) = %q, want none", got)
	}
	id := int64(42)
	if got := idOrNone(&id); got != "42" {
		t.Fatalf("idOrNone(&42) = %q", got)
	}
}
