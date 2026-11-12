package dbtest

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) { os.Exit(Main(m)) }

// TestMigratedSchemaHasTheRequiredExclusions is the guard the flake in issue #136 asked for: the
// constraints the tests rely on being there is asserted before any test that relies on them runs, so
// a schema that lost one says so here rather than as a duplicate accepted in an unrelated package.
func TestMigratedSchemaHasTheRequiredExclusions(t *testing.T) {
	ctx := context.Background()
	pool := Pool(t)

	missing, err := missingConstraints(ctx, pool, requiredConstraints)
	if err != nil {
		t.Fatalf("missingConstraints: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("the migrated schema is missing %v", missing)
	}
	if err := verifySchema(ctx, pool, requiredConstraints); err != nil {
		t.Fatalf("verifySchema on a migrated schema: %v", err)
	}
}

// TestMissingConstraintsNamesWhatIsAbsent: the answer has to name what is gone, because "a
// constraint is missing" without which one is not something a reader can act on.
func TestMissingConstraintsNamesWhatIsAbsent(t *testing.T) {
	ctx := context.Background()
	pool := Pool(t)

	wanted := append([]string{"no_such_constraint_here"}, requiredConstraints...)
	missing, err := missingConstraints(ctx, pool, wanted)
	if err != nil {
		t.Fatalf("missingConstraints: %v", err)
	}
	if len(missing) != 1 || missing[0] != "no_such_constraint_here" {
		t.Fatalf("missing = %v, want exactly the name that does not exist", missing)
	}

	// An empty list asks about nothing and misses nothing, so the walk over the names is exercised
	// on its empty path too.
	none, err := missingConstraints(ctx, pool, nil)
	if err != nil {
		t.Fatalf("missingConstraints(nil): %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("missing(nil) = %v, want none", none)
	}
}

// TestConstraintProblemWordsBothCases: the message is the whole point of the check, and the two cases
// ask the reader to do different things — look at the migrations, or get the extension installed for
// the role — so both are asserted here, without needing a database that lacks btree_gist.
func TestConstraintProblemWordsBothCases(t *testing.T) {
	withExtension := constraintProblem([]string{"gone_one"}, true)
	if withExtension == nil {
		t.Fatal("a missing constraint with the extension present was not reported")
	}
	msg := withExtension.Error()
	if !strings.Contains(msg, "gone_one") {
		t.Fatalf("the message does not name the missing constraint: %q", msg)
	}
	if strings.Contains(msg, "btree_gist") {
		t.Fatalf("the message blames the extension although it is installed: %q", msg)
	}
	if !strings.Contains(msg, "#136") {
		t.Fatalf("the message does not point at the issue that explains it: %q", msg)
	}

	withoutExtension := constraintProblem([]string{"gone_one", "gone_two"}, false)
	if withoutExtension == nil {
		t.Fatal("a missing constraint without the extension was not reported")
	}
	msg = withoutExtension.Error()
	if !strings.Contains(msg, "btree_gist") {
		t.Fatalf("the message does not mention the extension, which is the likely cause: %q", msg)
	}
	if !strings.Contains(msg, "gone_one") || !strings.Contains(msg, "gone_two") {
		t.Fatalf("the message does not name every missing constraint: %q", msg)
	}
}

// TestVerifySchemaSaysWhichConstraintAndWhy: the message is the whole point of the check — it has to
// name the constraint, say what goes wrong without it, and mention the extension when that is why.
// Simulated by asking about a name no schema has, since a schema cannot be made to lose a constraint
// without editing the migrations.
func TestVerifySchemaSaysWhichConstraintAndWhy(t *testing.T) {
	ctx := context.Background()
	pool := Pool(t)

	err := verifySchema(ctx, pool, []string{"allocation_run_no_overlap", "gone_one"})
	if err == nil {
		t.Fatal("verifySchema accepted a schema without a named constraint")
	}
	msg := err.Error()
	if !strings.Contains(msg, "gone_one") {
		t.Fatalf("the message does not name the missing constraint: %q", msg)
	}
	if !strings.Contains(msg, "#136") {
		t.Fatalf("the message does not point at the issue that explains it: %q", msg)
	}
	if strings.Contains(msg, "allocation_run_no_overlap") {
		t.Fatalf("the message names a constraint that *is* there: %q", msg)
	}
	// And a schema that has everything asked of it is accepted, so the check is not simply always red.
	if err := verifySchema(ctx, pool, []string{"allocation_run_no_overlap"}); err != nil {
		t.Fatalf("verifySchema refused a constraint that is present: %v", err)
	}
}
