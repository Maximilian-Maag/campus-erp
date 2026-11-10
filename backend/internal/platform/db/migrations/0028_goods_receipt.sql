-- Goods receipts (Wareneingang): what actually arrived, line by line.
--
-- Vendor reference (docs/08-reuse-inventory.md §3): the reference implementation's
-- procurement model carries purchase_order / purchase_order_line; the goods receipt is the
-- delivery side of the same order, and it is what makes a *partial* receipt possible — the
-- whole-order match (docs/12) can only convert an order in full.
--
-- Append-only, like every other movement: a receipt is a fact about a delivery, so a
-- correction is a further receipt, never an edit. The received quantity of an order line is
-- derived from its receipt lines rather than stored on the line, so the two can never
-- disagree.

CREATE TABLE IF NOT EXISTS goods_receipt (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    bigint NOT NULL REFERENCES purchase_order(id),
    reference   text   NOT NULL,
    -- The value this receipt converted from a reservation into a consumption, as it was
    -- converted: stored, so a later price change cannot re-value a receipt that happened.
    received_cents bigint NOT NULL DEFAULT 0 CHECK (received_cents >= 0),
    received_on date   NOT NULL,
    received_by text   NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    -- The delivery note is the idempotency key: the same receipt recorded twice for one
    -- order is refused rather than counted twice.
    CONSTRAINT goods_receipt_key UNIQUE (order_id, reference)
);

CREATE TABLE IF NOT EXISTS goods_receipt_line (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    receipt_id     bigint NOT NULL REFERENCES goods_receipt(id),
    line_no        int    NOT NULL,
    quantity_milli bigint NOT NULL CHECK (quantity_milli > 0),
    CONSTRAINT goods_receipt_line_key UNIQUE (receipt_id, line_no)
);

CREATE INDEX IF NOT EXISTS goods_receipt_order_idx ON goods_receipt(order_id);
CREATE INDEX IF NOT EXISTS goods_receipt_line_receipt_idx ON goods_receipt_line(receipt_id);
