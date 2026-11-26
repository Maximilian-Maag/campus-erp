# Ubiquitous language

Generated from `docs/ubiquitous-language.json` by `tools/gen_language.py`.
Do not edit this page; edit the JSON and run `make language-gen`.

One concept, one name. The same word is used in the requirements, the C4
model, the API contract, the database, the code and the German UI copy. A
concept that needs two names is two concepts.

## Orthography

The same concept is spelled for its layer. Neither layer may mix the two.

| layer | canonical | never | why |
| --- | --- | --- | --- |
| prose | `centre` | `center` | Prose and UI copy are British English; a cost centre and a budget centre are spelled with -re. |
| code | `centre` | `center` | Identifiers, JSON and SQL use the same -re spelling as the prose, so a search for the concept finds one word in every layer. |

## Terms

| concept | German | definition | never |
| --- | --- | --- | --- |
| cash book day | Kassentag | One business date's cash book, opened and closed as a unit. | — |
| cash booking | Kassenbuchung | One income or expense movement in a cash book day. | — |
| payment order | Ausgabeanordnung | An instruction to pay one recipient one amount, approved before it is paid. | `Zahlungsanordnung` |
| payment run | Zahlungslauf | A batch of approved payment orders exported as one SEPA credit-transfer file. | — |
| collection run | Einzugslauf | A batch of receivables collected under mandates as one SEPA direct-debit file. | `Einzug`, `Einzüge` |
| mandate | SEPA-Mandat | A debtor's authorisation to collect, with its sequence type and reference. | — |
| receivable | Offener Posten | An amount owed by a debtor, collected by a collection run or settled another way. | `claim reference` |
| bank statement | Kontoauszug | A CAMT or MT940 file imported against a cash account. | — |
| return | Rückläufer | A collection or transfer the bank sent back, releasing the receivable it settled. | — |
| dunning notice | Mahnung | One reminder issued to a debtor at a dunning level. | — |
| dunning level | Mahnstufe | A step in the dunning chain, priced by amount bands. | — |
| foundation | Stiftung | A foundation whose funds are administered in its own books. | — |
| foundation book | Stiftungsbuch | One year's book of a foundation, carried over into the next year. | — |
| carry-over | Übertrag | Posting a foundation book's net as the next year's opening movement. | `carryforward`, `carry forward`, `rollover` |
| budget position | Haushaltsstelle | A budget line identified by year, chapter and title, carrying its allocation. | `Haushaltstitel` |
| budget source | Mittelquelle | Where a position's money comes from: a chapter and title, a funder or a foundation. | — |
| budget centre | Bewirtschaftungsstelle | Who administers a position's money. | — |
| commitment | Mittelbindung | An amount reserved on a budget position before it is spent. | — |
| purchase order | Bestellung | An order to a supplier, approved before goods are received. | — |
| goods receipt | Wareneingang | The recorded delivery of ordered goods, with the quantity received. | — |
| supplier | Lieferant | A party the product orders goods or services from. | — |
| incoming invoice | Eingangsrechnung | A supplier's invoice received and matched to an order. | — |
| e-invoice | E-Rechnung | A structured invoice in UBL, CII or ebInterface form. | `electronic invoice` |
| cost centre | Kostenstelle | An organisational unit that carries costs. | — |
| cost type | Kostenart | What a cost is: the nature of the expense. | — |
| cost unit | Kostenträger | The product, service or project a cost is carried to. | — |
| allocation | Umlage | Distributing a cost centre's costs onto receivers by a key. | — |
| service rate | Verrechnungssatz | The internal price per unit at which a cost centre bills its service. | — |
| accounting record | Buchung | One KLR movement booked on a cost centre. | — |
| report | Auswertung | A definition plus its rows, rendered as a file or a screen. | — |
| artefact | Artefakt | A stored file produced by a report, export or document render. | — |
| lookup | Nachschlagewert | A value in a namespace the product reads but does not own. | — |
| role | Rolle | A permission set a principal holds. | — |
| principal | Identität | The authenticated identity a request carries. | — |

## Where it is enforced

`tools/check_language.py` reads the JSON and scans every tracked text file.
It runs as `make language` and in CI. Prose is scanned with code spans and
fenced blocks removed, so an endpoint such as `/api/cost-centres` is not
mistaken for prose. The historical migrations and the legacy analysis keep
their original spelling: they describe the system this product replaces.
