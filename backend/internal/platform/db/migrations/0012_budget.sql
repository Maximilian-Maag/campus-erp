-- Budget (Mittelbewirtschaftung): extend the existing budget_position with the
-- allocated amount and add append-only postings.
--
-- budget_position already exists from the S2 payment-order slice (year, chapter,
-- title, cost_centre, cost_carrier) and is referenced by payment_order. We keep
-- that shape and add what Mittelüberwachung needs.
--
-- Vendor reference (docs/08-reuse-inventory.md §3): budget_position /
-- budget_posting / budget_source / budget_center.

ALTER TABLE budget_position
    ADD COLUMN IF NOT EXISTS description     text        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS allocated_cents bigint      NOT NULL DEFAULT 0 CHECK (allocated_cents >= 0),
    ADD COLUMN IF NOT EXISTS created_by      text        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS created_at      timestamptz NOT NULL DEFAULT now();

-- Append-only. A correction is a compensating posting, never an edit.
CREATE TABLE IF NOT EXISTS budget_posting (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    position_id  bigint NOT NULL REFERENCES budget_position(id),
    kind         text   NOT NULL CHECK (kind IN ('commitment', 'release', 'actual')),
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    reference    text   NOT NULL DEFAULT '',
    booked_by    text   NOT NULL,
    booked_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS budget_posting_position_idx ON budget_posting(position_id);
