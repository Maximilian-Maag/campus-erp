# KLR — Kosten- und Leistungsrechnung

Cost accounting: cost centres (Kostenstellen), cost units (Kostenträger) and
append-only accounting records, with a per-cost-centre report and a distribution
engine that moves a centre's costs onto its receivers. Mirrors the vendor's
`cost_center` / `cost_unit` / `accounting_record` (`docs/08` §3); the legacy side
is the `cob` schema (`docs/03` §3).

## Model

| table | role |
|---|---|
| `cost_center` | Kostenstelle (number UNIQUE, name) |
| `cost_unit` | Kostenträger (number UNIQUE, name) |
| `accounting_record` | append-only posting: cost centre, optional cost unit, kind `cost`/`revenue`, amount, business date |
| `allocation` | one distribution key: source centre, receiving centre, share, validity |
| `allocation_run` | one distribution of a centre's period onto its receivers |

Amounts are integer cents. A correction is a compensating record, never an edit.

A record carries a **business date** (the period it belongs to) as well as the
moment it was entered. The two differ in the normal case: the month's costs are
entered after the month ended, and that is the period they must be distributed
over.

## Report

```
costs    = Σ amount where kind = 'cost'
revenues = Σ amount where kind = 'revenue'
balance  = revenues − costs
```

The report is scoped to one cost centre, so one cost centre never sees another's
records (tested).

The same figures reach the report catalogue: `klr-cost-center-overview` (Kostenstellenübersicht,
docs/25) is every cost centre with its Kosten, Erlöse and Saldo and a totals row, rendered through
the same worker as the other reports.

## Distribution (Umlage)

A cost centre's costs are distributed over its receivers by **keys**, which are
data — the vendor's `overhead_calc*` / `allocation*` shape is not published, so a
key is a row (source centre, receiving centre, share in basis points, validity)
and an operator changes a distribution without the engine moving. The reasoning
is the same as for the per-state statistics layouts (ADR-0012).

Running a distribution for a period (`from`..`to`, both inclusive):

1. The keys **in force at the period's end** are the ones that distribute it.
2. Their shares must add up to exactly 10000 basis points. Less is refused rather
   than silently under-distributing; more is refused by the table as well.
