package dbtest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
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

// btreeGistSchema names the schema btree_gist is installed in, or "" when it is not installed at
// all. The *schema* is what matters, not merely that the extension exists: an extension installed
// into a per-process schema is dropped with that schema, and every exclusion constraint that depends
// on it goes with it — in a process that has done nothing wrong (issues #136, #138).
func btreeGistSchema(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var installedIn string
	err := pool.QueryRow(ctx,
		`SELECT n.nspname
		   FROM pg_extension e
		   JOIN pg_namespace n ON n.oid = e.extnamespace
		  WHERE e.extname = 'btree_gist'`).Scan(&installedIn)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read btree_gist's schema: %w", err)
	}
	return installedIn, nil
}

// isPerProcessSchema reports whether a schema name is one this harness creates and therefore drops
// when its process exits. Nothing database-wide — an extension, a role, a tablespace — may live
// there.
func isPerProcessSchema(name string) bool { return strings.HasPrefix(name, "test_") }

// ensureExtension installs btree_gist into `public` before any per-process schema exists, and moves
// one that a killed run left in a per-process schema. Migration 0033 does the same for every other
// consumer; this is here as well because the harness is what creates the short-lived schemas, and a
// test run should not depend on a migration having been applied to a database that already had the
// extension somewhere perishable.
func ensureExtension(ctx context.Context, base *pgxpool.Pool) error {
	installedIn, err := btreeGistSchema(ctx, base)
	if err != nil {
		return err
	}
	switch {
	case installedIn == "":
		if _, err := base.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public`); err != nil {
			return fmt.Errorf("install btree_gist in public: %w", err)
		}
		return nil
	case isPerProcessSchema(installedIn):
		// A per-process schema that still holds the extension belongs to a run that is over; its
		// schema is about to be swept, and the sweep would take the extension from everyone else.
		if _, err := base.Exec(ctx, `ALTER EXTENSION btree_gist SET SCHEMA public`); err != nil {
			return fmt.Errorf("move btree_gist out of %s into public: %w", installedIn, err)
		}
		return nil
	default:
		return nil
	}
}

// verifySchema reports when a named exclusion constraint is absent, saying which and what goes wrong
// without it, because the failure that would otherwise appear is a duplicate accepted somewhere else.
// It also reports whether btree_gist is missing or installed somewhere perishable, which is the usual
// reason for that.
//
// The names are a parameter rather than the package's own list so the message can be tested: a schema
// cannot be made to lose a constraint without editing the migrations, so a test asks about a name no
// schema has and reads what the caller would have been told.
func verifySchema(ctx context.Context, pool *pgxpool.Pool, wanted []string) error {
	missing, err := missingConstraints(ctx, pool, wanted)
	if err != nil {
		return err
	}
	installedIn, err := btreeGistSchema(ctx, pool)
	if err != nil {
		return err
	}
	if len(missing) != 0 {
		return constraintProblem(missing, installedIn)
	}
	// The constraints are here, but if the extension they need lives in a schema that is dropped when
	// a process exits, they will not be here later — in this process, mid-test, and in every other
	// process at once. That is the whole of issues #136 and #138, so it is refused at start-up.
	return extensionProblem(installedIn)
}

// constraintProblem words the refusal. It is separate from the query so both cases can be tested
// without a database that lacks the extension, and they read differently on purpose: one calls for
// the migration to be looked at, the other for the extension to be installed for the role.
func constraintProblem(missing []string, installedIn string) error {
	if installedIn != "" {
		return fmt.Errorf("this schema is missing the exclusion constraint(s) %v, so a duplicate that "+
			"the database must refuse will be accepted and the test that asserts it will fail somewhere "+
			"else entirely; the migrations should have added them (issue #136)", missing)
	}
	return fmt.Errorf("this schema is missing the exclusion constraint(s) %v and the database has no "+
		"btree_gist extension, which is what they need; CREATE EXTENSION btree_gist is not permitted "+
		"for this role, or the migration that installs it did not run (issue #136)", missing)
}

// extensionProblem refuses an extension that is installed somewhere that does not outlive this
// process. It is separate from the query for the same reason as constraintProblem: both the refusal
// and the acceptance read as they should without a database that is set up wrongly.
func extensionProblem(installedIn string) error {
	if installedIn == "" {
		return fmt.Errorf("the database has no btree_gist extension, which the exclusion constraints " +
			"these tests rely on need (issue #136)")
	}
	if !isPerProcessSchema(installedIn) {
		return nil
	}
	return fmt.Errorf("btree_gist is installed in %q, a schema this harness drops when its process "+
		"exits, and DROP SCHEMA CASCADE takes the extension with it — and with the extension every "+
		"exclusion constraint that depends on it, in every other test process at once. A duplicate "+
		"the database must refuse would then be accepted halfway through a run (issues #136, #138). "+
		"The extension belongs in `public`; migration 0033 and dbtest.ensureExtension put it there",
		installedIn)
}
