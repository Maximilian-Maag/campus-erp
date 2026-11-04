-- KLR cost types (Kostenarten): the kind of cost a record is, and the bridge from the general
-- ledger's account codes to it (docs/13). The vendor's cost_type / cost_type_bridge shape is not
-- published, so the mapping is data: an account code is a string on the bridge, and when the
-- ledger side lands its codes line up without a code change — the same reasoning as the
-- distribution keys and the per-state statistics layouts.
CREATE TABLE IF NOT EXISTS cost_type (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code       text   NOT NULL UNIQUE,
    name       text   NOT NULL,
    created_by text   NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- One account belongs to exactly one cost type: the bridge is a function, not a bag of links,
-- so the account code is unique across the whole bridge rather than per cost type.
CREATE TABLE IF NOT EXISTS cost_type_account (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cost_type_id bigint NOT NULL REFERENCES cost_type(id),
    account_code text   NOT NULL UNIQUE
);

-- A record may carry the cost type it belongs to. NULL is a record nobody has classified yet,
-- which is what an import from the ledger starts as.
ALTER TABLE accounting_record
    ADD COLUMN IF NOT EXISTS cost_type_id bigint REFERENCES cost_type(id);

CREATE INDEX IF NOT EXISTS accounting_record_cost_type_idx
    ON accounting_record (cost_type_id)
    WHERE cost_type_id IS NOT NULL;
