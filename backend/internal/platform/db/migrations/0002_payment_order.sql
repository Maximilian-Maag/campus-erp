-- Canonical schema, slice S2 (Kasse / payment orders, "Anordnungen").
--
-- Legacy background (kbs.ao / kbs.aoar / kbs.zv_freigabe):
--   * `ao` is a ~120-column god table mixing identity, accounting dimensions,
--     recipient address, bank details, SEPA fields, dunning flags and 16 free-text
--     lines (grund1..grund16, each 27 chars).
--   * `zv_freigabe` is a VERBATIM CLONE of `ao` with a few extra columns; the
--     four-eyes release works by COPYING the row. Nothing records who released
--     what, when, or why.
--   * There are no foreign keys anywhere in the legacy schema.
--
-- The canonical model separates these concerns and makes the approval an explicit,
-- auditable decision.

CREATE TABLE IF NOT EXISTS payment_recipient (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name           text NOT NULL,
    account_holder text NOT NULL,
    iban           text NOT NULL,
    bic            text NOT NULL DEFAULT '',
    bank_name      text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT payment_recipient_iban_key UNIQUE (iban)
);

-- The accounting dimension (legacy: kap/tit/dr/ds_nr/ut/fb/ins/pers/huel/kostelle/
-- kotraeger flattened onto every order row).
CREATE TABLE IF NOT EXISTS budget_position (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    year         int  NOT NULL CHECK (year BETWEEN 1900 AND 2999),
    chapter      text NOT NULL,
    title        text NOT NULL,
    cost_centre  text NOT NULL DEFAULT '',
    cost_carrier text NOT NULL DEFAULT '',
    CONSTRAINT budget_position_key UNIQUE (year, chapter, title, cost_centre, cost_carrier)
);

CREATE TABLE IF NOT EXISTS payment_order (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    reference          text   NOT NULL UNIQUE,
    business_year      int    NOT NULL CHECK (business_year BETWEEN 1900 AND 2999),
    amount_cents       bigint NOT NULL CHECK (amount_cents > 0),
    currency           text   NOT NULL DEFAULT 'EUR' CHECK (currency ~ '^[A-Z]{3}$'),
    recipient_id       bigint NOT NULL REFERENCES payment_recipient(id),
    budget_position_id bigint NOT NULL REFERENCES budget_position(id),
    purpose            text   NOT NULL,
    due_date           date   NOT NULL,
    status             text   NOT NULL CHECK (status IN ('draft','submitted','approved','rejected','cancelled')),
    created_by         text   NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    submitted_by       text,
    submitted_at       timestamptz,
    decided_by         text,
    decided_at         timestamptz,
    decision_reason    text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS payment_order_status_idx ON payment_order(status);

-- Append-only record of every workflow decision. This is the piece the legacy
-- clone-based approval never had.
CREATE TABLE IF NOT EXISTS payment_order_decision (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id bigint NOT NULL REFERENCES payment_order(id),
    decision text   NOT NULL CHECK (decision IN ('submitted','approved','rejected','cancelled')),
    actor    text   NOT NULL,
    reason   text   NOT NULL DEFAULT '',
    at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS payment_order_decision_order_idx ON payment_order_decision(order_id);
