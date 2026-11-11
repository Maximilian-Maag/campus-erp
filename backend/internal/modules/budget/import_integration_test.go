package budget_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/budgetimport"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
)

// budgetFile writes a file in the standard layout for one run: the codes and the Title carry the
// run's suffix, so a second test run against the same database imports a file of its own.
func budgetFile(t *testing.T) (string, string, string, string) {
	s := suffix(t)
	file := strings.Join([]string{
		"source_code;source_name;source_chapter;center_code;center_name;year;chapter;title;description;ansatz",
		"LHH-" + s + ";Landesmittel;05;BW-" + s + ";Fakultät 1;2026;05;A" + s + ";Beschaffung;10.000,00",
		"DFG-" + s + ";Drittmittel;;BW-" + s + ";Fakultät 1;2026;07;B" + s + ";Projekt;2.500,50",
		"", // a trailing blank line is not a row
	}, "\n")
	return file, "LHH-" + s, "DFG-" + s, s
}

// TestImportBudgetFile is the acceptance test of the slice: one file creates the sources, the
// centres and the positions, names each position from its own line, and imports the same file a
// second time as a no-op counted as kept.
func TestImportBudgetFile(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	pool := testPool(t)
	file, sourceCode, otherCode, s := budgetFile(t)

	res, err := svc.Import(ctx, "", file, "alice")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Layout != budgetimport.StandardLayout.ID {
		t.Fatalf("layout = %q, want the standard layout", res.Layout)
	}
	// Both rows name the same Bewirtschaftungsstelle, so it is created once and found once — the
	// count that says the file's second reference did not become a second centre.
	if res.SourcesCreated != 2 || res.CentersCreated != 1 || res.CentersKept != 1 ||
		res.PositionsCreated != 2 || res.PositionsNamed != 2 {
		t.Fatalf("import = %+v, want 2 sources, 1 centre created and 1 kept, 2 positions, 2 named", res)
	}
	if res.SourcesKept != 0 || res.PositionsKept != 0 {
		t.Fatalf("import = %+v, want nothing else kept on a fresh file", res)
	}

	// The file is what is now stored: the names and the Ansatz as the file wrote them, and every
	// position named by its own line.
	sources, err := svc.Sources(ctx)
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	src, ok := findSource(sources, sourceCode)
	if !ok || src.Name != "Landesmittel" || src.Chapter != "05" {
		t.Fatalf("source %s = %+v, want Landesmittel in chapter 05", sourceCode, src)
	}
	other, ok := findSource(sources, otherCode)
	if !ok || other.Chapter != "" {
		t.Fatalf("source %s = %+v, want it without a chapter", otherCode, other)
	}
	pos, ok := findPosition(t, svc, 2026, "05", "A"+s)
	if !ok || !pos.Named() || pos.AllocatedCents != 1000000 || pos.Description != "Beschaffung" {
		t.Fatalf("position = %+v, want it named with the file's Ansatz", pos)
	}
	if pos.SourceID == nil || *pos.SourceID != src.ID {
		t.Fatalf("position source = %v, want the file's source %d", pos.SourceID, src.ID)
	}

	// The same file again: nothing is created twice, and nothing is named twice.
	again, err := svc.Import(ctx, "", file, "alice")
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	// Kept counts the file's references: both rows name the same centre, so it is found twice.
	want := budget.ImportResult{
		Layout: budgetimport.StandardLayout.ID, SourcesKept: 2, CentersKept: 2, PositionsKept: 2,
	}
	if again != want {
		t.Fatalf("second import = %+v, want %+v", again, want)
	}

	// One audit event carries the counts of the run, and the subject names the layout.
	store := audit.NewStore(pool)
	event, ok, err := store.LastEvent(ctx, "budget_import.run", "budget_import/"+budgetimport.StandardLayout.ID)
	if err != nil || !ok {
		t.Fatalf("LastEvent: %v, found = %v", err, ok)
	}
	if event.Actor != "alice" {
		t.Fatalf("audit actor = %q, want alice", event.Actor)
	}
	// The detail carries this run's counts: everything kept, nothing created, nothing named.
	for _, want := range []string{"sources_kept=2", "centers_kept=2", "positions_kept=2", "positions_named=0"} {
		if !strings.Contains(event.After, want) {
			t.Fatalf("audit after = %q, want it to carry %q", event.After, want)
		}
	}
}

