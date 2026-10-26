# E-invoicing — XRechnung 3.0 (UBL 2.1)

A German public body must accept electronic invoices in **XRechnung**; as a
supplier it must be able to emit them. This is the first capability ported from
the productive source on the strength of `docs/08-reuse-inventory.md` §4.

## Why this shape

The productive HISinOne models an e-invoice as the **EN16931 business groups**
(`einvoice_metadata`, `einvoice_seller` (BG-4), `einvoice_buyer` (BG-7),
`einvoice_line` (BG-25), `einvoice_vat_breakdown` (BG-23), …) and ships the full
XRechnung 3.0 UBL schema plus a worked example. We keep the same shape with our
conventions (integer cents, basis-point rates, milli-unit quantities, declared
FKs, enum checks) — `migrations/0011_e_invoice.sql`.

## What we built

| piece | where |
|---|---|
| canonical tables | `e_invoice`, `e_invoice_party`, `e_invoice_line`, `e_invoice_vat` |
| model + rules | `internal/core/einvoice` (`Validate`, `ComputeTotals`, `VATGroups`) |
| generator | `internal/core/einvoice.GenerateXRechnung` → UBL 2.1 XML |
| persistence + service | `internal/modules/einvoice` (atomic insert, audited) |
| HTTP | `POST /api/e-invoices`, `GET /api/e-invoices/{id}`, `GET /api/e-invoices/{id}/xrechnung` |

The generator is deterministic and is pinned by a golden file
(`internal/core/einvoice/testdata/golden_invoice.xml`) that **validates against
the productive XRechnung 3.0 `UBL-Invoice-2.1.xsd`** (the schema is proprietary
and lives outside the repo; the test runs when `ERP_XRECHNUNG_XSD` points at it).

## Rules enforced

We go beyond "well-formed": the document carries the XRechnung identification
(`urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0`) and we
enforce the mandatory fields a German public-sector invoice needs:

- **BR-DE-15** buyer reference (Leitweg-ID), **BR-DE-16** seller VAT id,
  **BR-DE-7** seller contact e-mail;
- seller and buyer postal address (name, city, post code, country);
- a payment means code and, for a credit transfer, the payee IBAN;
- **BR-CO-10**: each line net = quantity × unit price (round half away from zero);
- VAT breakdown derived per (category, rate) with the tax rounded half-up.

## Not yet done

- Inbound (Eingangsrechnung) parsing/validation of a received XRechnung/ZUGFeRD.
- ZUGFeRD/CII syntax (the vendor ships it too; UBL is the public-sector format).
- The full KoSIT Schematron ruleset (we enforce the subset we can satisfy).
