# 18 — Test isolation: fixing the database flakiness

A kasse integration test failed once during a full run and passed in isolation
and on every re-run. This note records the root cause and the fix, because a
flaky test is a false signal — it teaches the team to re-run instead of to trust.

## Symptom

`TestPaymentRunRejectsInvalidCreditorBIC` intermittently reported
`want ErrBICInvalid, got <nil>` under `go test ./...`, but passed alone.

## Root cause

Three things combined:

1. **One database shared by every test binary.** The harness opened the shared
   `DATABASE_URL` and all tests wrote to the same tables. Go runs packages — and
   `tools/run_tests.sh` runs gremlins, which runs a package's tests — in
   parallel; two `kasse` test binaries can therefore run at once.
2. **A shared identity.** `payment_recipient` is keyed by **IBAN**, and
   `OrderRepo.CreateRecipient` is an **upsert** (`ON CONFLICT (iban) DO UPDATE
   SET bic = EXCLUDED.bic`). The test fixtures reused the same hardcoded IBAN
   (`DE89370400440532013000`) with different BICs — one test's valid BIC
   (`COBADEFFXXX`), the invalid-BIC test's `NOTABIC`.
3. **The assertion reads shared state.** The invalid-BIC test creates a
   recipient with `NOTABIC` and expects the payment run to reject the order. When
   another binary upserted the same IBAN to a valid BIC in between, the run
   correctly accepted it — so the test failed for a reason that had nothing to do
   with the code under test.

Unique *suffixes* (the existing `uniqueSuffix`/`testSeqNext`) were not enough:
the identity here is the IBAN, which was fixed.

## The fix

**Isolation, not more uniqueness.** `internal/platform/dbtest` gives each test
**process** its own schema: it creates `test_<pid>`, opens the pool with
`search_path` set to it, and runs the migrations into it. Two processes can no
longer touch the same table. The schema is dropped when the process exits
(`dbtest.Main`), so the shared database stays clean; a reused PID starts fresh.
Because gremlins *kills* timed-out test processes (so `TestMain` never runs),
`dbtest` also sweeps schemas whose owning PID is gone at the start of the next
run — a killed run cannot leak a schema, and a concurrent run is never touched.
Every package's `testPool` now delegates to it, and each has a `TestMain` that
calls `dbtest.Main`.

**Belt and braces in kasse.** The recipient fixtures now use `uniqueIBAN(t)` — a
valid mod-97 IBAN built from the test sequence — so no two recipients ever share
an identity, with or without isolation.

Verified: two `kasse` test binaries running concurrently both pass, and the
database shows one `test_<pid>` schema per process (removed on exit).

## What this says about kasse

- `payment_recipient` keyed by IBAN with an upsert on `bic` means the BIC is
  global per IBAN: two callers (or tests) writing the same IBAN race on it. That
  is acceptable for production (a bank account's BIC is stable) but it is exactly
  why tests must not reuse an IBAN.
- The `kasse` package still holds eight sub-domains (cash book, payment orders,
  payment runs, statements, dunning, foundations, tax, IBAN helpers) in one
  package. Splitting it along those lines is a standing refactor — not a
  correctness issue, but the reason integration fixtures are easy to cross.

## Remaining

- `audit_event` and `budget_position` are written by several packages; with
  per-process schemas they are now isolated too, so the whole class is gone. Any
  new integration test should use `dbtest.Pool` rather than reopening the shared
  DSN.
