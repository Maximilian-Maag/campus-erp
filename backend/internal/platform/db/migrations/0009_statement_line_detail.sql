-- Statement-line detail: partial clearing + counterparty + structured references
-- (docs/06-vendor-model-crosswalk.md §2.3). The vendor's bankstatementline
-- carries uncleared_amount and a counterparty block; the structured :86: fields
-- (EREF+, ABWA+/ABWE+, and the numbered ?30/?31/?32/?33 convention) supply them.
ALTER TABLE bank_statement_line
    ADD COLUMN IF NOT EXISTS uncleared_cents   bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS end_to_end_ref    text   NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS counterparty_name text   NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS counterparty_iban text   NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS counterparty_bic  text   NOT NULL DEFAULT '';
