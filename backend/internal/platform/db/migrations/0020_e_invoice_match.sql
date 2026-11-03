-- Record the two-way match on the invoice itself (docs/09, docs/12).
--
-- The audit stream records that a match happened, but that is the trail, not queryable state:
-- a screen has to be able to tell a matched invoice from an unmatched one without replaying
-- events. NULL means "not matched yet".
--
-- The order stays the authority — a received order is matched — and this column is what the
-- invoice side knows afterwards.

ALTER TABLE e_invoice
    ADD COLUMN IF NOT EXISTS matched_order_id bigint REFERENCES purchase_order(id);

CREATE INDEX IF NOT EXISTS e_invoice_matched_order_id_idx
    ON e_invoice (matched_order_id)
    WHERE matched_order_id IS NOT NULL;
