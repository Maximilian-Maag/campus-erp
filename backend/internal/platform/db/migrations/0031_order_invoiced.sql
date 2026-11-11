-- An order may be billed in parts.
--
-- The invoice match used to require the invoice's gross to equal the order's gross, so an invoice
-- that bills one delivery could not be matched at all (docs/12). The order now records how much of
-- its commitment has been invoiced, so a partial invoice converts only what it bills and the order
-- stays open for the rest; it becomes `received` when nothing is left.
--
-- The column is a running total rather than a derived sum: the invoices that matched an order live
-- in the e-invoice module, and procurement must not read another module's tables to know its own
-- state. Receipts keep their own total (`goods_receipt.received_cents`), and the two together may
-- never exceed the gross — the service refuses an invoice that would.
ALTER TABLE purchase_order
    ADD COLUMN IF NOT EXISTS invoiced_cents bigint NOT NULL DEFAULT 0
        CHECK (invoiced_cents >= 0);
