# Open questions

To resolve in the project discussion. Ordered by blocking-ness.

## Q1 — Which product is the target? (blocking)

The request says "HISinOne". The dump contains the **Hochschul-ERP**
(`HISSVA`/`HISMBS`/`HISFSV`/`HISCOB`/`HISKBS`), not HISinOne campus
(`APP`/`STU`/`EXA`/`DOC`). They share no tables.

- Is the target the ERP (this dump)?
- Or was a campus-management dump intended and the wrong file supplied?
- Or is "HISinOne" being used as an umbrella name for the whole HIS suite?

## Q2 — What is the actual objective?

Understanding? Integration with another system? Migration off HIS? Replacing a
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
for ~38k columns cannot be recovered from the DB alone. Is there access to HIS
documentation, or to HR/finance/cashier domain experts?

## Q6 — Are the `__`-prefixed tables authoritative?

Both `k_plz` and `__k_plz` exist; which is live per module? This determines the
correct tables to read. Requires HIS release documentation or app-team input.

## Q7 — Relationship integrity?

With zero FKs, join paths are undocumented. Do we (a) derive them empirically
from data, (b) obtain them from HIS, or (c) treat the app as the source of truth?

## Q8 — Interface contracts?

Payroll (LBV), banking (SEPA/MT940), ELSTER, statistics: are interface specs
available, or must they be reverse-engineered from the `imp_*`/`out_*` tables
and file formats?

## Q9 — Repository name & visibility?

Currently `hisinone-db-reverse-engineering` (private). Rename if Q1 changes the
target.

## Q10 — Compliance?

The data is HR/finance — inherently sensitive even as sample data. Any
DSGVO / data-protection constraints on how it is stored and processed in this
project?
