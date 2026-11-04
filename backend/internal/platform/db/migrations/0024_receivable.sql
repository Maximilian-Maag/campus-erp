-- Open receivables (offene Posten): the claim a direct-debit collection run
-- selects its items from. A receivable is a debtor's debt under a mandate, due
-- on a date; a run gathers the open ones due by its collection date. The state
-- is soft (open/collected/cancelled) — a collected item is linked to the run
-- that took it, never deleted (ADR-0006). See docs/22-collection-runs.md.
CREATE TABLE IF NOT EXISTS receivable (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    mandate_reference text   NOT NULL REFERENCES direct_debit_mandate(mandate_reference),
    amount_cents      bigint NOT NULL CHECK (amount_cents > 0),
    currency          text   NOT NULL,
    due_date          date   NOT NULL,
    purpose           text   NOT NULL,
    state             text   NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'collected', 'cancelled')),
    collected_run_id  bigint REFERENCES direct_debit_run(id),
    source            text   NOT NULL DEFAULT '',
    created_by        text   NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);

-- The selection query: the open items due by a date.
CREATE INDEX IF NOT EXISTS receivable_state_due_idx ON receivable(state, due_date);
