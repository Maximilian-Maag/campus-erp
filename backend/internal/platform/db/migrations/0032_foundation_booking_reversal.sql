-- A foundation's movements are append-only: a booking made in error is answered by a
-- counter-movement that points back at it, never by editing or deleting the original.
--
-- The two columns make "reversed" a stored fact rather than a flag that can drift, and the partial
-- unique indexes put the two halves of the rule in the database as well as in the service: one
-- reversal per booking, and one reversal per counter-movement. A booking that pointed at a reversal
-- could cancel a cancellation, which nothing here supports.

ALTER TABLE foundation_booking
    ADD COLUMN reverses bigint REFERENCES foundation_booking (id),
    ADD COLUMN reversed_by bigint REFERENCES foundation_booking (id);

CREATE UNIQUE INDEX foundation_booking_reverses_key
    ON foundation_booking (reverses) WHERE reverses IS NOT NULL;

CREATE UNIQUE INDEX foundation_booking_reversed_by_key
    ON foundation_booking (reversed_by) WHERE reversed_by IS NOT NULL;

-- A booking is an original or a reversal, never both.
ALTER TABLE foundation_booking
    ADD CONSTRAINT foundation_booking_reversal_shape
        CHECK (num_nonnulls(reverses, reversed_by) <= 1);
