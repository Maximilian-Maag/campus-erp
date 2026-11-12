package dbtest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// requiredConstraints are the exclusions whose *absence* changes behaviour silently rather than
// loudly: each refuses a second, overlapping run, and every test that asserts "a duplicate is
// refused" would instead see the duplicate accepted — as an unrelated assertion about a status code,
// in a package that has nothing to do with the missing constraint.
//
// The constraints are added by the migrations and need the `btree_gist` extension, which is
// database-wide while the test schemas are per process. If a migration that adds one of these is
// skipped or fails quietly, nothing else notices; this does. See issue #136.
var requiredConstraints = []string{
	"allocation_run_no_overlap",
	"cost_unit_run_no_overlap",
}

// missingConstraints reports which of the named constraints the current schema does not have. It is
// a function rather than a call site so its two answers can be tested (see the integration test) and
// so a caller can ask about a name that should not exist.
func missingConstraints(ctx context.Context, pool *pgxpool.Pool, wanted []string) ([]string, error) {
	rows, err := pool.Query(ctx,
		`SELECT c.conname
		   FROM pg_constraint c
		   JOIN pg_class t ON t.oid = c.conrelid
		   JOIN pg_namespace n ON n.oid = t.relnamespace
		  WHERE n.nspname = current_schema()
		    AND c.contype = 'x'
		    AND c.conname = ANY($1)`, wanted)
	if err != nil {
		return nil, fmt.Errorf("read constraints: %w", err)
	}
	defer rows.Close()
	present := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan constraint: %w", err)
		}
		present[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read constraints: %w", err)
	}
	missing := make([]string, 0, len(wanted))
	for _, name := range wanted {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	return missing, nil
}

// verifySchema reports when a named exclusion constraint is absent, saying which and what goes wrong
// without it, because the failure that would otherwise appear is a duplicate accepted somewhere else.
// It also reports whether btree_gist is missing, which is the usual reason for that.
//
// The names are a parameter rather than the package's own list so the message can be tested: a schema
// cannot be made to lose a constraint without editing the migrations, so a test asks about a name no
// schema has and reads what the caller would have been told.
func verifySchema(ctx context.Context, pool *pgxpool.Pool, wanted []string) error {
	missing, err := missingConstraints(ctx, pool, wanted)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	var hasExtension bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'btree_gist')`).Scan(&hasExtension); err != nil {
		return fmt.Errorf("read extensions: %w", err)
	}
	return constraintProblem(missing, hasExtension)
}

// constraintProblem words the refusal. It is separate from the query so both cases can be tested
// without a database that lacks the extension, and they read differently on purpose: one calls for
// the migration to be looked at, the other for the extension to be installed for the role.
func constraintProblem(missing []string, hasBtreeGist bool) error {
	if hasBtreeGist {
		return fmt.Errorf("this schema is missing the exclusion constraint(s) %v, so a duplicate that "+
			"the database must refuse will be accepted and the test that asserts it will fail somewhere "+
			"else entirely; the migrations should have added them (issue #136)", missing)
	}
	return fmt.Errorf("this schema is missing the exclusion constraint(s) %v and the database has no "+
		"btree_gist extension, which is what they need; CREATE EXTENSION btree_gist is not permitted "+
		"for this role, or the migration that installs it did not run (issue #136)", missing)
}
