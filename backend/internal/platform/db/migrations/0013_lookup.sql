-- Canonical lookup catalogue.
--
-- The productive source has 3,532 separate key tables (one per coded value set,
-- docs/08-reuse-inventory.md §5). A better implementation keeps one catalogue
-- keyed by (namespace, code); a new value set is rows, not a table.

CREATE TABLE IF NOT EXISTS lookup_value (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    namespace  text    NOT NULL,
    code       text    NOT NULL,
    short_text text    NOT NULL,
    long_text  text    NOT NULL DEFAULT '',
    sort_order int     NOT NULL DEFAULT 0,
    active     boolean NOT NULL DEFAULT true,
    CONSTRAINT lookup_value_key UNIQUE (namespace, code)
);

CREATE INDEX IF NOT EXISTS lookup_value_namespace_idx ON lookup_value (namespace);
