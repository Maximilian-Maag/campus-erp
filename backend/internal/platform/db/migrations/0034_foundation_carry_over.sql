-- A foundation's year-to-year carry-over posts the opening balance into the next
-- year's book. The service checked "is the target book still empty" with a plain
-- SELECT and then inserted the movement separately, so two concurrent carry-overs
-- both saw an empty target and both posted the net: the next year opened with
-- double the money and every row looked legitimate (#171).
--
-- The flag makes the opening movement a stored fact, and the partial unique index
-- puts "one opening movement per book" in the database as well as in the service:
-- the second concurrent carry-over loses the race at the insert instead of
-- silently doubling the book.

ALTER TABLE foundation_booking
    ADD COLUMN IF NOT EXISTS carries_over boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX IF NOT EXISTS foundation_booking_carry_over_key
    ON foundation_booking (book_id) WHERE carries_over;
