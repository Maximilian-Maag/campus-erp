# 31 — Steuersätze und Finanzämter

The two catalogues behind the exact VAT arithmetic: **Steuersätze** (VAT types — a rate and whether
it is Umsatzsteuer or Vorsteuer) and **Finanzämter** (the tax offices the declaration goes to, with
the account the tax is paid to). They are the other half of the tax domain whose foundation
references are described in `29-foundations.md`.

## What the domain holds

Both are per-year master data inherited from the source system, and both were **write-only** before
this screen: a VAT type could be stored and read back by key, a Finanzamt by year and number, but no
route listed them. A screen cannot be built on a route that does not exist, so the reads came first.

| Route | Meaning |
| --- | --- |
| `GET /api/vat-types` | the VAT-type catalogue, ordered by key |
| `POST /api/vat-types` | a rate, with its kind (`output` = Umsatzsteuer, `input` = Vorsteuer) |
| `GET /api/tax-offices?year=` | the Finanzämter, narrowed by year at the server |
| `POST /api/tax-offices` | one Finanzamt with address and account |

`year` is optional on the office list: without it every year is listed, which is what an operator
who has not chosen one expects. A year that is not a number is refused with a code rather than read
as every year.

## What the screen does

`/finanzen/steuern` (Finanzen — Steuersätze und Finanzämter) lists both catalogues, files a new VAT
type and a new Finanzamt, and narrows the offices by year. Two rules it follows on purpose:

- **The rate is shown as a percentage, the kind in words.** The API carries basis points (`1900`) and
  the codes `output`/`input`; the screen shows `19 %` and *Umsatzsteuer*, and an unknown kind shows
  its own code rather than nothing — a new kind must be visible, not silent.
- **The screen checks only what it can see.** A missing key, a missing name, a rate that is not a
  percentage: refused before the request. The IBAN's checksum and the BIC's shape are the server's,
  so a wrong one comes back as a refusal instead of being re-implemented here.

The two reads report their failures **separately**: a Finanzamt list that cannot be read does not hide
the VAT catalogue, and both alerts are shown when both fail. One alert for two failures would mean
the operator never learns about the second.

## What is not done

- **No editing.** A rate is written once; a correction is a new record. The source system's data is
  appended-to as well, but a screen for changing a wrong rate is not built.
- **No export.** The Finanzamt's account is what a payment run would need; wiring it into a
  Zahlungslauf is the Kasse's business and is not part of this screen.
- **The tax offices are not deduplicated against the bank master data.** A Finanzamt and a bank
  account that are the same account in the source are still two records here.
