-- Canonical schema, slice S3 (Kasse / payment run + SEPA export, "Zahlungslauf").
--
-- Legacy background (kbs.zawe / kbs.zabe / kbs.zv_freigabe):
--   * `zawe` groups payments per bank account and carries the running totals as
--     ten numeric pairs (e_ist/a_ist/e_soll/a_soll/... income/expense x
--     actual/target/in-flight), plus `art`, `sperre`, `stap_nr`, `export_format`.
--   * The debtor account is denormalised onto every payment row (blz/konto/iban/
--     bic/inhaber/bname/waehrung); there is no bank-account entity.
--   * The produced bank file is not stored; only the aggregate counters survive,
--     so a re-run cannot be proven byte-identical and double payment cannot be
--     ruled out from the database alone.
--
-- The canonical model introduces the missing bank-account entity, an explicit
-- payment run that owns the produced file, and a per-order uniqueness constraint
-- so an approved order can be paid at most once.
--
-- Format: SEPA credit transfer, ISO 20022 pain.001.001.09 (DK/GBIC-compatible),
-- matching the productive HISinOne writer
-- (rm.fia.payments.datex.parser.sepa.SEPAWriterCreditTransferVersion39). The
-- format is legally binding (ADR-0001 interface parity, IR-2).

-- The university's own bank accounts (the debtor side of an export).
CREATE TABLE IF NOT EXISTS bank_account (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       text NOT NULL,
    iban       text NOT NULL,
    bic        text NOT NULL DEFAULT '',
    currency   text NOT NULL DEFAULT 'EUR' CHECK (currency ~ '^[A-Z]{3}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT bank_account_iban_key UNIQUE (iban)
);

-- One payment run = one produced bank file for one debtor account.
-- `file_xml` and `file_hash` make the run reproducible and auditable: the exact
-- bytes that were handed to the bank are stored, so a re-run returns the same
-- file instead of regenerating a different one.
CREATE TABLE IF NOT EXISTS payment_run (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    reference      text   NOT NULL UNIQUE,
    account_id     bigint NOT NULL REFERENCES bank_account(id),
    execution_date date   NOT NULL,
    message_id     text   NOT NULL,
    item_count     int    NOT NULL CHECK (item_count > 0),
    sum_cents      bigint NOT NULL CHECK (sum_cents > 0),
    file_xml       text   NOT NULL,
    file_hash      text   NOT NULL,
    created_by     text   NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- The orders a run pays. UNIQUE(order_id) is the double-payment guard: an
-- approved order belongs to at most one run.
CREATE TABLE IF NOT EXISTS payment_run_item (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id        bigint NOT NULL REFERENCES payment_run(id),
    order_id      bigint NOT NULL REFERENCES payment_order(id),
    end_to_end_id text   NOT NULL,
    amount_cents  bigint NOT NULL CHECK (amount_cents > 0),
    CONSTRAINT payment_run_item_order_key UNIQUE (order_id)
);

CREATE INDEX IF NOT EXISTS payment_run_item_run_idx ON payment_run_item(run_id);
