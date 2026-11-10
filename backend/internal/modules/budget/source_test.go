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

func TestNewCenter(t *testing.T) {
	center, err := NewCenter("  BW-1 ", "  Fakultät 1 ")
	if err != nil {
		t.Fatalf("NewCenter: %v", err)
	}
	if center.Code != "BW-1" || center.Name != "Fakultät 1" || !center.Active {
		t.Fatalf("centre = %+v", center)
	}
	for _, c := range []struct {
		name        string
		code, name2 string
		want        error
	}{
		{"no code", "", "Fakultät 1", ErrCenterCodeRequired},
		{"no name", "BW-1", "   ", ErrCenterNameRequired},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewCenter(c.code, c.name2); !errors.Is(err, c.want) {
				t.Fatalf("NewCenter error = %v, want %v", err, c.want)
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
	if !(Position{SourceID: &id}).Named() || !(Position{CenterID: &id}).Named() {
		t.Fatal("either half names the position")
	}
	if !(Position{SourceID: &id, CenterID: &id}).Named() {
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
