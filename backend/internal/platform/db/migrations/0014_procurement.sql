-- Canonical schema, procurement (Bestellung).
--
-- Vendor reference (docs/08-reuse-inventory.md §3): purchase_order /
-- purchase_order_line. Amounts are integer cents, quantities milli-units, VAT
-- basis points; the order references a budget_position so approval can commit
-- budget (docs/10-budget.md).

CREATE TABLE IF NOT EXISTS purchase_order (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number             text   NOT NULL UNIQUE,
    supplier           text   NOT NULL,
    budget_position_id bigint NOT NULL REFERENCES budget_position(id),
    currency           text   NOT NULL DEFAULT 'EUR' CHECK (currency ~ '^[A-Z]{3}$'),
    net_cents          bigint NOT NULL CHECK (net_cents   >= 0),
    vat_cents          bigint NOT NULL CHECK (vat_cents   >= 0),
    gross_cents        bigint NOT NULL CHECK (gross_cents >= 0),
    status             text   NOT NULL CHECK (status IN ('draft','approved','received','cancelled')),
    created_by         text   NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    approved_by        text,
    approved_at        timestamptz,
    received_by        text,
    received_at        timestamptz
);

CREATE TABLE IF NOT EXISTS purchase_order_line (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id         bigint NOT NULL REFERENCES purchase_order(id),
    line_no          int    NOT NULL,
    description      text   NOT NULL,
    quantity_milli   bigint NOT NULL CHECK (quantity_milli > 0),
    unit_price_cents bigint NOT NULL CHECK (unit_price_cents >= 0),
    net_cents        bigint NOT NULL CHECK (net_cents >= 0),
    vat_category     text   NOT NULL DEFAULT 'S',
    vat_rate_bp      bigint NOT NULL CHECK (vat_rate_bp BETWEEN 0 AND 10000),
    CONSTRAINT purchase_order_line_key UNIQUE (order_id, line_no)
);
