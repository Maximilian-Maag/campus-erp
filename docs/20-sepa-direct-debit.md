# 20 — SEPA direct debit (pain.008.001.08)

The collection counterpart of the credit-transfer export (`docs/legacy/kbs-sepa-export.md`,
`internal/core/sepa/pain001.go`). It is the `bank-sepa-sdd` interface of the
registry (`internal/platform/interfaces`), now implemented.

## What it is

`sepa.DirectDebit` renders an **ISO 20022 pain.008.001.08** customer direct-debit
initiation: one creditor collecting from many debtors on one collection date under
one sequence type. `sepa.DirectDebitBatch` is the general form and the one writer:
it carries **one or more** `PmtInf` blocks, each with its own sequence type and
collection date, under a header that totals them — the shape a collection run takes
when it collects mandates of mixed types (`docs/22`). `DirectDebit` is the
one-instruction case and delegates to the batch, so a single-type file is the same
bytes it has always been. Like the credit transfer it is stdlib-only (no XML
library), deterministic for a given input, and pinned by golden files
(`testdata/golden_pain008.xml`, `testdata/golden_pain008_two_groups.xml`).

## Element order (XSD sequences)

```
Document > CstmrDrctDbtInitn
  GrpHdr:            MsgId, CreDtTm, NbOfTxs, CtrlSum, InitgPty(Nm)
  PmtInf:            PmtInfId, PmtMtd=DD, NbOfTxs, CtrlSum,
                     PmtTpInf(SvcLvl=SEPA, LclInstrm, SeqTp),
                     ReqdColltnDt, Cdtr(Nm), CdtrAcct(Id/IBAN), CdtrAgt(BICFI),
                     ChrgBr=SLEV, CdtrSchmeId, DrctDbtTxInf+
  DrctDbtTxInf:      PmtId(EndToEndId), InstdAmt@Ccy, DrctDbtTx(MndtRltdInf:
                     MndtId, DtOfSgntr), DbtrAgt(BICFI), Dbtr(Nm),
                     DbtrAcct(Id/IBAN), RmtInf(Ustrd)
```

Two SEPA specifics worth noting:

- The **creditor identifier** (SEPA Gläubiger-ID) goes in
  `PmtInf/CdtrSchmeId/Id/PrvtId/Othr/Id` with `SchmeNm/Cd = SEPA` — not a flat
  field.
- Each transaction carries its **mandate**: `MndtId` and `DtOfSgntr` (the mandate
  signature date) under `DrctDbtTx/MndtRltdInf`.

`ReqdColltnDt` is a plain ISO date (unlike pain.001, whose `ReqdExctnDt` wraps a
`<Dt>`). The creditor/debtor agent blocks (`CdtrAgt`/`DbtrAgt`) are emitted only
when a BIC is present.

## Validation

`Marshal` validates before rendering and returns a sentinel error matchable with
`errors.Is`:

| error | when |
|---|---|
| `ErrMessageIDRequired` / `ErrInitiatingPartyRequired` / `ErrCreditorNameRequired` | a required text is blank |
| `ErrCreditorIDRequired` | no SEPA creditor identifier |
| `ErrSequenceTypeInvalid` | `SeqTp` not in `FRST`/`RCUR`/`OOFF`/`FNAL` |
| `ErrLocalInstrumentInvalid` | `LclInstrm` not `CORE`/`B2B` |
| `ErrCollectionDateRequired` | no `ReqdColltnDt` |
| `ErrMandateIDRequired` / `ErrMandateDateRequired` | a transaction has no mandate |
| `ErrDebtorNameRequired` / `ErrDebtorIBANInvalid` / `ErrDebtorBICInvalid` | debtor side |
| `ErrCreditorIBANInvalid` / `ErrCreditorBICInvalid` | creditor side |
| `ErrAmountNotPositive` / `ErrNoTransactions` | amount / batch |
| `ErrFieldTooLong` | a field exceeds its ISO `Max35Text` / `Max140Text` limit |

IBAN (ISO 7064 mod-97) and BIC (ISO 9362) validation are shared with pain.001.

## The GBIC profile

Generated files are validated against the vendor's German Banking Industry Committee
(GBIC) schema, which is stricter than plain ISO 20022. Two consequences for callers:

- The scheme name in the creditor identifier is proprietary: the file carries
  `CdtrSchmeId/Id/PrvtId/Othr/SchmeNm/Prtry` (not `Cd`).
- `DbtrAgt` (the debtor agent, i.e. the debtor's BIC) is **mandatory** in pain.008.
  A transaction whose mandate carries no BIC cannot be rendered — `Marshal` returns
  `ErrDebtorAgentRequired` rather than emitting a file the bank would reject.

## Not yet done

- **The collection run exists** (`internal/modules/kasse/collectionrun*.go`,
  `docs/22-collection-runs.md`): it gathers mandates into a pain.008 for one
  collection date, under one sequence type or — filed without one — under each
  mandate's own, which makes the file carry one `PmtInf` per type. It can also select
  its items from the open receivables due for collection (`from-receivables`,
  `docs/22`). Still open: EBICS transport.
- **XSD validation is skipped**: the vendor ships the ISO 20022 schemas, but none is
  committed here, so `golden.ValidateXSD` has no schema to run against. The golden
  file plus the structural tests are the guard.
- EBICS transport (the channel these files travel over) remains `planned`
  (`docs/14 §4.3`).
