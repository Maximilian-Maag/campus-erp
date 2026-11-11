package etlwiring_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/cmd/etlwiring"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/lookup"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/dbtest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

// TestDatasetsHoldsBothCollections runs the list a runtime builds at start-up, so a dataset that is
// only nameable is not mistaken for one that is runnable: every entry must have a name, a source
// and a sink, and the four the cash-book module owns must really be in it.
func TestDatasetsHoldsBothCollections(t *testing.T) {
	pool := dbtest.Pool(t)
	store := audit.NewStore(pool)
	datasets := etlwiring.Datasets(
		pool,
		lookup.NewValueService(lookup.NewValueRepo(pool), store),
		kasse.NewTaxService(kasse.NewTaxRepo(pool), store),
		kasse.NewService(kasse.NewRepo(pool), store),
	)

	got := make([]string, 0, len(datasets))
	for _, ds := range datasets {
		if ds.Name == "" {
			t.Fatal("a dataset has no name, so the registry would drop it")
		}
		if ds.Source == nil {
			t.Fatalf("%s has no source, so a run would read nothing", ds.Name)
		}
		if ds.Sink == nil {
			t.Fatalf("%s has no sink, so a run would import nothing", ds.Name)
		}
		got = append(got, ds.Name)
	}
	sort.Strings(got)
	want := []string{
		"kasse/counter-book", "kasse/foundation", "kasse/tax-office", "kasse/vat",
		"lookup/bank_leit_zahl", "lookup/krankenkasse",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("the registry would hold %v, want %v", got, want)
	}
}

// TestBothCommandsUseTheAssembly guards the last hop, which no test can execute: that each command
// calls the assembly above. It is deliberately a source check — a missing call cannot be observed
// from inside the process that lacks it, which is how these datasets were unreachable for several
// slices while every test passed.
func TestBothCommandsUseTheAssembly(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "api", "main.go"),
		filepath.Join("..", "worker", "main.go"),
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(src)
		if !strings.Contains(text, "etlwiring.Datasets(") {
			t.Errorf("%s does not call etlwiring.Datasets: the module's ETL datasets are "+
				"unreachable from this command", path)
		}
		if !strings.Contains(text, "etl.NewRegistry(") {
			t.Errorf("%s has no ETL registry at all", path)
		}
	}
}
