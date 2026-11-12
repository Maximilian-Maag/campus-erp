package kasse_test

import (
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// TestStandardDatasetsAreShapedForTheRegistry runs the module's datasets the way a runtime gets
// them: built by StandardDatasets, registered under a name, and runnable by that name.
//
// This is the guard for a failure that is invisible from a unit test of a source and its sink: the
// four datasets this module owns were implemented and integration-tested long before they were
// *registered*, so the registry never held them and every run of them answered "unknown dataset".
// A dataset that no runtime registers is not wired, however well its mapping is tested.
func TestStandardDatasetsAreShapedForTheRegistry(t *testing.T) {
	pool := testPool(t)
	store := audit.NewStore(pool)
	datasets := kasse.StandardDatasets(pool, kasse.NewTaxService(kasse.NewTaxRepo(pool), store), kasse.NewService(kasse.NewRepo(pool), store))

	want := []string{"kasse/counter-book", "kasse/foundation", "kasse/tax-office", "kasse/vat"}
	if len(datasets) != len(want) {
		t.Fatalf("StandardDatasets returned %d datasets, want %d", len(datasets), len(want))
	}
	seen := map[string]bool{}
	for _, ds := range datasets {
		if ds.Name == "" {
			t.Fatal("a dataset has no name, so the registry would drop it")
		}
		if ds.Source == nil {
			t.Fatalf("%s has no source", ds.Name)
		}
		if ds.Sink == nil {
			t.Fatalf("%s has no sink, so a run would import nothing", ds.Name)
		}
		if seen[ds.Name] {
			t.Fatalf("%s is registered twice; the second would silently replace the first", ds.Name)
		}
		seen[ds.Name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Fatalf("%s is missing from StandardDatasets: it would be unreachable from the API", name)
		}
	}

	// A registry built from them answers by name — the property the HTTP layer relies on when it
	// lists the datasets and runs one (docs/19).
	registry := etl.NewRegistry(etl.NewRunner(store).WithLedger(etl.NewAuditLedger(store)), datasets...)
	names := registry.Names()
	for _, name := range want {
		found := false
		for _, got := range names {
			if got == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("the registry does not list %s (it lists %v)", name, names)
		}
	}
}
