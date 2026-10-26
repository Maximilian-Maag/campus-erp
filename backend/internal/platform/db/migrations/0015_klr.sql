-- Canonical schema, KLR (Kosten- und Leistungsrechnung).
--
-- Vendor reference (docs/08-reuse-inventory.md §3): cost_center / cost_unit /
-- cost_type_bridge / accounting_record and the clearing tables. Legacy side: the
-- cob schema (docs/03 §3). Core kept here: cost centres, cost units (Kostenträger)
-- and append-only accounting records.

CREATE TABLE IF NOT EXISTS cost_center (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number     text   NOT NULL UNIQUE,
    name       text   NOT NULL,
    created_by text   NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cost_unit (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number     text   NOT NULL UNIQUE,
    name       text   NOT NULL,
    created_by text   NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Append-only. A correction is a compensating record.
CREATE TABLE IF NOT EXISTS accounting_record (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cost_center_id bigint NOT NULL REFERENCES cost_center(id),
    cost_unit_id   bigint REFERENCES cost_unit(id),
    kind           text   NOT NULL CHECK (kind IN ('cost', 'revenue')),
    amount_cents   bigint NOT NULL CHECK (amount_cents > 0),
    reference      text   NOT NULL DEFAULT '',
    booked_by      text   NOT NULL,
    booked_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS accounting_record_cost_center_idx ON accounting_record (cost_center_id);
