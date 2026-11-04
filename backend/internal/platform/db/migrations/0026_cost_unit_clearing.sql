-- The KLR's Kostenträgerrechnung: the keys that charge a cost centre's costs to its cost units
-- (Kostenträger), and the runs that use them (docs/13).
--
-- docs/03 reads the legacy `kpum` / `um*` tables as "Umlage/Verrechnung" — the same apportionment
-- the centre-side engine already does — so the vendor's `cost_unit_clearing` is taken to be its
-- unit-side half: the receiver is a cost unit rather than another cost centre. As with
-- `allocation`, the vendor's shape is not published, so what is kept here is *data* an operator
-- can change without the engine moving.
--
-- A run writes a cost on the source centre tagged with the unit that takes it, and relieves the
-- source by the same amount. The source centre's Saldo is therefore unchanged while the unit now
-- carries the cost — the same rule as the centre-side engine, where a run moves cost without
-- creating or destroying any.

CREATE TABLE IF NOT EXISTS cost_unit_key (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_center_id bigint NOT NULL REFERENCES cost_center(id),
    target_unit_id   bigint NOT NULL REFERENCES cost_unit(id),
    -- Basis points: 10000 is all of it, 2500 a quarter.
    share_bp         bigint NOT NULL CHECK (share_bp BETWEEN 0 AND 10000),
    valid_from       date   NOT NULL,
    valid_to         date,
    created_by       text   NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cost_unit_key_period CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE INDEX IF NOT EXISTS cost_unit_key_source_idx ON cost_unit_key (source_center_id);

-- One run per source centre and period, as on the centre side: the costs must not be attributed
-- to the units twice. btree_gist is already installed by 0022.
CREATE TABLE IF NOT EXISTS cost_unit_run (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_center_id bigint NOT NULL REFERENCES cost_center(id),
    period_from      date   NOT NULL,
    period_to        date   NOT NULL,
    allocated_cents  bigint NOT NULL CHECK (allocated_cents > 0),
    receivers        int    NOT NULL CHECK (receivers >= 0),
    run_by           text   NOT NULL,
    run_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cost_unit_run_period CHECK (period_to >= period_from),
    CONSTRAINT cost_unit_run_no_overlap EXCLUDE USING gist (
        source_center_id WITH =,
        daterange(period_from, period_to, '[]') WITH &&
    )
);
