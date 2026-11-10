package budget_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
)

// TestBudgetSourceAndCenter is the acceptance test of the slice: a Mittelquelle and a
// Bewirtschaftungsstelle are stored and listed, a position is assigned both, and every refusal
// leaves the position as it was.
func TestBudgetSourceAndCenter(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	pool := testPool(t)

	src, err := svc.CreateSource(ctx, "LHH-"+suffix(t), "Landesmittel", "05", "alice")
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	if src.ID == 0 || !src.Active || src.Chapter != "05" || src.CreatedBy != "alice" {
		t.Fatalf("source = %+v", src)
	}
	center, err := svc.CreateCenter(ctx, "BW-"+suffix(t), "Fakultät 1", "alice")
	if err != nil {
		t.Fatalf("CreateCenter: %v", err)
	}
	if center.ID == 0 || !center.Active {
		t.Fatalf("centre = %+v", center)
	}

	// Both reads list what was stored, in code order.
	sources, err := svc.Sources(ctx)
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	centers, err := svc.Centers(ctx)
	if err != nil {
		t.Fatalf("Centers: %v", err)
	}
	if !containsSource(sources, src.ID) || !containsCenter(centers, center.ID) {
		t.Fatalf("the stored source/centre is missing from %d/%d rows", len(sources), len(centers))
	}
	for i := 1; i < len(sources); i++ {
		if sources[i-1].Code > sources[i].Code {
			t.Fatalf("sources are not in code order: %q before %q", sources[i-1].Code, sources[i].Code)
		}
	}

	// A position, unassigned first, then named.
	pos, err := svc.CreatePosition(ctx, 2026, "05", "T-"+suffix(t), "Beschaffung", 100000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	if pos.Named() || pos.SourceID != nil || pos.CenterID != nil {
		t.Fatalf("a new position must be unassigned: %+v", pos)
	}
	assigned, err := svc.AssignPosition(ctx, pos.ID, &src.ID, &center.ID, "alice")
	if err != nil {
		t.Fatalf("AssignPosition: %v", err)
	}
	if assigned.SourceID == nil || *assigned.SourceID != src.ID || assigned.CenterID == nil || *assigned.CenterID != center.ID {
		t.Fatalf("assigned = %+v", assigned)
	}
	if !assigned.Named() {
		t.Fatal("an assigned position is named")
	}
	// The assignment is stored, not just returned.
	loaded, err := svc.GetPosition(ctx, pos.ID)
	if err != nil {
		t.Fatalf("GetPosition: %v", err)
	}
	if loaded.SourceID == nil || *loaded.SourceID != src.ID || loaded.CenterID == nil || *loaded.CenterID != center.ID {
		t.Fatalf("loaded = %+v", loaded)
	}
	// It is also in the list the Mittelübersicht reads.
	list, err := svc.Positions(ctx)
	if err != nil {
		t.Fatalf("Positions: %v", err)
	}
	for _, p := range list {
		if p.ID == pos.ID && (p.SourceID == nil || *p.SourceID != src.ID) {
			t.Fatalf("listed position = %+v", p)
		}
	}

	// Clearing one half leaves the other, which is what "one operation" means.
	cleared, err := svc.AssignPosition(ctx, pos.ID, nil, &center.ID, "alice")
	if err != nil {
		t.Fatalf("AssignPosition (clear the source): %v", err)
	}
	if cleared.SourceID != nil || cleared.CenterID == nil || *cleared.CenterID != center.ID {
		t.Fatalf("cleared = %+v", cleared)
	}

	// Every refusal: a duplicate code, an unknown source, an unknown centre, an unknown position,
	// and a blank actor. None of them changes the position.
	unknown := int64(9223372036854775807)
	if _, err := svc.CreateSource(ctx, src.Code, "Andere", "", "alice"); !errors.Is(err, budget.ErrSourceExists) {
		t.Fatalf("duplicate source code = %v, want ErrSourceExists", err)
	}
	if _, err := svc.CreateCenter(ctx, center.Code, "Andere", "alice"); !errors.Is(err, budget.ErrCenterExists) {
		t.Fatalf("duplicate centre code = %v, want ErrCenterExists", err)
	}
	if _, err := svc.CreateSource(ctx, "", "Ohne Code", "", "alice"); !errors.Is(err, budget.ErrSourceCodeRequired) {
		t.Fatalf("blank code = %v, want ErrSourceCodeRequired", err)
	}
	if _, err := svc.CreateSource(ctx, "X", "X", "", "  "); !errors.Is(err, budget.ErrActorRequired) {
		t.Fatalf("blank actor = %v, want ErrActorRequired", err)
	}
	if _, err := svc.CreateCenter(ctx, "Y", "Y", "  "); !errors.Is(err, budget.ErrActorRequired) {
		t.Fatalf("blank actor = %v, want ErrActorRequired", err)
	}
	if _, err := svc.AssignPosition(ctx, pos.ID, &unknown, nil, "alice"); !errors.Is(err, budget.ErrAssignmentUnknown) {
		t.Fatalf("unknown source = %v, want ErrAssignmentUnknown", err)
	}
	if _, err := svc.AssignPosition(ctx, pos.ID, nil, &unknown, "alice"); !errors.Is(err, budget.ErrAssignmentUnknown) {
		t.Fatalf("unknown centre = %v, want ErrAssignmentUnknown", err)
	}
	if _, err := svc.AssignPosition(ctx, unknown, &src.ID, nil, "alice"); !errors.Is(err, budget.ErrPositionNotFound) {
		t.Fatalf("unknown position = %v, want ErrPositionNotFound", err)
	}
	if _, err := svc.AssignPosition(ctx, pos.ID, &src.ID, nil, "  "); !errors.Is(err, budget.ErrActorRequired) {
		t.Fatalf("blank actor = %v, want ErrActorRequired", err)
	}
	after, err := svc.GetPosition(ctx, pos.ID)
	if err != nil {
		t.Fatalf("GetPosition: %v", err)
	}
	if after.SourceID != nil || after.CenterID == nil || *after.CenterID != center.ID {
		t.Fatalf("a refused assignment changed the position: %+v", after)
	}

	if err := audit.NewStore(pool).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func containsSource(list []budget.Source, id int64) bool {
	for _, s := range list {
		if s.ID == id {
			return true
		}
	}
	return false
}

func containsCenter(list []budget.Center, id int64) bool {
	for _, c := range list {
		if c.ID == id {
			return true
		}
	}
	return false
}
