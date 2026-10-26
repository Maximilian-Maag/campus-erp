-- Rename the own-account entity to match the productive HISinOne model
-- (docs/06-vendor-model-crosswalk.md §2.2): the vendor calls the university's own
-- account `cash_account` and reserves `bank_account` for a counterparty account.
--
-- The rename carries the referencing FKs (payment_run.account_id,
-- bank_statement.account_id) with it, so no data migration is needed.
ALTER TABLE bank_account RENAME TO cash_account;
ALTER TABLE cash_account RENAME CONSTRAINT bank_account_iban_key TO cash_account_iban_key;
