import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, ProcurementOrderSummary } from '../core/api/generated';
import { formatCents } from '../core/format';

/**
 * Procurement — purchase orders (Bestellungen).
 *
 * Lists the orders and approves one that is still a draft, which is the step that commits
 * budget. The status filter is the server's own: choosing one asks the API for that status
 * rather than sifting what happens to be loaded, so the screen works on a list it never holds
 * in full.
 */
@Component({
  selector: 'app-purchase-orders',
  template: `
    <section aria-labelledby="orders-heading">
      <h2 id="orders-heading">Bestellungen</h2>

      <div class="filter">
        <label for="order-status">Status</label>
        <select id="order-status" [value]="status()" (change)="filterTo($any($event.target).value)">
          <option value="">Alle</option>
          <option value="draft">Entwurf</option>
          <option value="approved">Genehmigt</option>
          <option value="received">Erhalten</option>
          <option value="cancelled">Storniert</option>
        </select>
      </div>

      @if (loadError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }
      @if (actionError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }

      @if (loading()) {
        <p role="status">Bestellungen werden geladen.</p>
      } @else if (orders().length === 0 && !loadError()) {
        <p role="status">
          {{ status() ? 'Keine Bestellung in diesem Status.' : 'Es sind keine Bestellungen angelegt.' }}
        </p>
      } @else if (!loadError()) {
        <table>
          <caption>
            Bestellungen mit Lieferant, Betrag und Status
          </caption>
          <thead>
            <tr>
              <th scope="col">Nummer</th>
              <th scope="col">Lieferant</th>
              <th scope="col" class="amount">Betrag</th>
              <th scope="col">Status</th>
              <th scope="col">Aktion</th>
            </tr>
          </thead>
          <tbody>
            @for (order of orders(); track order.ID) {
              <tr [attr.data-testid]="'order-' + order.Number">
                <td>{{ order.Number }}</td>
                <td>{{ order.Supplier }}</td>
                <td class="amount">{{ euro(order.GrossCents) }}</td>
                <td [attr.data-testid]="'status-' + order.Number">{{ statusLabel(order.Status) }}</td>
                <td>
                  @if (order.Status === 'draft') {
                    <button
                      type="button"
                      [disabled]="approving() !== null"
                      [attr.aria-label]="'Bestellung ' + order.Number + ' genehmigen'"
                      (click)="approve(order)"
                    >
                      Genehmigen
                    </button>
                  }
                </td>
              </tr>
            }
          </tbody>
        </table>
      }
    </section>
  `,
})
export class PurchaseOrdersComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly orders = signal<ProcurementOrderSummary[]>([]);
  readonly status = signal('');
  readonly loading = signal(true);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  /** The order being approved, so its button can report progress and block a double submit. */
  readonly approving = signal<number | null>(null);

  // Loading starts when the screen is shown, not when it is constructed: the initial state is
  // then observable (and testable) before any request is made.
  ngOnInit(): void {
    this.load();
  }

  /** Narrow to one status ("" for all). The server does the filtering. */
  filterTo(status: string): void {
    this.status.set(status);
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listPurchaseOrders(this.status() || undefined).subscribe({
      next: (orders) => {
        this.orders.set(orders ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Konnte die Bestellungen nicht laden.');
        this.loading.set(false);
      },
    });
  }

  approve(order: ProcurementOrderSummary): void {
    this.approving.set(order.ID);
    this.actionError.set(null);
    this.api.approvePurchaseOrder(order.ID).subscribe({
      next: () => {
        this.approving.set(null);
        this.load();
      },
      error: () => {
        this.approving.set(null);
        this.actionError.set('Konnte die Bestellung ' + order.Number + ' nicht genehmigen.');
      },
    });
  }

  /** The lifecycle state in the words the screen uses. */
  statusLabel(status: string): string {
    switch (status) {
      case 'draft':
        return 'Entwurf';
      case 'approved':
        return 'Genehmigt';
      case 'received':
        return 'Erhalten';
      case 'cancelled':
        return 'Storniert';
      default:
        return status;
    }
  }

  /** Amounts are rendered by the shared formatter, not by each screen's own. */
  readonly euro = formatCents;
}
