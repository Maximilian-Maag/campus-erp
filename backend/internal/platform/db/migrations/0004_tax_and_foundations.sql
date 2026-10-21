-- Canonical schema, slice S7 (Kasse / tax-office data and foundations).
--
-- Legacy background:
--   * kbs.mwst_art — VAT types. Columns mw_art (code, varchar(8)), mw_bez
--     (description), mw_kz (rate, numeric), mw_typ (a 1-char U/V flag) and
--     mw_vorzeichen (a '+'/'-' string). 32 rows, the one populated table here.
--     The rate is decimal and the sign is carried as a CHARACTER.
--   * kbs.finanzamt — tax-office master data, one row per year, including its bank
--     account (blz/konto/bic/iban/inhaber) as plain columns with no validation.
--   * kbs.stift — foundations: year, number, description and a 1-char carry-over
--     flag. kbs.stiftprot holds the a_/n_ before-after change log.
--
-- Canonical: integer basis points, an enumerated kind instead of a sign character,
-- real constraints, and IBAN validation in the domain.

CREATE TABLE IF NOT EXISTS vat_type (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code              text   NOT NULL UNIQUE,
    description       text   NOT NULL DEFAULT '',
    rate_basis_points bigint NOT NULL CHECK (rate_basis_points BETWEEN 0 AND 10000),
    kind              text   NOT NULL CHECK (kind IN ('output', 'input'))
);

CREATE TABLE IF NOT EXISTS tax_office (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    year           int    NOT NULL CHECK (year BETWEEN 1900 AND 2999),
    number         text   NOT NULL,
    name           text   NOT NULL,
    street         text   NOT NULL DEFAULT '',
    postal_code    text   NOT NULL DEFAULT '',
    city           text   NOT NULL DEFAULT '',
    account_holder text   NOT NULL DEFAULT '',
    iban           text   NOT NULL DEFAULT '',
    bic            text   NOT NULL DEFAULT '',
    CONSTRAINT tax_office_key UNIQUE (year, number)
);

CREATE TABLE IF NOT EXISTS foundation (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    year        int     NOT NULL CHECK (year BETWEEN 1900 AND 2999),
    number      text    NOT NULL,
    description text    NOT NULL,
    carry_over  boolean NOT NULL DEFAULT false,
    CONSTRAINT foundation_key UNIQUE (year, number)
);
