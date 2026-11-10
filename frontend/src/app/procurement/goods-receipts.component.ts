import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, ProcurementOrderSummary, ReceiptState } from '../core/api/generated';
import { formatCents, formatDay, formatMilli, isoDay, quantityToMilli } from '../core/format';

/**
 * Procurement — Wareneingang (goods receipts).
 *
 * Records what actually arrived on an approved order, line by line, which is what makes a partial
 * delivery possible: the whole-order match can only convert an order in full (`docs/12`). The
 * screen picks an order, shows every line's ordered / received / open quantity as the server
 * derived it, and books the quantities that arrived.
 *
 * A quantity is typed in units and sent in milli-units (`quantityToMilli`), like an amount typed
 * in euro and sent in cents. Only the lines that are still open get an input, and a blank input
 * means "nothing of this line" — so a delivery of one line of a three-line order is one number,
 * not three.
 */
@Component({
  selector: 'app-goods-receipts',
  template: `
    <section aria-labelledby="receipts-heading">
      <h2 id="receipts-heading">Wareneingang</h2>

      @if (loadError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }
      @if (actionError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }
      @if (notice(); as n) {
        <p role="status" class="notice">{{ n }}</p>
      }

      @if (loadingOrders()) {
        <p role="status">Bestellungen werden geladen.</p>
      } @else if (orders().length === 0 && !loadError()) {
        <p role="status">Es ist keine freigegebene Bestellung offen.</p>
      } @else if (!loadError()) {
        <div class="picker">
          <label for="receipt-order">Bestellung</label>
          <select id="receipt-order" [value]="order()" (change)="choose($any($event.target).value)">
            <option value="">Bitte wählen</option>
            @for (o of orders(); track o.ID) {
              <option [value]="o.ID">{{ o.Number }} — {{ o.Supplier }}</option>
            }
          </select>
        </div>
      }

      @if (state(); as st) {
        <p>
          Bestellung {{ selected()?.Number }} von {{ selected()?.Supplier }}, Auftragssumme
          {{ amount(selected()?.GrossCents ?? 0) }}.
        </p>

        <table>
          <caption>
            Positionen mit bestellter, gelieferter und offener Menge
          </caption>
          <thead>
            <tr>
              <th scope="col">Position</th>
              <th scope="col">Beschreibung</th>
              <th scope="col" class="quantity">Bestellt</th>
              <th scope="col" class="quantity">Geliefert</th>
              <th scope="col" class="quantity">Offen</th>
            </tr>
          </thead>
          <tbody>
            @for (p of st.Progress; track p.LineNo) {
              <tr [attr.data-testid]="'line-' + p.LineNo">
                <td>{{ p.LineNo }}</td>
                <td>{{ p.Description }}</td>
                <td class="quantity">{{ quantity(p.OrderedMilli) }}</td>
                <td class="quantity">{{ quantity(p.ReceivedMilli) }}</td>
                <td class="quantity">{{ quantity(open(p)) }}</td>
              </tr>
            }
          </tbody>
        </table>

        @if (st.Complete) {
          <p role="status">Die Bestellung ist vollständig geliefert.</p>
        } @else {
          <form (submit)="record($event)">
            <p>
              <label for="receipt-reference">Lieferschein</label>
              <input
                id="receipt-reference"
                [value]="reference()"
                (input)="reference.set($any($event.target).value)"
              />
            </p>
            <p>
              <label for="receipt-date">Datum</label>
              <input
                id="receipt-date"
                type="date"
                [value]="receivedOn()"
                (input)="receivedOn.set($any($event.target).value)"
              />
            </p>
            @for (p of openLines(); track p.LineNo) {
              <p>
                <label [attr.for]="'receipt-line-' + p.LineNo">
                  Menge Position {{ p.LineNo }} (offen {{ quantity(open(p)) }})
                </label>
                <input
                  [id]="'receipt-line-' + p.LineNo"
                  [attr.data-testid]="'quantity-' + p.LineNo"
                  [value]="quantities()[p.LineNo] ?? ''"
                  (input)="setQuantity(p.LineNo, $any($event.target).value)"
                />
              </p>
            }
            <button type="submit" [disabled]="busy()">
              {{ busy() ? 'Wareneingang wird gebucht.' : 'Wareneingang buchen' }}
            </button>
          </form>
        }

        @if (st.Receipts.length > 0) {
          <table>
            <caption>
              Gebuchte Wareneingänge
            </caption>
            <thead>
              <tr>
                <th scope="col">Lieferschein</th>
                <th scope="col">Datum</th>
                <th scope="col">Positionen</th>
                <th scope="col" class="amount">Wert</th>
              </tr>
            </thead>
            <tbody>
              @for (r of st.Receipts; track r.ID) {
                <tr [attr.data-testid]="'receipt-' + r.ID">
                  <td>{{ r.Reference }}</td>
                  <td>{{ day(r.ReceivedOn) }}</td>
                  <td>{{ lineList(r) }}</td>
                  <td class="amount">{{ amount(r.ReceivedCents) }}</td>
                </tr>
              }
            </tbody>
          </table>
        }
      }
    </section>
  `,
})
export class GoodsReceiptsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly orders = signal<ProcurementOrderSummary[]>([]);
  readonly loadingOrders = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  readonly order = signal('');
  readonly state = signal<ReceiptState | null>(null);
  readonly busy = signal(false);
  readonly reference = signal('');
  readonly receivedOn = signal(isoDay(new Date()));
  /** What the operator typed per line number; a line with no entry has nothing typed yet. */
  readonly quantities = signal<Record<number, string | undefined>>({});

  /** Loads the approved orders a delivery can be booked against. */
  ngOnInit(): void {
    this.loadOrders();
  }

  private loadOrders(): void {
    this.loadingOrders.set(true);
    this.loadError.set(null);
    this.api.listPurchaseOrders('approved').subscribe({
      next: (orders) => {
        this.orders.set(orders);
        this.loadingOrders.set(false);
      },
      error: () => {
        this.loadError.set('Die Bestellungen konnten nicht geladen werden.');
        this.loadingOrders.set(false);
      },
    });
  }

  /** Chooses an order and reads its arrival state. */
  choose(value: string): void {
    this.order.set(value);
    this.state.set(null);
    this.actionError.set(null);
    this.notice.set(null);
    this.reference.set('');
    this.quantities.set({});
    if (value === '') {
      return;
    }
    this.loadState(Number(value));
  }

  private loadState(id: number): void {
    this.api.listGoodsReceipts(id).subscribe({
      next: (state) => this.state.set(state),
      error: () => this.loadError.set('Der Wareneingang konnte nicht geladen werden.'),
    });
  }

  /** Books the quantities that arrived. */
  record(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);

    const id = Number(this.order());
    if (id <= 0) {
      this.actionError.set('Es ist keine Bestellung gewählt.');
      return;
    }
    if (this.reference().trim() === '') {
      this.actionError.set('Ein Lieferschein ist erforderlich.');
      return;
    }
    if (!/^\d{4}-\d{2}-\d{2}$/.test(this.receivedOn())) {
      this.actionError.set('Ein Datum ist erforderlich.');
      return;
    }
    const lines: { line_no: number; quantity_milli: number }[] = [];
    for (const p of this.openLines()) {
      const typed = (this.quantities()[p.LineNo] ?? '').trim();
      if (typed === '') {
        continue;
      }
      const milli = quantityToMilli(typed);
      if (milli === null) {
        this.actionError.set(`Die Menge der Position ${p.LineNo} ist keine gültige Menge.`);
        return;
      }
      lines.push({ line_no: p.LineNo, quantity_milli: milli });
    }
    if (lines.length === 0) {
      this.actionError.set('Es ist keine Menge erfasst.');
      return;
    }

    this.busy.set(true);
    this.api
      .recordGoodsReceipt(id, { reference: this.reference(), received_on: this.receivedOn(), lines })
      .subscribe({
        next: (result) => {
          this.state.set(result.State);
          this.quantities.set({});
          this.reference.set('');
          this.notice.set(
            result.State.Complete
              ? `Wareneingang gebucht (${this.amount(result.Receipt.ReceivedCents)}). Die Bestellung ist vollständig geliefert.`
              : `Wareneingang gebucht (${this.amount(result.Receipt.ReceivedCents)}).`
          );
          this.busy.set(false);
          // An order that arrived in full leaves the approved list, so the picker is read again.
          this.loadOrders();
        },
        error: () => {
          this.actionError.set('Der Wareneingang konnte nicht gebucht werden.');
          this.busy.set(false);
        },
      });
  }

  setQuantity(lineNo: number, value: string): void {
    this.quantities.update((current) => ({ ...current, [lineNo]: value }));
  }

  /** The lines an operator may still deliver: those with something open. */
  openLines() {
    return (this.state()?.Progress ?? []).filter((p) => this.open(p) > 0);
  }

  /** What is still open on a line: what was ordered less what arrived. */
  open(p: { OrderedMilli: number; ReceivedMilli: number }): number {
    return Math.max(0, p.OrderedMilli - p.ReceivedMilli);
  }

  selected(): ProcurementOrderSummary | undefined {
    const id = Number(this.order());
    return this.orders().find((o) => o.ID === id);
  }

  /** The lines of a receipt as the operator reads them ("1: 0,5; 2: 2"). */
  lineList(r: { Lines: { LineNo: number; QuantityMilli: number }[] }): string {
    return r.Lines.map((l) => `${l.LineNo}: ${formatMilli(l.QuantityMilli)}`).join('; ');
  }

  amount(cents: number): string {
    return formatCents(cents);
  }

  day(iso: string): string {
    return formatDay(iso);
  }

  quantity(milli: number): string {
    return formatMilli(milli);
  }
}
