-- Give a stored invoice a direction (docs/09).
--
-- Nothing marked a row as inbound (a received Eingangsrechnung) or outbound (one we issued),
-- so the match could not tell them apart and had to lean on the order's state alone. An
-- outgoing invoice has no business being matched to a purchase order.
--
-- Existing rows: the default says "outbound" because that is the route everything was created
-- through, except the rows the audit stream shows arriving through `einvoice.receive` — which
-- is exactly what the trail is for, and it is used here to correct them.

ALTER TABLE e_invoice
    ADD COLUMN IF NOT EXISTS direction text NOT NULL DEFAULT 'outbound'
        CHECK (direction IN ('outbound', 'inbound'));

UPDATE e_invoice i
   SET direction = 'inbound'
 WHERE i.direction = 'outbound'
   AND EXISTS (
       SELECT 1 FROM audit_event e
        WHERE e.subject = 'e_invoice/' || i.id
          AND e.action = 'einvoice.receive'
   );

CREATE INDEX IF NOT EXISTS e_invoice_direction_idx ON e_invoice (direction);
