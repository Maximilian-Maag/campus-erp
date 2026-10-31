# 20 — SEPA direct debit (pain.008.001.08)

The collection counterpart of the credit-transfer export (`docs/legacy/kbs-sepa-export.md`,
`internal/core/sepa/pain001.go`). It is the `bank-sepa-sdd` interface of the
registry (`internal/platform/interfaces`), now implemented.

## What it is

`sepa.DirectDebit` renders an **ISO 20022 pain.008.001.08** customer direct-debit
initiation: one creditor collecting from many debtors on one collection date under
one sequence type. Like the credit transfer it is stdlib-only (no XML library),
deterministic for a given input, and pinned by a golden file
(`testdata/golden_pain008.xml`).

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

## Not yet done

- **The collection run exists** (`internal/modules/kasse/collectionrun*.go`,
  `docs/22-collection-runs.md`): it gathers mandates into a pain.008 for one
  collection date and sequence type. Still open: selecting items from outstanding
  receivables automatically (today they are supplied explicitly), and EBICS
  transport.
- **XSD validation is skipped**: the vendor ships the ISO 20022 schemas, but none is
  committed here, so `golden.ValidateXSD` has no schema to run against. The golden
  file plus the structural tests are the guard.
- EBICS transport (the channel these files travel over) remains `planned`
  (`docs/14 §4.3`).
