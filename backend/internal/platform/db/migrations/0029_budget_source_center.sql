-- Budget: where the money comes from and who administers it (docs/10).
--
-- Vendor reference (docs/08-reuse-inventory.md §3): the reference implementation's budget model is
-- budget_position / budget_posting / budget_source / budget_center, with the two join tables
-- budget_center_budget_position and budget_center_budget_source. A *source* (Mittelquelle, a
-- Haushaltsstelle) is where a title's money comes from; a *centre* (Bewirtschaftungsstelle) is who
-- administers it.
--
-- The vendor's two join tables are flattened into two columns on the position: at this size a
-- position draws on one source and is administered by one centre, so a join table would carry
-- nothing a column does not. The mapping from the vendor's shape is data an operator can change
-- (an import fills these columns), not code.

CREATE TABLE IF NOT EXISTS budget_source (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code       text   NOT NULL UNIQUE,
    name       text   NOT NULL,
    -- The Haushaltskapitel the source sits in; the vendor keeps this in
    -- budget_source_k_chapter_igr, which is a chapter per source as well.
    chapter    text   NOT NULL DEFAULT '',
    active     boolean NOT NULL DEFAULT true,
    created_by text   NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS budget_center (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code       text   NOT NULL UNIQUE,
    name       text   NOT NULL,
    active     boolean NOT NULL DEFAULT true,
    created_by text   NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- A position draws on one source and is administered by one centre. Both are optional: a position
-- may exist before its funding is settled, and the Mittelüberwachung does not depend on either.
ALTER TABLE budget_position
    ADD COLUMN IF NOT EXISTS source_id bigint REFERENCES budget_source(id),
    ADD COLUMN IF NOT EXISTS center_id bigint REFERENCES budget_center(id);
