-- SEPA direct-debit collection runs (slice: collection run, issue #22). A run
-- gathers mandates into one pain.008 file for one collection date and one
-- sequence type; the produced file is stored so the run is reproducible (the
-- direct-debit counterpart of payment_run). See docs/22-collection-runs.md.
CREATE TABLE IF NOT EXISTS direct_debit_run (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    reference        text   NOT NULL UNIQUE,
    account_id       bigint NOT NULL REFERENCES cash_account(id),
    creditor_id      text   NOT NULL,
    collection_date  date   NOT NULL,
    sequence_type    text   NOT NULL CHECK (sequence_type IN ('FRST', 'RCUR', 'OOFF', 'FNAL')),
    local_instrument text   NOT NULL CHECK (local_instrument IN ('CORE', 'B2B')),
    message_id       text   NOT NULL,
    item_count       integer NOT NULL,
    sum_cents        bigint NOT NULL,
    file_xml         text   NOT NULL,
    file_hash        text   NOT NULL,
    created_by       text   NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS direct_debit_run_item (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id            bigint NOT NULL REFERENCES direct_debit_run(id) ON DELETE CASCADE,
    mandate_reference text   NOT NULL REFERENCES direct_debit_mandate(mandate_reference),
    end_to_end_id     text   NOT NULL,
    amount_cents      bigint NOT NULL,
    purpose           text   NOT NULL,
    CONSTRAINT direct_debit_run_item_mandate_key UNIQUE (run_id, mandate_reference)
);

CREATE INDEX IF NOT EXISTS direct_debit_run_item_run_idx ON direct_debit_run_item(run_id);
