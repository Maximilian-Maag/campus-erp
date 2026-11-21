import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, ProcurementOrderSummary } from '../core/api/generated';
import { formatCents } from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * Procurement — purchase orders (Bestellungen).
 *
 * Lists the orders and approves one that is still a draft, which is the step that commits
 * budget. The status filter is the server's own: choosing one asks the API for that status
 * rather than sifting what happens to be loaded, so the screen works on a list it never holds
 * in full.
 *
 * The heading and the labels are message keys, so the screen names itself in whatever language
 * the clerk picked; the API's own status values are shown as the API reports them.
 */
@Component({
  selector: 'app-purchase-orders',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.beschaffungBestellungen.title" subtitleKey="page.beschaffungBestellungen.subtitle">
      <section>
        <div class="filter">
          <label for="order-status">{{ 'common.status' | transloco }}</label>
          <select id="order-status" [value]="status()" (change)="filterTo($any($event.target).value)">
            <option value="">{{ 'common.all' | transloco }}</option>
            <option value="draft">{{ 'status.draft' | transloco }}</option>
            <option value="approved">{{ 'status.approved' | transloco }}</option>
            <option value="received">{{ 'beschaffung.orders.statusReceived' | transloco }}</option>
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
          <p role="status">{{ 'beschaffung.orders.loading' | transloco }}</p>
        } @else if (orders().length === 0 && !loadError()) {
          <p role="status">
            {{ (status() ? 'beschaffung.orders.emptyFiltered' : 'beschaffung.orders.empty') | transloco }}
          </p>
        } @else if (!loadError()) {
          <table>
            <caption>
              {{ 'beschaffung.orders.caption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.number' | transloco }}</th>
                <th scope="col">{{ 'beschaffung.orders.supplier' | transloco }}</th>
                <th scope="col" class="amount">{{ 'common.amount' | transloco }}</th>
                <th scope="col" class="amount">{{ 'beschaffung.orders.billed' | transloco }}</th>
                <th scope="col">{{ 'common.status' | transloco }}</th>
                <th scope="col">{{ 'common.action' | transloco }}</th>
              </tr>
            </thead>
            <tbody>
              @for (order of orders(); track order.ID) {
                <tr [attr.data-testid]="'order-' + order.Number">
                  <td>{{ order.Number }}</td>
                  <td>{{ order.Supplier }}</td>
                  <td class="amount">{{ euro(order.GrossCents) }}</td>
                  <td class="amount" [attr.data-testid]="'billed-' + order.Number">{{ billedLabel(order) }}</td>
                  <td [attr.data-testid]="'status-' + order.Number">{{ statusLabel(order.Status) }}</td>
                  <td>
                    @if (order.Status === 'draft') {
                      <button
                        type="button"
                        [disabled]="approving() !== null"
                        [attr.aria-label]="'beschaffung.orders.approveLabel' | transloco: { number: order.Number }"
                        (click)="approve(order)"
                      >
                        {{ 'beschaffung.orders.approve' | transloco }}
                      </button>
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
                {{ (loadingMore() ? 'beschaffung.orders.loadingMore' : 'beschaffung.orders.loadMore') | transloco }}
              </button>
            </p>
          }
        }
      </section>
    </app-page>
  `,
})
export class PurchaseOrdersComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly orders = signal<ProcurementOrderSummary[]>([]);
  /**
   * One page of orders, sent explicitly rather than left to the server's default: a screen whose page
   * size changed when the server did would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
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
    this.hasMore.set(false);
    this.api
      .listPurchaseOrders(this.status() || undefined, String(this.pageSize), '0')
      .subscribe({
        next: (orders) => {
          const page = orders ?? [];
          this.orders.set(page);
          // A full page means there may be more; a short one is the end of the list.
          this.hasMore.set(page.length === this.pageSize);
          this.loading.set(false);
        },
        error: () => {
          this.loadError.set(this.i18n.translate('beschaffung.orders.loadFailed'));
          this.loading.set(false);
        },
      });
  }

  /**
   * Reads the next page of the chosen status and appends it. The offset is the number of orders
   * already on screen, which is why the list is never re-sorted in the browser: the pages only line
   * up while the order the server answered with is the order they are shown in.
   *
   * A failed page is reported without clearing what is there — the orders already read are still
   * valid, and blanking the screen would suggest they are not.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api
      .listPurchaseOrders(
        this.status() || undefined,
        String(this.pageSize),
        String(this.orders().length),
      )
      .subscribe({
        next: (orders) => {
          const page = orders ?? [];
          this.orders.set([...this.orders(), ...page]);
          this.hasMore.set(page.length === this.pageSize);
          this.loadingMore.set(false);
        },
        error: () => {
          this.loadingMore.set(false);
          this.actionError.set(this.i18n.translate('beschaffung.orders.loadMoreFailed'));
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
        this.actionError.set(
          this.i18n.translate('beschaffung.orders.approveFailed', { number: order.Number }),
        );
      },
    });
  }

  /** The lifecycle state in the words the screen uses. */
  statusLabel(status: string): string {
    switch (status) {
      case 'draft':
        return this.i18n.translate('status.draft');
      case 'approved':
        return this.i18n.translate('status.approved');
      case 'received':
        return this.i18n.translate('beschaffung.orders.statusReceived');
      case 'cancelled':
        return this.i18n.translate('status.cancelled');
      default:
        return status;
    }
  }

  /** Amounts are rendered by the shared formatter, not by each screen's own. */
  readonly euro = formatCents;

  /**
   * What has been invoiced of an order, in words: nothing yet, a part of it, or all of it. An order
   * may be billed in parts (docs/12), so "approved" alone no longer says whether money is still
   * expected — this column does.
   */
  billedLabel(order: ProcurementOrderSummary): string {
    if (order.InvoicedCents === 0) {
      return '—';
    }
    if (order.InvoicedCents >= order.GrossCents) {
      return this.i18n.translate('beschaffung.orders.billedFull', {
        amount: formatCents(order.InvoicedCents),
      });
    }
    return this.i18n.translate('beschaffung.orders.billedPartial', {
      amount: formatCents(order.InvoicedCents),
      total: formatCents(order.GrossCents),
    });
  }
}
