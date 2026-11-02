# Open questions

> **Update (productive source received).** The productive the reference implementation build was
> handed over (three archives in `~/Downloads`). Its measured contents —
> including the vendor's 1 486-table schema *with declared FKs and German
> comments*, the payment-traffic layer (SEPA/CAMT/MT940 + ISO 20022 XSDs) and the
> ERP Java sources — are inventoried in `docs/05-productive-source-inventory.md`.
> That resolves or de-risks several questions below; the affected answers are
> noted inline.

To resolve in the project discussion. Ordered by blocking-ness.

## Q1 — Which product is the target? (blocking)

**Answered by the productive source (see `docs/05` §8).** the reference implementation is the
vendor's unified product (single schema, English table names); the supplied dump
is the **legacy** the legacy university ERP (module schemas `sva4`/`mbs`/`cob`/`kbs`, German
abbreviations). They share no tables. The target is the legacy ERP (the dump);
the reference implementation is its successor and an authoritative reference model.

## Q2 — What is the actual objective?

Understanding? Integration with another system? Migration off the legacy system? Replacing a
module? Reporting/BI? Data quality? Each implies a very different depth and a
different set of required artefacts.

## Q3 — Which modules matter?

The dump covers four modules. Is the project about one (e.g. Personal), a subset,
or all four + their seams?

## Q4 — Is production data available?

The dump is a Muster/seed DB (many empty tables). Real row volumes, distributions
and edge cases are unknown. Will a production extract (anonymised) be provided?

## Q5 — Domain-expert access?

The DB has essentially **no comments** (6 columns, 0 tables). Column semantics
for ~38k columns cannot be recovered from the DB alone. Is there access to vendor
documentation, or to HR/finance/cashier domain experts?

## Q6 — Are the `__`-prefixed tables authoritative?

Both `k_plz` and `__k_plz` exist; which is live per module? This determines the
correct tables to read. Requires vendor release documentation or app-team input.

## Q7 — Relationship integrity?

With zero FKs, join paths are undocumented. Do we (a) derive them empirically
from data, (b) obtain them from the vendor, or (c) treat the app as the source of truth?

## Q8 — Interface contracts?

Payroll (LBV), banking (SEPA/MT940), ELSTER, statistics: are interface specs
available, or must they be reverse-engineered from the `imp_*`/`out_*` tables
and file formats?

## Q9 — Repository name & visibility?

Currently `campus-erp` (private). Rename if Q1 changes the
target.

## Q10 — Compliance?

The data is HR/finance — inherently sensitive even as sample data. Any
DSGVO / data-protection constraints on how it is stored and processed in this
project?
