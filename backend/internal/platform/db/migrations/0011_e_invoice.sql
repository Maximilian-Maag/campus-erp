-- Canonical schema, e-invoicing (Ausgangs-/Eingangsrechnung).
--
-- Vendor reference (docs/08-reuse-inventory.md §4): the productive source models
-- an e-invoice as EN16931 business groups (einvoice_metadata/seller/buyer/line/
-- vat_breakdown/document_total/payment_instructions). We keep the same shape but
-- with our conventions: integer cents, basis-point VAT rates, milli-unit
-- quantities, declared FKs and enum checks (ADR-0001/0002).
--
-- The emitted document is XRechnung 3.0 (UBL 2.1), the format a German public
-- body must accept; code lists are UNTDID 1001 (type), 4461 (payment means) and
-- 5305 (VAT category).

CREATE TABLE IF NOT EXISTS e_invoice (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number             text   NOT NULL UNIQUE,
    issue_date         date   NOT NULL,
    type_code          text   NOT NULL DEFAULT '380',  -- UNTDID 1001 (380 = invoice)
    currency           text   NOT NULL DEFAULT 'EUR' CHECK (currency ~ '^[A-Z]{3}$'),
    buyer_reference    text   NOT NULL DEFAULT '',
    payment_means_code text   NOT NULL DEFAULT '58',   -- UNTDID 4461 (58 = SEPA credit transfer)
    payee_iban         text   NOT NULL DEFAULT '',
    payment_terms      text   NOT NULL DEFAULT '',
    net_cents          bigint NOT NULL CHECK (net_cents   >= 0),
    vat_cents          bigint NOT NULL CHECK (vat_cents   >= 0),
    gross_cents        bigint NOT NULL CHECK (gross_cents >= 0),
    created_by         text   NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS e_invoice_party (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    invoice_id      bigint NOT NULL REFERENCES e_invoice(id),
    role            text   NOT NULL CHECK (role IN ('seller', 'buyer')),
    name            text   NOT NULL,
    street          text   NOT NULL DEFAULT '',
    city            text   NOT NULL DEFAULT '',
    postal_code     text   NOT NULL DEFAULT '',
    country         text   NOT NULL DEFAULT 'DE' CHECK (country ~ '^[A-Z]{2}$'),
    vat_id          text   NOT NULL DEFAULT '',
    endpoint_id     text   NOT NULL DEFAULT '',
    endpoint_scheme text   NOT NULL DEFAULT 'EM',
    email           text   NOT NULL DEFAULT '',
    CONSTRAINT e_invoice_party_key UNIQUE (invoice_id, role)
);

CREATE TABLE IF NOT EXISTS e_invoice_line (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    invoice_id       bigint NOT NULL REFERENCES e_invoice(id),
    line_no          int    NOT NULL,
    description      text   NOT NULL,
    quantity_milli   bigint NOT NULL CHECK (quantity_milli > 0),
    unit_code        text   NOT NULL DEFAULT 'C62',  -- UNECE (C62 = one)
    unit_price_cents bigint NOT NULL CHECK (unit_price_cents >= 0),
    net_cents        bigint NOT NULL CHECK (net_cents >= 0),
    vat_category     text   NOT NULL DEFAULT 'S',    -- UNTDID 5305 (S = standard)
    vat_rate_bp      bigint NOT NULL CHECK (vat_rate_bp BETWEEN 0 AND 10000),
    CONSTRAINT e_invoice_line_key UNIQUE (invoice_id, line_no)
);

CREATE TABLE IF NOT EXISTS e_invoice_vat (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    invoice_id    bigint NOT NULL REFERENCES e_invoice(id),
    category      text   NOT NULL,
    rate_bp       bigint NOT NULL CHECK (rate_bp BETWEEN 0 AND 10000),
    taxable_cents bigint NOT NULL CHECK (taxable_cents >= 0),
    tax_cents     bigint NOT NULL CHECK (tax_cents >= 0),
    CONSTRAINT e_invoice_vat_key UNIQUE (invoice_id, category, rate_bp)
);