// An import creates and names; it never edits what a person has already settled.
func TestImportDoesNotOverwriteWhatIsThere(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	pool := testPool(t)
	file, sourceCode, _, s := budgetFile(t)

	if _, err := svc.Import(ctx, "", file, "alice"); err != nil {
		t.Fatalf("Import: %v", err)
	}
	// A person renames the source and takes the position off its source.
	if _, err := pool.Exec(ctx, `UPDATE budget_source SET name = 'Landesmittel (korrigiert)' WHERE code = $1`, sourceCode); err != nil {
		t.Fatalf("rename the source: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE budget_position SET source_id = NULL, center_id = NULL WHERE title = $1`, "A"+s); err != nil {
		t.Fatalf("unname the position: %v", err)
	}

	res, err := svc.Import(ctx, "", file, "alice")
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if res.SourcesKept != 2 || res.PositionsKept != 2 {
		t.Fatalf("import = %+v, want everything kept", res)
	}
	// The name a person corrected survives; the position is named again, because it names nothing.
	sources, err := svc.Sources(ctx)
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	if src, ok := findSource(sources, sourceCode); !ok || src.Name != "Landesmittel (korrigiert)" {
		t.Fatalf("source = %+v, want the corrected name to survive the import", src)
	}
	if res.PositionsNamed != 1 {
		t.Fatalf("named = %d, want the one position that had no source", res.PositionsNamed)
	}
	pos, ok := findPosition(t, svc, 2026, "05", "A"+s)
	if !ok || !pos.Named() {
		t.Fatalf("position = %+v, want it named again", pos)
	}
}

// A file with one unreadable line imports nothing at all: a half-imported budget is worse than a
// refused one.
func TestImportRefusesTheWholeFile(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	file, sourceCode, _, s := budgetFile(t)
	broken := file + "\nLHH-" + s + "-X;Nur teilweise;05;BW-" + s + ";Fakultät 1;2026;05;C" + s + ";Kaputt;nicht-eine-zahl\n"

	_, err := svc.Import(ctx, "", broken, "alice")
	if !errors.Is(err, budgetimport.ErrBadRow) {
		t.Fatalf("err = %v, want a bad row", err)
	}
	if !strings.Contains(err.Error(), "row 5") {
		t.Fatalf("err = %q, want it to name row 5", err)
	}
	// Nothing of the file was written, not even the rows that were readable.
	sources, err := svc.Sources(ctx)
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	if _, ok := findSource(sources, sourceCode); ok {
		t.Fatalf("source %s was imported although another line was unreadable", sourceCode)
	}
	if _, ok := findPosition(t, svc, 2026, "05", "A"+s); ok {
		t.Fatal("a position was imported although another line was unreadable")
	}
	if _, ok := findPosition(t, svc, 2026, "05", "C"+s); ok {
		t.Fatal("the unreadable row's position was imported")
	}
}

func TestImportRefusals(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	file, _, _, _ := budgetFile(t)

	if _, err := svc.Import(ctx, "vendor-import/v9", file, "alice"); !errors.Is(err, budgetimport.ErrUnknownLayout) {
		t.Fatalf("unknown layout err = %v, want ErrUnknownLayout", err)
	}
	if _, err := svc.Import(ctx, "", "   \n", "alice"); !errors.Is(err, budgetimport.ErrEmpty) {
		t.Fatalf("empty file err = %v, want ErrEmpty", err)
	}
	if _, err := svc.Import(ctx, "", file, "   "); !errors.Is(err, budget.ErrActorRequired) {
		t.Fatalf("blank actor err = %v, want ErrActorRequired", err)
	}
	// A refused import writes nothing and audits nothing.
	store := audit.NewStore(testPool(t))
	if _, ok, _ := store.LastEvent(ctx, "budget_import.run", "budget_import/vendor-import/v9"); ok {
		t.Fatal("a refused import wrote an audit event")
	}
}

func findSource(list []budget.Source, code string) (budget.Source, bool) {
	for _, s := range list {
		if s.Code == code {
			return s, true
		}
	}
	return budget.Source{}, false
}

func findPosition(t *testing.T, svc *budget.PositionService, year int, chapter, title string) (budget.Position, bool) {
	t.Helper()
	positions, err := svc.Positions(context.Background())
	if err != nil {
		t.Fatalf("Positions: %v", err)
	}
	for _, p := range positions {
		if p.Year == year && p.Chapter == chapter && p.Title == title {
			return p, true
		}
	}
	return budget.Position{}, false
}
