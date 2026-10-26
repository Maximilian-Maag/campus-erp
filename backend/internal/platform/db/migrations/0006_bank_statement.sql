-- Canonical schema, slice S4 (Kasse / bank statement import + reconciliation).
--
-- Legacy background (kbs.mt940 / kbs.datmt940 / kbs.zabe / kbs.rueckausz_*):
--   * `mt940` is a 97-column god table: it mixes the statement HEADER
--     (mt_ausz_nr, mt_blatt_nr, mt_anfangsaldo, mt_endsaldo, mt_blz, mt_kto) with
--     every statement LINE (mt_valuta, mt_umsatzbetrag, mt_buchungschl, mt_kdref,
--     mt_bankref, mt_gebuehren, mt_grund1..mt_grund16) and with the matching
--     fields (zj_nr, zbu_nr, ao_nr, jahr, ds_nr, kap, ...).
--   * A statement therefore has no identity of its own; its header is repeated on
--     every line. `datmt940` only records which file was read when.
--   * There is no record of whether a line was matched to a payment, so
--     reconciliation state cannot be reconstructed.
--
-- The canonical model separates the statement header from its lines and records
-- the reconciliation outcome explicitly.
--
-- Format: SWIFT MT940. The field semantics follow the productive HISinOne parser
-- (rm.fia.payments.datex.mt940.Field61 / MT940Record).

-- One imported statement (one account, one statement number/page).
CREATE TABLE IF NOT EXISTS bank_statement (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id            bigint NOT NULL REFERENCES bank_account(id),
    reference             text   NOT NULL DEFAULT '',
    statement_number      int    NOT NULL DEFAULT 0,
    page                  int    NOT NULL DEFAULT 0,
    currency              text   NOT NULL DEFAULT 'EUR' CHECK (currency ~ '^[A-Z]{3}$'),
    opening_balance_cents bigint NOT NULL DEFAULT 0,
    opening_date          date,
    closing_balance_cents bigint NOT NULL DEFAULT 0,
    closing_date          date,
    line_count            int    NOT NULL DEFAULT 0,
    file_hash             text   NOT NULL,
    imported_by           text   NOT NULL,
    imported_at           timestamptz NOT NULL DEFAULT now(),
    -- Idempotency: the same file imported twice for the same account does not
    -- create a second copy of a statement.
    CONSTRAINT bank_statement_key UNIQUE (account_id, file_hash, statement_number, page)
);

-- One booking (:61:) with its reconciliation outcome.
CREATE TABLE IF NOT EXISTS bank_statement_line (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    statement_id  bigint NOT NULL REFERENCES bank_statement(id),
    line_no       int    NOT NULL,
    value_date    date,
    booking_date  date,
    credit        boolean NOT NULL,
    is_return     boolean NOT NULL DEFAULT false,
    amount_cents  bigint NOT NULL,
    currency      text   NOT NULL DEFAULT 'EUR',
    posting_key   text   NOT NULL DEFAULT '',
    client_ref    text   NOT NULL DEFAULT '',
    bank_ref      text   NOT NULL DEFAULT '',
    purpose       text   NOT NULL DEFAULT '',
    status        text   NOT NULL CHECK (status IN ('unmatched','matched','returned')),
    order_id      bigint REFERENCES payment_order(id),
    CONSTRAINT bank_statement_line_no_key UNIQUE (statement_id, line_no)
);

CREATE INDEX IF NOT EXISTS bank_statement_line_status_idx ON bank_statement_line(status);
