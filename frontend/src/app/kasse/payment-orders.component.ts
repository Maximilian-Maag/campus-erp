import { Component, OnInit, inject, signal } from '@angular/core';
import { Observable } from 'rxjs';
import { CampusErpApi, KasseOrderSummary } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';

/**
 * Kasse — Zahlungsanordnungen (payment orders).
 *
 * Lists the payment orders soonest due first and drives their workflow: a draft is submitted,
 * a submitted order is approved, rejected (with a reason) or cancelled. The status filter is the
 * server's, so choosing one asks the API for that status rather than sifting what happens to be
 * loaded.
 *
 * The rules of the workflow — that only a draft may be submitted, that approval is four-eyes —
 * stay with the API. The screen offers the actions a state allows and reports a refusal as it
 * comes back, rather than re-deriving a backend guard in TypeScript.
 */
@Component({
  selector: 'app-payment-orders',
  template: `
    <section aria-labelledby="orders-heading">
      <h2 id="orders-heading">Zahlungsanordnungen</h2>

      <div class="filter">
        <label for="order-status">Status</label>
        <select id="order-status" [value]="status()" (change)="filterTo($any($event.target).value)">
          <option value="">Alle</option>
          <option value="draft">Entwurf</option>
          <option value="submitted">Vorgelegt</option>
          <option value="approved">Genehmigt</option>
          <option value="rejected">Abgelehnt</option>
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
        <p role="status">Zahlungsanordnungen werden geladen.</p>
      } @else if (orders().length === 0 && !loadError()) {
        <p role="status">
          {{ status() ? 'Keine Anordnung in diesem Status.' : 'Es sind keine Zahlungsanordnungen angelegt.' }}
        </p>
      } @else if (!loadError()) {
        <table>
          <caption>
            Zahlungsanordnungen mit Verwendungszweck, Betrag, Fälligkeit und Status
          </caption>
          <thead>
            <tr>
              <th scope="col">Referenz</th>
              <th scope="col">Verwendungszweck</th>
              <th scope="col" class="amount">Betrag</th>
              <th scope="col">Fällig</th>
              <th scope="col">Status</th>
              <th scope="col">Aktion</th>
            </tr>
          </thead>
          <tbody>
            @for (order of orders(); track order.ID) {
              <tr [attr.data-testid]="'order-' + order.Reference">
                <td>{{ order.Reference }}</td>
                <td>{{ order.Purpose }}</td>
                <td class="amount">{{ euro(order.AmountCents) }}</td>
                <td>{{ day(order.DueDate) }}</td>
                <td [attr.data-testid]="'status-' + order.Reference">{{ statusLabel(order.Status) }}</td>
                <td>
                  @if (order.Status === 'draft') {
                    <button
                      type="button"
                      [disabled]="busy() !== null"
                      [attr.aria-label]="'Anordnung ' + order.Reference + ' vorlegen'"
                      (click)="submit(order)"
                    >
                      Vorlegen
                    </button>
                  }
                  @if (order.Status === 'submitted') {
                    <button
                      type="button"
                      [disabled]="busy() !== null"
                      [attr.aria-label]="'Anordnung ' + order.Reference + ' genehmigen'"
                      (click)="approve(order)"
                    >
                      Genehmigen
                    </button>
                    <button
                      type="button"
                      [disabled]="busy() !== null"
                      [attr.aria-label]="'Anordnung ' + order.Reference + ' ablehnen'"
                      (click)="startReject(order)"
                    >
                      Ablehnen
                    </button>
                  }
                  @if (order.Status === 'draft' || order.Status === 'submitted') {
                    <button
                      type="button"
                      [disabled]="busy() !== null"
                      [attr.aria-label]="'Anordnung ' + order.Reference + ' stornieren'"
                      (click)="cancel(order)"
                    >
                      Stornieren
                    </button>
                  }
                  @if (rejecting() === order.ID) {
                    <div class="reject">
                      <label [attr.for]="'reason-' + order.ID">Grund</label>
                      <input
                        [id]="'reason-' + order.ID"
                        type="text"
                        [value]="reason()"
                        (input)="reason.set($any($event.target).value)"
                      />
                      <button type="button" [disabled]="busy() !== null" (click)="reject(order)">
                        Ablehnung bestätigen
                      </button>
                      <button type="button" (click)="stopReject()">Abbrechen</button>
                    </div>
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
export class PaymentOrdersComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly orders = signal<KasseOrderSummary[]>([]);
  readonly status = signal('');
  readonly loading = signal(true);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  /** The order an action is running for, so its buttons can report progress and block a double submit. */
  readonly busy = signal<number | null>(null);
  /** The order whose rejection is being given a reason. */
  readonly rejecting = signal<number | null>(null);
  readonly reason = signal('');

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
    this.api.listPaymentOrders(this.status() || undefined).subscribe({
      next: (orders) => {
        this.orders.set(orders ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Konnte die Zahlungsanordnungen nicht laden.');
        this.loading.set(false);
      },
    });
  }

  submit(order: KasseOrderSummary): void {
    this.act(order, 'vorlegen', this.api.submitPaymentOrder(order.ID));
  }

  approve(order: KasseOrderSummary): void {
    this.act(order, 'genehmigen', this.api.approvePaymentOrder(order.ID));
  }

  cancel(order: KasseOrderSummary): void {
    this.act(order, 'stornieren', this.api.cancelPaymentOrder(order.ID));
  }

  /** Rejection needs a reason, so it is a second step rather than a stray click. */
  startReject(order: KasseOrderSummary): void {
    this.rejecting.set(order.ID);
    this.reason.set('');
    this.actionError.set(null);
  }

  stopReject(): void {
    this.rejecting.set(null);
    this.reason.set('');
  }

  reject(order: KasseOrderSummary): void {
    const reason = this.reason().trim();
    if (reason === '') {
      this.actionError.set('Für eine Ablehnung ist ein Grund erforderlich.');
      return;
    }
    // A sent rejection closes the reason box; a refused one keeps it, so the reason is still
    // there to correct and send again.
    this.act(order, 'ablehnen', this.api.rejectPaymentOrder(order.ID, { reason }), () => {
      this.rejecting.set(null);
      this.reason.set('');
    });
  }

  /**
   * Runs a workflow action and reloads, so the row shows the state the server now holds. `done`
   * runs only on success, which is how a rejection closes its reason box without any other
   * action touching one.
   */
  private act(order: KasseOrderSummary, verb: string, request: Observable<unknown>, done?: () => void): void {
    this.busy.set(order.ID);
    this.actionError.set(null);
    request.subscribe({
      next: () => {
        this.busy.set(null);
        if (done) {
          done();
        }
        this.load();
      },
      error: () => {
        this.busy.set(null);
        this.actionError.set('Konnte die Anordnung ' + order.Reference + ' nicht ' + verb + '.');
      },
    });
  }

  /** The workflow state in the words the screen uses. */
  statusLabel(status: string): string {
    switch (status) {
      case 'draft':
        return 'Entwurf';
      case 'submitted':
        return 'Vorgelegt';
      case 'approved':
        return 'Genehmigt';
      case 'rejected':
        return 'Abgelehnt';
      case 'cancelled':
        return 'Storniert';
      default:
        return status;
    }
  }

  /** Amounts and dates are rendered by the shared formatters, not by each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;
}
