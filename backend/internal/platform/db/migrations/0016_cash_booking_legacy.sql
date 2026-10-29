-- Import provenance for cash bookings (ETL from the legacy counter book).
--
-- Normal bookings leave legacy_ref NULL. A booking imported from the legacy
-- counter book (kbs.schalterbuch) carries a stable reference — the legacy table,
-- year and booking number — so re-running the import upserts in place instead of
-- duplicating, without weakening the day's own integrity. See docs/legacy/kbs-cashbook.md.

ALTER TABLE cash_booking ADD COLUMN IF NOT EXISTS legacy_ref text;

CREATE UNIQUE INDEX IF NOT EXISTS cash_booking_legacy_ref_key
    ON cash_booking (legacy_ref)
    WHERE legacy_ref IS NOT NULL;
