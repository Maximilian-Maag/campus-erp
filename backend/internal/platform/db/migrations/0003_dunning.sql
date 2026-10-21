-- Canonical schema, slice S5 (Kasse / dunning, "Mahnwesen").
--
-- Legacy background (kbs.mahn / kbs.mahnprot):
--   * `mahn` is a dunning level whose five amount bands are FLATTENED into the row:
--     bis_betr1..5 / mahn_geb1..5 / prct1..5 — an upper bound, a fee and a rate per
--     band, five bands hard-coded as columns.
--   * `mahnprot` is a hand-rolled change log: every field duplicated as a_<col>
--     (before) and n_<col> (after) plus aend_dat/zeit/art/sachb.
--   * Rates are `numeric` percentages; there are no constraints on ordering.
--
-- The canonical model puts bands in their own table (any number of them) and keeps
-- rates as integer basis points (1/100 percent) so money arithmetic stays exact.

CREATE TABLE IF NOT EXISTS dunning_level (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    year         int     NOT NULL CHECK (year BETWEEN 1900 AND 2999),
    code         text    NOT NULL,
    description  text    NOT NULL DEFAULT '',
    days_allowed int     NOT NULL DEFAULT 0 CHECK (days_allowed >= 0),
    active       boolean NOT NULL DEFAULT true,
    next_code    text    NOT NULL DEFAULT '',
    CONSTRAINT dunning_level_key UNIQUE (year, code)
);

CREATE TABLE IF NOT EXISTS dunning_band (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    level_id          bigint NOT NULL REFERENCES dunning_level(id) ON DELETE CASCADE,
    position          int    NOT NULL CHECK (position > 0),
    up_to_cents       bigint NOT NULL CHECK (up_to_cents >= 0),
    fee_cents         bigint NOT NULL CHECK (fee_cents >= 0),
    rate_basis_points bigint NOT NULL CHECK (rate_basis_points >= 0),
    CONSTRAINT dunning_band_position_key UNIQUE (level_id, position)
);

CREATE INDEX IF NOT EXISTS dunning_band_level_idx ON dunning_band(level_id);

-- An issued dunning notice. The charge is stored, not recomputed later, so a
-- configuration change cannot silently alter the history.
CREATE TABLE IF NOT EXISTS dunning_notice (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    claim_reference   text   NOT NULL,
    level_code        text   NOT NULL,
    outstanding_cents bigint NOT NULL CHECK (outstanding_cents > 0),
    charge_cents      bigint NOT NULL CHECK (charge_cents >= 0),
    issued_on         date   NOT NULL,
    issued_by         text   NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS dunning_notice_claim_idx ON dunning_notice(claim_reference);
