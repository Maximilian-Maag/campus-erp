import { Component, OnInit, inject, signal } from '@angular/core';
import { Observable } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, KasseOrderSummary } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * Kasse — Ausgabeanordnungen (payment orders).
 *
 * Lists the payment orders soonest due first and drives their workflow: a draft is submitted,
 * a submitted order is approved, rejected (with a reason) or cancelled. The status filter is the
 * server's, so choosing one asks the API for that status rather than sifting what happens to be
 * loaded.
 *
 * The rules of the workflow — that only a draft may be submitted, that approval is four-eyes —
 * stay with the API. The screen offers the actions a state allows and reports a refusal as it
 * comes back, rather than re-deriving a backend guard in TypeScript. The states are named with
 * the shared vocabulary's status keys, so a workflow state reads the same here as anywhere else.
 */
@Component({
  selector: 'app-payment-orders',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.kasseAnordnungen.title" subtitleKey="page.kasseAnordnungen.subtitle">
      <section>
        <div class="filter">
          <label for="order-status">{{ 'common.status' | transloco }}</label>
          <select id="order-status" [value]="status()" (change)="filterTo($any($event.target).value)">
            <option value="">{{ 'common.all' | transloco }}</option>
            <option value="draft">{{ 'status.draft' | transloco }}</option>
            <option value="submitted">{{ 'status.submitted' | transloco }}</option>
            <option value="approved">{{ 'status.approved' | transloco }}</option>
            <option value="rejected">{{ 'status.rejected' | transloco }}</option>
            <option value="cancelled">{{ 'status.cancelled' | transloco }}</option>
          </select>
        </div>

        @if (loadError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (actionError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }

        @if (loading()) {
          <p role="status">{{ 'kasse.orders.loading' | transloco }}</p>
        } @else if (orders().length === 0 && !loadError()) {
          <p role="status">
            {{ (status() ? 'kasse.orders.emptyFiltered' : 'kasse.orders.empty') | transloco }}
          </p>
        } @else if (!loadError()) {
          <table>
            <caption>
              {{ 'kasse.orders.caption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.reference' | transloco }}</th>
                <th scope="col">{{ 'common.purpose' | transloco }}</th>
                <th scope="col" class="amount">{{ 'common.amount' | transloco }}</th>
                <th scope="col">{{ 'common.due' | transloco }}</th>
                <th scope="col">{{ 'common.status' | transloco }}</th>
                <th scope="col">{{ 'common.action' | transloco }}</th>
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
                        [attr.aria-label]="'kasse.orders.submitLabel' | transloco: { reference: order.Reference }"
                        (click)="submit(order)"
                      >
                        {{ 'kasse.orders.submit' | transloco }}
                      </button>
                    }
                    @if (order.Status === 'submitted') {
                      <button
                        type="button"
                        [disabled]="busy() !== null"
                        [attr.aria-label]="'kasse.orders.approveLabel' | transloco: { reference: order.Reference }"
                        (click)="approve(order)"
                      >
                        {{ 'kasse.orders.approve' | transloco }}
                      </button>
                      <button
                        type="button"
                        [disabled]="busy() !== null"
                        [attr.aria-label]="'kasse.orders.rejectLabel' | transloco: { reference: order.Reference }"
                        (click)="startReject(order)"
                      >
                        {{ 'kasse.orders.reject' | transloco }}
                      </button>
                    }
                    @if (order.Status === 'draft' || order.Status === 'submitted') {
                      <button
                        type="button"
                        [disabled]="busy() !== null"
                        [attr.aria-label]="'kasse.orders.cancelLabel' | transloco: { reference: order.Reference }"
                        (click)="cancel(order)"
                      >
                        {{ 'common.cancel' | transloco }}
                      </button>
                    }
                    @if (rejecting() === order.ID) {
                      <div class="reject">
                        <label [attr.for]="'reason-' + order.ID">{{ 'kasse.orders.reason' | transloco }}</label>
                        <input
                          [id]="'reason-' + order.ID"
                          type="text"
                          [value]="reason()"
                          (input)="reason.set($any($event.target).value)"
                        />
                        <button type="button" [disabled]="busy() !== null" (click)="reject(order)">
                          {{ 'kasse.orders.confirmReject' | transloco }}
                        </button>
                        <button type="button" (click)="stopReject()">
                          {{ 'kasse.orders.abandon' | transloco }}
                        </button>
                      </div>
                    }
                  </td>
                </tr>
              }
            </tbody>
          </table>
          @if (hasMore()) {
            <p>
              <button
                type="button"
                data-testid="load-more"
                [disabled]="loadingMore()"
                (click)="loadMore()"
              >
                {{ (loadingMore() ? 'kasse.common.loadingMore' : 'kasse.common.loadMore') | transloco }}
              </button>
            </p>
          }
        }
      </section>
    </app-page>
  `,
})
export class PaymentOrdersComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly orders = signal<KasseOrderSummary[]>([]);
  readonly status = signal('');
  /**
   * One page of orders, sent explicitly rather than left to the server's default: a screen whose
   * page size changed when the server did would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
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
    this.api.listPaymentOrders(this.status() || undefined, String(this.pageSize), '0').subscribe({
      next: (orders) => {
        const page = orders ?? [];
        this.orders.set(page);
        // A full page means there may be more; a short one is the end of the list.
        this.hasMore.set(page.length === this.pageSize);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('kasse.orders.loadFailed'));
        this.hasMore.set(false);
        this.loading.set(false);
      },
    });
  }

  /**
   * Reads the next page and appends it. The offset is the number of rows already on screen, which is
   * why the list is never re-sorted in the browser: the pages only line up while the order the server
   * answered with is the order they are shown in.
   *
   * A failed page is reported without clearing what is already there — the orders that were read are
   * still valid, and blanking the screen would suggest they are not.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api
      .listPaymentOrders(this.status() || undefined, String(this.pageSize), String(this.orders().length))
      .subscribe({
        next: (orders) => {
          const page = orders ?? [];
          this.orders.set([...this.orders(), ...page]);
          this.hasMore.set(page.length === this.pageSize);
          this.loadingMore.set(false);
        },
        error: () => {
          this.loadingMore.set(false);
          this.actionError.set(this.i18n.translate('kasse.orders.loadMoreFailed'));
        },
      });
  }

  submit(order: KasseOrderSummary): void {
    this.act(order, 'kasse.orders.step.submit', this.api.submitPaymentOrder(order.ID));
  }

  approve(order: KasseOrderSummary): void {
    this.act(order, 'kasse.orders.step.approve', this.api.approvePaymentOrder(order.ID));
  }

  cancel(order: KasseOrderSummary): void {
    this.act(order, 'kasse.orders.step.cancel', this.api.cancelPaymentOrder(order.ID));
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
      this.actionError.set(this.i18n.translate('kasse.orders.reasonRequired'));
      return;
    }
    // A sent rejection closes the reason box; a refused one keeps it, so the reason is still
    // there to correct and send again.
    this.act(order, 'kasse.orders.step.reject', this.api.rejectPaymentOrder(order.ID, { reason }), () => {
      this.rejecting.set(null);
      this.reason.set('');
    });
  }

  /**
   * Runs a workflow action and reloads, so the row shows the state the server now holds. `done`
   * runs only on success, which is how a rejection closes its reason box without any other
   * action touching one. `step` is the message key of what was attempted, so the message names the
   * step rather than a generic "action" — and in the clerk's language.
   */
  private act(order: KasseOrderSummary, step: string, request: Observable<unknown>, done?: () => void): void {
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
        this.actionError.set(
          this.i18n.translate('kasse.orders.actionFailed', {
            reference: order.Reference,
            step: this.i18n.translate(step),
          })
        );
      },
    });
  }

  /** The workflow state in the words the shared vocabulary gives it. */
  statusLabel(status: string): string {
    switch (status) {
      case 'draft':
        return this.i18n.translate('status.draft');
      case 'submitted':
        return this.i18n.translate('status.submitted');
      case 'approved':
        return this.i18n.translate('status.approved');
      case 'rejected':
        return this.i18n.translate('status.rejected');
      case 'cancelled':
        return this.i18n.translate('status.cancelled');
      default:
        return status;
    }
  }

  /** Amounts and dates are rendered by the shared formatters, not by each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;
}
