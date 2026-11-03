-- A KLR record belongs to a period, which is not the same as the day it was entered
-- (docs/13).
--
-- The distribution engine runs over a period — the month just closed, say — but `booked_at` is
-- when the record was typed in, which is after that period ended. Without a business date the
-- engine could only ever distribute the period the costs happened to be entered in, which is
-- the opposite of what a month-end close needs.
--
-- Existing rows: their booking date is the only thing known about them, so that is what they
-- get. New rows default to today.

ALTER TABLE accounting_record ADD COLUMN IF NOT EXISTS business_date date;
UPDATE accounting_record SET business_date = booked_at::date WHERE business_date IS NULL;
ALTER TABLE accounting_record ALTER COLUMN business_date SET NOT NULL;
ALTER TABLE accounting_record ALTER COLUMN business_date SET DEFAULT CURRENT_DATE;

CREATE INDEX IF NOT EXISTS accounting_record_business_date_idx
    ON accounting_record (cost_center_id, business_date);
