-- btree_gist must live in a schema that outlives the process that installed it (issues #136, #138).
--
-- Migration 0022 installs the extension with `CREATE EXTENSION IF NOT EXISTS`, which puts it into
-- whatever schema is current. On a server with the default search_path that is `public`, and every
-- connection sees it for the life of the database. Under the test harness
-- (internal/platform/dbtest) the search_path is the per-process `test_<pid>` schema, so the extension
-- was installed into a schema that is *dropped* when that process exits — and `DROP SCHEMA ...
-- CASCADE` takes the extension with it, and with the extension every exclusion constraint that
-- depends on it, in every other schema. That is how "a second, overlapping allocation run is
-- refused" became `201` in a run that had not touched the engine (#136), and how the schema guard
-- found the constraints missing on a schema that had just been migrated (#138). Both were transient
-- and both passed on a rerun, because they depended on *another* test process exiting in between.
--
-- `public` is the database-wide place for an extension: it is created with the database and dropped
-- with it, so no process can take it away from its siblings. An extension that is already in
-- `public` — or in a schema an operator chose for it — is left where it is; only the perishable case
-- is moved.

DO $$
DECLARE
    installed_in text;
BEGIN
    SELECT n.nspname
      INTO installed_in
      FROM pg_extension e
      JOIN pg_namespace n ON n.oid = e.extnamespace
     WHERE e.extname = 'btree_gist';

    IF installed_in IS NOT NULL
       AND installed_in <> 'public'
       AND EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'public') THEN
        -- `public` may have been dropped on a hardened server; then the extension is left alone
        -- rather than the migration failing on a schema that is not there.
        ALTER EXTENSION btree_gist SET SCHEMA public;
    END IF;
END $$;
