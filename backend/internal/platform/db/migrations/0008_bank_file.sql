-- Split the physical import file (bank_file) from the statements it contains
-- (docs/06-vendor-model-crosswalk.md §2.1). Matches the reference implementation
-- model (bankfile -> bankstatement) and the legacy split (datmt940 recorded the
-- file, mt940 the lines).
--
-- One file may contain several statements; idempotency moves from the statement
-- to the file entity.

CREATE TABLE IF NOT EXISTS bank_file (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id  bigint NOT NULL REFERENCES cash_account(id),
    file_name   text   NOT NULL DEFAULT '',
    file_hash   text   NOT NULL,
    imported_by text   NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT bank_file_key UNIQUE (account_id, file_hash)
);

ALTER TABLE bank_statement ADD COLUMN IF NOT EXISTS bank_file_id bigint REFERENCES bank_file(id);

-- Backfill: one bank_file per distinct (account, hash) already present.
INSERT INTO bank_file (account_id, file_hash, imported_by)
    SELECT DISTINCT account_id, file_hash, imported_by FROM bank_statement
    ON CONFLICT (account_id, file_hash) DO NOTHING;

UPDATE bank_statement s
   SET bank_file_id = f.id
  FROM bank_file f
 WHERE f.account_id = s.account_id AND f.file_hash = s.file_hash
   AND s.bank_file_id IS NULL;

ALTER TABLE bank_statement ALTER COLUMN bank_file_id SET NOT NULL;

-- The unique key moves from the (account, file) pair to the file entity.
ALTER TABLE bank_statement DROP CONSTRAINT IF EXISTS bank_statement_key;
ALTER TABLE bank_statement ADD CONSTRAINT bank_statement_key UNIQUE (bank_file_id, statement_number, page);

ALTER TABLE bank_statement DROP COLUMN IF EXISTS file_hash;
