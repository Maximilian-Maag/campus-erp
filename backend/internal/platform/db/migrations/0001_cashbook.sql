-- Canonical schema, slice S1 (Kasse / cash book).
-- One database per tenant (ADR-0002), so no tenant_id columns: the database IS the tenant.

CREATE TABLE audit_event (
    seq       bigint      PRIMARY KEY,
    at        timestamptz NOT NULL,
    actor     text        NOT NULL,
    action    text        NOT NULL,
    subject   text        NOT NULL,
    before    text        NOT NULL DEFAULT '',
    after     text        NOT NULL DEFAULT '',
    prev_hash text        NOT NULL,
    hash      text        NOT NULL
);

CREATE TABLE cash_book_day (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_date date        NOT NULL UNIQUE,
    status        text        NOT NULL CHECK (status IN ('open', 'closed')),
    opening_cents bigint      NOT NULL,
    closing_cents bigint,
    opened_by     text        NOT NULL,
    opened_at     timestamptz NOT NULL DEFAULT now(),
    closed_by     text,
    closed_at     timestamptz
);

CREATE TABLE cash_booking (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    day_id       bigint      NOT NULL REFERENCES cash_book_day (id),
    kind         text        NOT NULL CHECK (kind IN ('income', 'expense')),
    amount_cents bigint      NOT NULL CHECK (amount_cents > 0),
    purpose      text        NOT NULL,
    booked_by    text        NOT NULL,
    booked_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cash_booking_day_idx ON cash_booking (day_id);
