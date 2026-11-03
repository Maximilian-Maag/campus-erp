-- The KLR's distribution keys (Verteilerschlüssel) and the runs that use them (docs/13).
--
-- btree_gist is needed so a run's uniqueness can be an exclusion over its *period* and not just
-- over an exact pair of dates: two runs over overlapping windows would distribute the same cost
-- twice.
--
-- The vendor's `overhead_calc*` / `allocation*` shape is not published, so what is kept here is
-- *data* rather than code: a key says how much of one cost centre's costs go to another, and an
-- operator can change it without the engine moving. That is the same reasoning as the
-- per-state statistics layouts (ADR-0012).
--
-- Receivers are cost centres. Distributing to a cost unit (Kostenträger) is a separate step:
-- a record always belongs to a cost centre, so attributing a cost to a unit goes through the
-- Kostenträgerrechnung rather than through this engine (docs/13).

CREATE TABLE IF NOT EXISTS allocation (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_center_id bigint NOT NULL REFERENCES cost_center(id),
    target_center_id bigint NOT NULL REFERENCES cost_center(id),
    -- Basis points: 10000 is all of it, 2500 a quarter.
    share_bp         bigint NOT NULL CHECK (share_bp BETWEEN 0 AND 10000),
    valid_from       date   NOT NULL,
    valid_to         date,
    created_by       text   NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    -- A cost centre does not distribute to itself.
    CONSTRAINT allocation_no_self CHECK (target_center_id <> source_center_id),
    CONSTRAINT allocation_period CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE INDEX IF NOT EXISTS allocation_source_idx ON allocation (source_center_id);

-- One run per source centre and period. The records a run writes carry its reference, and a
-- second run over the same period — or any period overlapping it — is refused: the costs must
-- not be distributed twice. The overlap is a table constraint so that two callers racing each
-- other cannot both pass a check first.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS allocation_run (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_center_id bigint NOT NULL REFERENCES cost_center(id),
    period_from      date   NOT NULL,
    period_to        date   NOT NULL,
    allocated_cents  bigint NOT NULL CHECK (allocated_cents > 0),
    receivers        int    NOT NULL CHECK (receivers >= 0),
    run_by           text   NOT NULL,
    run_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT allocation_run_period CHECK (period_to >= period_from),
    CONSTRAINT allocation_run_no_overlap EXCLUDE USING gist (
        source_center_id WITH =,
        daterange(period_from, period_to, '[]') WITH &&
    )
);

-- A run's records are found by the reference it wrote.
CREATE INDEX IF NOT EXISTS accounting_record_reference_idx
    ON accounting_record (reference) WHERE reference <> '';
