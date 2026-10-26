-- Canonical schema, slice S6 (Kasse / foundation books).
--
-- Legacy background (kbs.stift / kbs.tgas / kbs.titkto):
--   * `stift` is foundation master data only (year, number, description,
--     carry-over flag); the fund movement is spread over `tgas` (a per-year
--     account key) and `titkto` (a ~50-column account/title table carrying the
--     actual/target amounts). There is no "book" entity, so isolating one
--     foundation's movements from another's is only a convention.
--
-- The canonical model gives each foundation its own book, so the movements of one
-- foundation can never be mixed with another's (ADR-0002-style isolation at the
-- domain level, tested).

CREATE TABLE IF NOT EXISTS foundation_book (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    foundation_id bigint NOT NULL REFERENCES foundation(id),
    reference     text   NOT NULL,
    currency      text   NOT NULL DEFAULT 'EUR' CHECK (currency ~ '^[A-Z]{3}$'),
    created_by    text   NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT foundation_book_key UNIQUE (foundation_id, reference)
);

-- Append-only movements. No hard deletes (ADR-0006): a correction is a reversal.
CREATE TABLE IF NOT EXISTS foundation_booking (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id      bigint NOT NULL REFERENCES foundation_book(id),
    kind         text   NOT NULL CHECK (kind IN ('income','expense')),
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    purpose      text   NOT NULL,
    booked_by    text   NOT NULL,
    booked_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS foundation_booking_book_idx ON foundation_booking(book_id);
