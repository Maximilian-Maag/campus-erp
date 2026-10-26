# Legacy findings — SEPA credit-transfer export (Kasse, `kbs`)

Reverse-engineered from the Muster dump `hisrm_muster_30_09_26.backup` (schema
`kbs`) **and** the productive HISinOne source (see `docs/05`). These findings
drive slice S3 and the ETL/interface mapping. Measured, not inferred.

## Legacy `kbs.zawe` — payment instruction / run

`zawe` groups payments per bank account (`blz`, `konto`, `iban`, `bic`, `inhaber`,
`bname`, `waehrung`, `art`, `sperre`, `stap_nr`, `export_format`) and carries the
running totals as ten `numeric` pairs (`e_ist`/`a_ist`/`e_soll`/`a_soll`/… income
× expense × actual/target/in-flight). `zabe` is the statement side.

Three structural facts matter for a replacement:

1. **No bank-account entity.** The debtor account is denormalised onto every
   payment row; there is nothing to point a run at.
2. **The produced file is not stored.** Only the aggregate counters survive, so
   "was this run exported?" and "did it change?" cannot be answered from the DB.
3. **Nothing prevents paying an order twice.** There is no per-order uniqueness.

## The format the vendor actually produces

The productive HISinOne writes SEPA credit transfers in
`rm.fia.payments.datex.parser.sepa.SEPAWriterCreditTransferVersion39`, as
**ISO 20022 pain.001.001.09** in the German DK/GBIC profile. The relevant facts,
read from that source and the shipped XSD
`pain.001.001.09_GBIC_5.xsd`:

| Element | Value the vendor sets |
|---|---|
| namespace | `urn:iso:std:iso:20022:tech:xsd:pain.001.001.09` |
| `GrpHdr/MsgId` | ≤ 35, from the run |
| `GrpHdr/CreDtTm` | creation, `YYYY-MM-DDThh:mm:ss` (no zone) |
| `GrpHdr/NbOfTxs`, `CtrlSum` | obligatory (SEPA 3.1+) |
| `GrpHdr/InitgPty/Nm` | debtor name, ≤ 140 |
| `PmtInf/PmtMtd` | `TRF` |
| `PmtInf/BtchBookg` | `true` |
| `PmtInf/PmtTpInf/SvcLvl/Cd` | `SEPA` |
| `PmtInf/ReqdExctnDt/Dt` | value date; if in the past, today |
| `PmtInf/Dbtr`, `DbtrAcct/Id/IBAN`, `DbtrAgt/FinInstnId/BICFI` | debtor side (BIC omitted when blank) |
| `PmtInf/ChrgBr` | `SLEV` |
| `CdtTrfTxInf/PmtId/EndToEndId` | client reference, ≤ 35 |
| `CdtTrfTxInf/Amt/InstdAmt` | amount, `Ccy="EUR"` |
| `CdtTrfTxInf/CdtrAgt/FinInstnId/BICFI` | creditor BIC, omitted when blank |
| `CdtTrfTxInf/Cdtr/Nm` | account holder, ≤ 140 |
| `CdtTrfTxInf/CdtrAcct/Id/IBAN` | creditor IBAN |
| `CdtTrfTxInf/RmtInf/Ustrd` | purpose, unstructured, ≤ 140 |

## What the canonical model does instead (slice S3)

`backend/internal/platform/db/migrations/0005_sepa_export.sql`:

| legacy `kbs` | canonical |
|---|---|
| debtor denormalised onto each payment | **`bank_account`** entity (the debtor side) |
| file discarded | **`payment_run.file_xml` + `file_hash`** — the exact bytes handed to the bank are stored |
| ten counter pairs | `payment_run.item_count` + `sum_cents` (the totals are recomputed from the items) |
| nothing stops double payment | **`payment_run_item` `UNIQUE(order_id)`** — an approved order is paid at most once |
| re-run unknowable | **idempotent on `reference`** — re-issuing a reference returns the same run and file |

The generator is `backend/internal/core/sepa/pain001.go`
(`sepa.CreditTransfer.Marshal`). It is deterministic for a given input, so the
output is pinned by a golden file, `backend/internal/core/sepa/testdata/golden_pain001.xml`.

## Verification

The golden file was validated against the **vendor's own XSD** shipped in the
productive source:

```
xmllint --noout --schema \
  pain001V00109GBIC5/xsd/pain.001.001.09_GBIC_5.xsd \
  backend/internal/core/sepa/testdata/golden_pain001.xml
# → golden_pain001.xml validates
```

Tests: unit (generator, validation branches, boundaries) + integration
(run creation, idempotency, double-payment guard, audit chain) + HTTP e2e. The
`sepa` and `kasse` packages are at **100 % mutation efficacy** (gremlins,
ADR-0008).

## Still open (S4)

Statement import — MT940 and CAMT.053 — is not done. The vendor's parser and the
camt.053.001.02/.08 and camt.054 XSDs are in the productive source
(`rm.fia.payments.datex.mt940`, `…sepa.camt*`), so S4 is unblocked on the format
in the same way S3 was.