3. The source's costs **in that period** (by business date) are split by the
   shares. The split is exact: each receiver gets its share rounded down and the
   **last one takes the remainder**, so no cent is created or lost — a share that
   rounds to nothing books nothing (a record's amount is always positive).
4. Each receiver is charged a `cost`; the source is relieved by the same total
   through a `revenue`. The records carry the run's reference (`UML:<run>`), so a
   distributed cost is traceable in the report.

The source's balance therefore nets to zero and **the sum of balances across the
centres is unchanged** — a distribution moves cost, it neither creates nor
destroys any (tested). The cost *volume* does rise by the allocated amount, since
the receivers' records are new and the source's are not removed: records are
append-only.

One run per cost centre and period, enforced by an exclusion constraint over the
period — **overlapping** windows are refused, not merely identical ones —
because two runs over the same days would hand out the same costs twice. The
records and the run are written in one transaction, so a run is never half
booked, and a refused run leaves the books untouched.

Receivers are **cost centres**. Distributing onto a cost *unit* (Kostenträger) is
a separate step: a record always belongs to a cost centre, so attributing a cost
to a unit goes through the Kostenträgerrechnung, not through this engine, and no
column for it was added here.

## API

| method | path |
|---|---|
| `POST` | `/api/klr/cost-centers` |
| `GET`  | `/api/klr/cost-centers` |
| `POST` | `/api/klr/cost-units` |
| `POST` | `/api/klr/cost-centers/{id}/records` |
| `GET`  | `/api/klr/cost-centers/{id}/report` |
| `POST` | `/api/klr/cost-centers/{id}/allocations` |
| `GET`  | `/api/klr/cost-centers/{id}/allocations` |
| `POST` | `/api/klr/cost-centers/{id}/allocations/run` |
| `POST` | `/api/klr/cost-centers/{id}/cost-unit-keys` |
| `GET`  | `/api/klr/cost-centers/{id}/cost-unit-keys` |
| `POST` | `/api/klr/cost-centers/{id}/cost-unit-keys/run` |
| `POST` | `/api/klr/service-rates` |
| `GET`  | `/api/klr/service-rates` |
| `POST` | `/api/klr/service-charges` |
| `POST` | `/api/klr/cost-types` |
| `GET`  | `/api/klr/cost-types` |
| `POST` | `/api/klr/cost-types/{id}/accounts` |
| `GET`  | `/api/klr/cost-types/{id}/accounts` |
| `GET`  | `/api/klr/accounts/{account}/cost-type` |

## Cost types and the account bridge

A cost *type* (Kostenart) says what a cost **is**; a cost *centre* says where it
arose. The catalogue is small and its code is its identity — creating a second
cost type with a code that is already in use is a conflict, not a second row.

The bridge from the ledger's accounts to the cost types is the piece the vendor
never published. It is modelled as **data**, not code: a cost type owns a set of
account codes, and an account code is a plain string. When the ledger side
lands, its codes are the ones the bridge already holds, so the mapping is
configured rather than rewritten.

The bridge is a **function, not a bag of links**: an account belongs to exactly
one cost type. Mapping the same code to the same cost type again is idempotent
(it returns the existing link); mapping it to a *different* cost type is
refused. The read side answers both directions — the accounts a cost type owns,
and the cost type an account belongs to (`/api/klr/accounts/{account}/cost-type`;
that path sits on the account because `/api/klr/cost-types/by-account/{account}`
would be as specific as `/api/klr/cost-types/{id}/accounts` and the router
refuses to guess between two equally specific patterns).

A record may name its cost type (`cost_type_id`), and a record with none is
valid — nobody has classified it yet. A cost type that does not exist is
refused by the store, not silently dropped.

The frontend has one screen for this, *Kostenarten* (`/klr/kostenarten`): it
lists the catalogue, creates entries, shows the accounts of the chosen cost type
and maps a new one, and looks an account up in the other direction. An empty
catalogue still offers the form, so the first cost type can be created from the
screen; and an account nobody has mapped shows as an answer ("keine Kostenart
hinterlegt"), not as a failure — the API's 404 is the answer.

The distribution has its own screen, *Umlage* (`/klr/umlage`): it picks a cost centre, shows
its keys, defines one (the share is typed as a percentage and stored as basis points), and runs
a period. The rule that a centre's shares must add up to 100 % stays with the API — the screen
does not re-derive it.

A record's `business_date` may be left out, in which case the API takes today —
the edge decides what "today" is, and the domain is always given a date. Every
write is audited (`cost_center.create`, `cost_unit.create`,
`accounting_record.create`, `allocation.define`, `allocation.run`,
`cost_type.create`, `cost_type.map_account`, `cost_unit_key.define`,
`cost_unit_run.run`, `service_rate.define`, `service_charge.record`), and
`allocation.run` names the period, the amount distributed and the number of
receivers.

## Kostenträgerrechnung: attributing a centre's costs to its units

The same distribution with the other kind of receiver. `cost_unit_key` says how much of a cost
centre's costs a cost unit takes, `cost_unit_run` records a run, and the run books one cost on
the source centre tagged with each unit that takes a share, plus a revenue relieving the source
by exactly what it handed out.

**The source centre's balance does not move.** The tagged costs and the relief cancel, so
attributing a centre's costs leaves the centre's Saldo where it was and the unit now carries
them — a run still neither creates nor destroys cost. Records are append-only, so the cost
*volume* rises by the attributed amount; the balances do not.

A run over a period with no key in force is refused (`ErrAllocationRunNoTargets`) and so is a
second run over the same or an overlapping period (`ErrAlreadyAllocated`, from the table's
exclusion constraint). A key whose validity has already closed does not distribute a later
period.

Its records carry the reference `KTU:<run id>`, distinct from the centre-side run's `UML:<run id>`,
so a reference always says which engine wrote it.

The frontend has one screen for this, *Kostenträgerrechnung* (`/klr/kostentraeger`): it picks a
cost centre, shows its keys, defines one (the share is typed as a percentage and stored as basis
points), and runs a period. What the shares must add up to stays with the API.

## Internal pricing: pricing a service and charging it out

The two engines above move *actual* costs: they take what a centre spent and hand it on. Internal
pricing is the other way round — a service is **priced** and then charged out. `service_rate` is
the price of one unit of a cost type's service over a period, and a charge prices a quantity at
the rate **in force on the day the service was rendered**.

Where several rates cover the day, the most recently started one wins. That is what lets a rate
change take effect without deleting the old one: the old rate keeps its history, and a charge made
under it is unaffected, because **the amount is computed once and stored** rather than re-derived
from today's prices. A charge that was made is a fact about the books.

The provider is relieved by exactly what the receiver is charged, so a charge moves cost between
the two and changes no total — the same rule as the key-based engines. A charge for no units, a
centre charging itself, and a day no rate covers are all refused. Its records carry the reference
`LEI:<charge id>`.

## Why "clearing" is two things here

`docs/08` lists the vendor's `cost_center_clearing` and `cost_unit_clearing` next to the
`allocation*` engine, and `docs/03` reads the legacy `kpum` / `um*` tables as
"Umlage/Verrechnung" — the same apportionment. So the two clearing tables are taken to be the two
sides of one distribution: the centre side is `allocation` + `allocation_run`, the unit side is
`cost_unit_key` + `cost_unit_run`. The vendor's exact shape is not published, so as everywhere
else in this package the mapping is data an operator can change rather than code.

## Not yet done

- The KLR has no screen for internal pricing yet; the rates and charges are driven through the API.
