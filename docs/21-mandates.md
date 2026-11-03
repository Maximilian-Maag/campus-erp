# 21 — SEPA direct-debit mandates

The authorisation data direct debit needs: who may be collected from, under which
mandate, and since when. Consumed by the pain.008 generator
(`docs/20-sepa-direct-debit.md`), it fills the gap that blocked a collection run.

## Model

`direct_debit_mandate` (`migrations/0017_direct_debit_mandate.sql`):

| column | role |
|---|---|
| `mandate_reference` | the SEPA mandate id (`MndtId`), `UNIQUE`, ≤ 35 chars |
| `debtor_name` / `debtor_iban` / `debtor_bic` | the debtor (BIC optional) |
| `signature_date` | when the mandate was signed (`DtOfSgntr`) |
| `sequence_type` | `FRST` / `RCUR` / `OOFF` / `FNAL` (constrained) |
| `active` | soft state (a mandate can be retired without deleting history) |
| `created_by` / `created_at` | provenance |

`active` and the `created_by`/`created_at` pair follow the catalogue/tax-office
convention; there are no hard deletes (ADR-0006).

## API

| method | path |
|---|---|
| `POST` | `/api/direct-debit/mandates` — register a mandate |
| `GET` | `/api/direct-debit/mandates/{reference}` — read one |

Direct debit is a receivables process, so the routes sit behind the `erp-kasse`
role like dunning (`docs/17`). The write is audited (`mandate.create`).

## Validation

`kasse.NewMandate` validates before any write and returns a sentinel error matchable
with `errors.Is`: `ErrMandateReferenceRequired` / `ErrMandateReferenceTooLong`,
`ErrNameRequired`, `ErrIBANInvalid` (ISO 7064 mod-97, shared with the rest of
Kasse), `ErrDebtorBICInvalid` (ISO 9362, shared with `core/sepa`), and
`ErrSignatureDateRequired` / `ErrSequenceTypeUnknown`. A duplicate reference is
`ErrMandateReferenceExists` (a `409`).

## Not yet done

- ~~No collection run yet~~ — done since: the collection run exists
  (`internal/modules/kasse/collectionrun*.go`, docs/22); it gathers mandates into a pain.008
  for one collection date and sequence type. What is still open there is selecting items from
  outstanding receivables automatically, and EBICS transport.
- No ETL dataset: the legacy mandate table (`kbs.mandat`) is empty in the Muster
  (`docs/legacy/kbs-payment-orders.md`), so the mapping cannot be validated.
- No UI; the generated Angular client already exposes the two operations.
