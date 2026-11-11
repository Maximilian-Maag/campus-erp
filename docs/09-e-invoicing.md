# E-invoicing — XRechnung 3.0 (UBL 2.1)

A German public body must accept electronic invoices in **XRechnung**; as a
supplier it must be able to emit them. This is the first capability ported from
the productive source on the strength of `docs/08-reuse-inventory.md` §4.

## Why this shape

The productive the reference implementation models an e-invoice as the **EN16931 business groups**
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
| generator (second syntax) | `internal/core/einvoice.GenerateCII` → UN/CEFACT CII XML (ZUGFeRD/Factur-X) |
| persistence + service | `internal/modules/einvoice` (atomic insert, audited) |
| HTTP | `POST /api/e-invoices`, `GET /api/e-invoices/{id}`, `GET /api/e-invoices/{id}/xrechnung`, `GET /api/e-invoices/{id}/cii`, `POST /api/e-invoices/{id}/match` |

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
- the VAT category and the payment means code to be ones EN 16931 allows — validated against
  the code lists in this package (UNTDID 5305 / 4461), which the lookup catalogue is seeded
  from (`docs/11`), so an invoice that would not conform is refused rather than generated;
- **BR-CO-10**: each line net = quantity × unit price (round half away from zero);
- VAT breakdown derived per (category, rate) with the tax rounded half-up.

## ZUGFeRD / Factur-X (CII)

XRechnung (UBL) is what a German public body *receives*; **ZUGFeRD** is what a supplier is asked for
when the counterpart prefers a hybrid file. Both carry the EN 16931 business terms, so
`GenerateCII` is a **second rendering of the same model**, not a second model: it runs the same
`Validate`, so an invoice this system refuses to emit as XRechnung is refused here too, and the two
renderings are pinned by two golden files (`golden_invoice.xml`, `golden_invoice_cii.xml`).

What is shipped here is the **CII data part** — the document that would be embedded as
`factur-x.xml`. A ZUGFeRD file *proper* is a PDF/A-3 with that XML as an attachment; assembling a
PDF/A-3 is a PDF job of its own and is not part of this (below).

Two things the CII syntax says differently, and how this build treats them:

- the **guideline** (BT-24) is a parameter value, not a fixed tag: `CIIGuideline` is the EN 16931
  core guideline, and a counterpart that demands a ZUGFeRD profile id (for instance
  `urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:en16931`) states it rather than this file
  guessing at one;
- the **delivery group** (`ram:ApplicableHeaderTradeDelivery`) is part of the element sequence even
  when it carries nothing, so it is written empty rather than left out — an omitted group is not the
  same document as an empty one to a CII parser.

`GET /api/e-invoices/{id}/cii` serves it (`application/xml`), next to the UBL route, and the
*E-Rechnung* screen offers both from each row. No CII schema is committed — the standard's schemas
live outside the repo, exactly as the XRechnung one does — so the guard is the golden file, a
well-formedness test that walks every token, and the business terms the document must carry.

### Reading either syntax

Both directions exist now, and they are two halves of one thing: what this package emits must read
back into the model it was built from.

- `ParseXRechnung` reads UBL 2.1 — what a public body is sent.
- `ParseCII` reads UN/CEFACT CII — what a supplier's ERP often sends. Its element paths mirror the
  writer element for element, and it runs the same `Validate`, so a document that would be refused
  as an outbound invoice is refused here too.
- `ParseReceived` decides between them by the **root element** (`Invoice` or
  `CrossIndustryInvoice`) and refuses anything else, a PDF container included. The caller —
  `POST /api/e-invoices/import` — therefore does not have to know which syntax arrived; that is the
  whole point, since the counterpart, not this system, chooses it.

A round-trip test renders the sample invoice in CII, reads it back and compares every field that
carries business meaning; the import is covered end to end as well, and both syntaxes of one invoice
are checked to produce the same totals. Two details this reader decides on purpose:

- the **date** must be format `102` (YYYYMMDD), which is what EN 16931 documents carry — another
  format is refused rather than guessed at;
- only the **VAT registration** (`schemeID="VA"`, or none) becomes the party's VAT id: a commercial
  register number under `SpecifiedTaxRegistration` is a different identifier and is not read as a
  VAT number. A seller that carries only the latter is refused, because the seller VAT id is
  required.

## Inbound (Eingangsrechnung)

`core.ParseReceived` reads a received invoice back — UBL 2.1 (XRechnung) or CII (ZUGFeRD/Factur-X),
whichever the counterpart sent (see *Reading either syntax*) —
into the same model and runs the same `Validate`, so a document we accept is held
to the rules we emit. `InvoiceService.ImportReceived` stores it (audited,
`einvoice.receive`) and `POST /api/e-invoices/received` takes the raw XML.

## Matching to the purchase order

`InvoiceService.Match` runs the two-way match (`POST /api/e-invoices/{id}/match`): the
order is looked up by the **buyer reference** the document carries — BT-10 is what a public
buyer puts on the order and the supplier echoes back — its gross must equal the order's, and
the match converts that order's commitment into an actual (`docs/12-procurement.md`). The key
is deliberate: an invoice without a buyer reference cannot name an order and is refused
(`core.ErrBuyerReference`) rather than guessed at.

A matched invoice **records the order it was bound to** (`e_invoice.matched_order_id`), so
`GET /api/e-invoices` can say which invoices are still open. The audit stream alone would not
do: that is the trail, not state a screen can read. A second match is refused
(`ErrAlreadyMatched`) — the invoice, not the order's state, is the first thing that stops it.

A stored invoice also carries its **direction**: `inbound` for a received Eingangsrechnung,
`outbound` for one this system issued. Only an inbound one can be matched — a purchase order's
receipt answers a document coming in, not one going out — and the screen does not offer the
action for the others. The column arrived after the first invoices did, so the migration
backfills it from the audit stream: the rows whose trail shows `einvoice.receive` are corrected
to `inbound`. That is what the trail is for.

## Not yet done

- ~~ZUGFeRD/CII syntax~~ — done since: `GenerateCII` renders the same model as CII and
  `GET /api/e-invoices/{id}/cii` serves it (above). What remains of a ZUGFeRD *file* is the
  **PDF/A-3 embedding** — the data part exists, the container does not.
- The full KoSIT Schematron ruleset (we enforce the subset we can satisfy).
- **No KoSIT Schematron.** The rules enforced are the ones this package can satisfy (docs/09
  above); the full ruleset is a separate body of work, and it is the same gap for both syntaxes.
- ~~Reading a received **CII** document back is not implemented~~ — done since: `ParseCII` reads the
  CII syntax and `ParseReceived` dispatches on the root element, so an inbound document is read
  whichever syntax arrives (see *Reading either syntax*). What is still missing for a ZUGFeRD
  *file* is unwrapping the PDF/A-3 container to get at the data part — the same gap as the
  embedding above, from the other side.
