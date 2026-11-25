-- The opening movement records which book it was carried from, and a unique index
-- allows one opening movement per source book. The guard in 0034 stops two
-- carry-overs landing in the same target book; it does not stop the same source
-- book being carried twice under two different target references, which posts the
-- net twice and doubles the following year's opening (#207).
--
-- The column is nullable so the opening movements written before this migration
-- keep working; the index only covers the rows that name a source.

ALTER TABLE foundation_booking
    ADD COLUMN IF NOT EXISTS carried_from_book_id bigint
        REFERENCES foundation_book (id);

CREATE UNIQUE INDEX IF NOT EXISTS foundation_booking_carried_from_key
    ON foundation_booking (carried_from_book_id) WHERE carried_from_book_id IS NOT NULL;
