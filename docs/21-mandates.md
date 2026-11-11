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
| `GET` | `/api/direct-debit/mandates` — list them, `?active=true` and `?sequence_type=` to narrow |
| `GET` | `/api/direct-debit/mandates/{reference}` — read one |
| `POST` | `/api/direct-debit/mandates/{reference}/retire` — withdraw the authorisation |

The list's filters are the server's: a screen that offers the mandates a run can
actually gather asks for the active ones collecting under the run's sequence type,
rather than holding every mandate to show a few.

Direct debit is a receivables process, so the routes sit behind the `erp-kasse`
role like dunning (`docs/17`). The write is audited (`mandate.create`).

## Screen — *Kasse: Mandate* (`/kasse/mandate`)

Lists the mandates and registers one. The **filters are the server's** — the state (*alle* / *nur
aktive*) and the sequence type are asked for, not narrowed on screen — exactly as the collection-run
screen asks for the mandates a run can gather, so both read the same route the same way.

The sequence types come from **one list** the collection run and this screen share
(`frontend/src/app/core/direct-debit.ts`): a run that collects `RCUR` may only gather mandates that
carry `RCUR`, so the two screens must not drift apart, and the code stays the value while the German
word is only a label.

The screen refuses what it can see is missing — a mandate without a reference, a debtor, an IBAN or
a signature date authorises nothing — and leaves the checks it cannot make to the server: the IBAN's
checksum (ISO 7064) and the BIC's shape are validated once, in Kasse, and a refusal is shown as it
comes back rather than replaced by a second implementation here. A refused mandate keeps what was
typed, so it can be corrected rather than retyped.

**Retiring** a mandate (the debtor revoked the authorisation) is a soft state: the row stays, the
flag goes, and the revocation is in the trail. The screen asks for it **twice** — the first click
turns the row into the question, the second answers it — because a revocation is effectively final
(no route re-activates a mandate), and a destructive action taken on one click is a mis-click away
from a mandate nobody may collect under. Once retired, the row says so and no longer offers the
action, without a reload: the mandate is soft state, so the change is visible where it was made.

A second revocation is refused (`ErrMandateRetired`, a `409`) rather than quietly repeated: the
revocation happened once, and a silent no-op would read like one that took effect. A reference
nothing carries is a `404`, and a blank actor is refused before the mandate is even read.

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
  for one collection date and sequence type, and can select its items from the open
  receivables (`docs/22`). What is still open there is EBICS transport.
- No ETL dataset: the legacy mandate table (`kbs.mandat`) is empty in the Muster
  (`docs/legacy/kbs-payment-orders.md`), so the mapping cannot be validated.
- ~~Registering a mandate is still an API call, not a screen~~ — done since: the *Kasse: Mandate*
  screen lists the mandates under the same filters the collection-run screen uses and registers one
  (above).
- ~~Retiring a mandate~~ — done since: `POST /api/direct-debit/mandates/{reference}/retire` and the
  screen's two-step revocation (above). What deliberately does **not** exist is re-activating one: a
  revoked mandate stays revoked, and a mandate that should be collected under again is a new
  authorisation with its own signature date and reference, not a flag turned back on.
