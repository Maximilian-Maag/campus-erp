package interfaces

import (
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/golden"
)

func TestStandardRegistryIsValid(t *testing.T) {
	r := Standard()
	if len(r.All()) < 10 {
		t.Fatalf("expected a populated registry, got %d", len(r.All()))
	}
	// The registry is sorted by key.
	for i := 1; i < len(r.All()); i++ {
		if r.All()[i-1].Key > r.All()[i].Key {
			t.Fatalf("registry not sorted at %d", i)
		}
	}
	if got := r.Count(Implemented); got != 7 {
		t.Errorf("implemented = %d, want 7", got)
	}
	if r.Count(Planned) == 0 {
		t.Error("expected planned interfaces")
	}
}

func TestGet(t *testing.T) {
	r := Standard()
	if it, ok := r.Get("bank-sepa-sct"); !ok || it.Status != Implemented {
		t.Errorf("bank-sepa-sct = %+v, %v", it, ok)
	}
	if _, ok := r.Get("nope"); ok {
		t.Error("unknown key should not resolve")
	}
}

func TestRenderIsPinned(t *testing.T) {
	golden.Assert(t, "testdata/interfaces.txt", []byte(Standard().Render()))
}

func TestNewRejectsInvalid(t *testing.T) {
	base := Interface{Key: "k", Requirement: "IR-1", Name: "n", Format: "f", Direction: Out, Status: Planned}
	cases := map[string]Interface{
		"empty key":     {Requirement: "IR-1", Name: "n", Format: "f", Direction: Out, Status: Planned},
		"empty name":    {Key: "k", Requirement: "IR-1", Format: "f", Direction: Out, Status: Planned},
		"empty req":     {Key: "k", Name: "n", Format: "f", Direction: Out, Status: Planned},
		"empty format":  {Key: "k", Requirement: "IR-1", Name: "n", Direction: Out, Status: Planned},
		"bad direction": {Key: "k", Requirement: "IR-1", Name: "n", Format: "f", Direction: "sideways", Status: Planned},
		"bad status":    {Key: "k", Requirement: "IR-1", Name: "n", Format: "f", Direction: Out, Status: "maybe"},
	}
	for name, it := range cases {
		if _, err := New([]Interface{it}); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := New([]Interface{base, base}); err == nil {
		t.Error("duplicate key: expected error")
	}
	if _, err := New([]Interface{base}); err != nil {
		t.Errorf("valid entry rejected: %v", err)
	}
}
