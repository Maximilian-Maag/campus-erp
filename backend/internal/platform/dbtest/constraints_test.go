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
	withExtension := constraintProblem([]string{"gone_one"}, "public")
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

	withoutExtension := constraintProblem([]string{"gone_one", "gone_two"}, "")
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

// TestExtensionOutlivesTheProcess: the extension the exclusion constraints depend on must not sit in
// a schema this harness drops. That invariant is the whole of issues #136 and #138 — the constraints
// were fine, the extension's schema went away with another process — so it is asserted on the
// database the tests are actually talking to.
func TestExtensionOutlivesTheProcess(t *testing.T) {
	ctx := context.Background()
	pool := Pool(t)

	installedIn, err := btreeGistSchema(ctx, pool)
	if err != nil {
		t.Fatalf("btreeGistSchema: %v", err)
	}
	if installedIn == "" {
		t.Fatal("btree_gist is not installed at all, though the migrations need it")
	}
	if isPerProcessSchema(installedIn) {
		t.Fatalf("btree_gist is installed in %q, a schema this harness drops when the process exits", installedIn)
	}
	if err := extensionProblem(installedIn); err != nil {
		t.Fatalf("extensionProblem refused a schema that outlives the process: %v", err)
	}
}

// TestSchemaDropDoesNotTakeTheConstraints: a sibling process finishing drops its schema, and the
// constraints in this one must still be there afterwards. The dropped schema holds nothing
// database-wide — that is the point of putting the extension in `public`.
func TestSchemaDropDoesNotTakeTheConstraints(t *testing.T) {
	ctx := context.Background()
	pool := Pool(t)

	if _, err := pool.Exec(ctx, "CREATE SCHEMA dbtest_sibling_exit"); err != nil {
		t.Fatalf("CREATE SCHEMA: %v", err)
	}
	if _, err := pool.Exec(ctx, "DROP SCHEMA dbtest_sibling_exit CASCADE"); err != nil {
		t.Fatalf("DROP SCHEMA: %v", err)
	}
	if err := verifySchema(ctx, pool, requiredConstraints); err != nil {
		t.Fatalf("dropping a sibling schema took the exclusion constraints with it: %v", err)
	}
}

// TestExtensionProblemRefusesAPerishableSchema: the refusal has to name the schema it found the
// extension in and the one it belongs in, because "the extension is in the wrong place" without
// either is not something a reader can act on.
func TestExtensionProblemRefusesAPerishableSchema(t *testing.T) {
	if err := extensionProblem("public"); err != nil {
		t.Fatalf("extensionProblem refused where the extension belongs: %v", err)
	}

	perishable := extensionProblem("test_4242")
	if perishable == nil {
		t.Fatal("an extension in a per-process schema was accepted")
	}
	msg := perishable.Error()
	if !strings.Contains(msg, "test_4242") {
		t.Fatalf("the message does not say where the extension is: %q", msg)
	}
	if !strings.Contains(msg, "public") {
		t.Fatalf("the message does not say where it belongs: %q", msg)
	}
	if !strings.Contains(msg, "#136") || !strings.Contains(msg, "#138") {
		t.Fatalf("the message does not point at the issues that explain it: %q", msg)
	}

	absent := extensionProblem("")
	if absent == nil {
		t.Fatal("a database without btree_gist was accepted")
	}
	if !strings.Contains(absent.Error(), "btree_gist") {
		t.Fatalf("the message does not name the missing extension: %q", absent.Error())
	}
}

// TestIsPerProcessSchema: the harness's own naming is the boundary between "dropped when this
// process ends" and "as long as the database exists", so it is asserted rather than assumed.
func TestIsPerProcessSchema(t *testing.T) {
	if !isPerProcessSchema("test_4242") {
		t.Fatal("the harness's own schema name was not recognised as per-process")
	}
	for _, stable := range []string{"public", "tenants", "testing"} {
		if isPerProcessSchema(stable) {
			t.Fatalf("%q was taken for a per-process schema", stable)
		}
	}
}
