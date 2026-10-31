-- SEPA direct-debit mandates (slice: direct debit). The data the pain.008
-- generator needs: the mandate reference, the debtor, the mandate's signature
-- date and the sequence type. See docs/21-mandates.md.
CREATE TABLE IF NOT EXISTS direct_debit_mandate (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    mandate_reference text   NOT NULL UNIQUE,
    debtor_name       text   NOT NULL,
    debtor_iban       text   NOT NULL,
    debtor_bic        text   NOT NULL DEFAULT '',
    signature_date    date   NOT NULL,
    sequence_type     text   NOT NULL CHECK (sequence_type IN ('FRST', 'RCUR', 'OOFF', 'FNAL')),
    active            boolean NOT NULL DEFAULT true,
    created_by        text   NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);
