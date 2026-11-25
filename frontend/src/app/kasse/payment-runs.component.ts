import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, CashAccount, KasseOrderSummary, PaymentRun } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';
import { problemCode } from '../core/problem';
import { PageComponent } from '../ui/page.component';

/**
 * Kasse — Zahlungsläufe (SEPA credit-transfer payment runs).
 *
 * The credit-transfer counterpart of the collection run: a run gathers approved payment orders
 * into one pain.001 file for one execution date, and stores the file so the run is reproducible.
 * The screen offers one of the university's own accounts, an execution date and the approved
 * orders to include, creates the run, and then shows the file it produced — which can also be
 * handed to the bank channel.
 *
 * Which orders a run may gather — approved ones, each paid at most once — is the API's decision:
 * the screen offers the orders the server reports as approved and reports a refusal (an
 * unapproved order, an already-paid one, a duplicate reference) as it comes back rather than
 * re-deriving the rule. Labels and messages are message keys, so the screen speaks the clerk's
 * language.
 */
@Component({
  selector: 'app-payment-runs',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.kasseZahlungslaeufe.title" subtitleKey="page.kasseZahlungslaeufe.subtitle">
      <section>
        @if (loadError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (actionError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (notice(); as n) {
          <p role="status" class="notice">{{ n }}</p>
        }

        @if (loading()) {
          <p role="status">{{ 'kasse.runs.loading' | transloco }}</p>
        } @else if (accounts().length === 0 && !loadError()) {
          <p role="status">{{ 'kasse.accounts.none' | transloco }}</p>
        } @else if (!loadError()) {
          @if (orders().length === 0) {
            <p role="status">{{ 'kasse.runs.noApprovedOrder' | transloco }}</p>
          } @else {
            <form (submit)="create($event)">
              <h2 id="payment-run-new-heading">{{ 'kasse.runs.create' | transloco }}</h2>
              <div class="field">
                <label for="payment-run-reference">{{ 'common.reference' | transloco }}</label>
                <input
                  id="payment-run-reference"
                  type="text"
                  [value]="reference()"
                  (input)="reference.set($any($event.target).value)"
                />
              </div>
              <div class="field">
                <label for="payment-run-account">{{ 'common.account' | transloco }}</label>
                <select
                  id="payment-run-account"
                  [value]="account()"
                  (change)="account.set($any($event.target).value)"
                >
                  @for (a of accounts(); track a.ID) {
                    <option [value]="a.ID" [selected]="'' + a.ID === account()">{{ a.Name }} ({{ a.IBAN }})</option>
                  }
                </select>
              </div>
              <div class="field">
                <label for="payment-run-execution-date">{{ 'kasse.runs.executionDate' | transloco }}</label>
                <input
                  id="payment-run-execution-date"
                  type="date"
                  [value]="executionDate()"
                  (input)="executionDate.set($any($event.target).value)"
                />
              </div>

              <h3 id="payment-run-orders-heading">{{ 'kasse.runs.approvedOrders' | transloco }}</h3>
              <table>
                <caption>
                  {{ 'kasse.runs.caption' | transloco }}
                </caption>
                <thead>
                  <tr>
                    <th scope="col">{{ 'kasse.runs.selection' | transloco }}</th>
                    <th scope="col">{{ 'common.reference' | transloco }}</th>
                    <th scope="col">{{ 'common.purpose' | transloco }}</th>
                    <th scope="col" class="amount">{{ 'common.amount' | transloco }}</th>
                    <th scope="col">{{ 'common.due' | transloco }}</th>
                  </tr>
                </thead>
                <tbody>
                  @for (order of orders(); track order.ID) {
                    <tr [attr.data-testid]="'order-' + order.Reference">
                      <td>
                        <input
                          type="checkbox"
                          [checked]="isSelected(order.ID)"
                          (change)="toggle(order.ID)"
                          [attr.aria-label]="'kasse.runs.selectLabel' | transloco: { reference: order.Reference }"
                        />
                      </td>
                      <td>{{ order.Reference }}</td>
                      <td>{{ order.Purpose }}</td>
                      <td class="amount">{{ euro(order.AmountCents) }}</td>
                      <td>{{ day(order.DueDate) }}</td>
                    </tr>
                  }
                </tbody>
              </table>
              @if (hasMore()) {
                <p>
                  <button
                    type="button"
                    data-testid="load-more-orders"
                    [disabled]="loadingMore()"
                    (click)="loadMoreOrders()"
                  >
                    {{ (loadingMore() ? 'kasse.common.loadingMore' : 'kasse.common.loadMore') | transloco }}
                  </button>
                </p>
              }
              <p role="status" [attr.data-testid]="'selected-sum'">
                {{
                  'kasse.runs.selectedSummary'
                    | transloco: { count: selected().length, sum: euro(selectedSum()) }
                }}
              </p>
              <div class="field">
                <button type="submit" [disabled]="submitting()">
                  {{ 'kasse.runs.create' | transloco }}
                </button>
              </div>
            </form>
          }

          @if (created(); as r) {
            <section aria-labelledby="payment-run-result-heading">
              <h2 id="payment-run-result-heading">{{ 'kasse.runs.createdHeading' | transloco }}</h2>
              <p role="status" [attr.data-testid]="'run-' + r.Reference">
                {{
                  'kasse.runs.createdSummary'
                    | transloco: {
                        reference: r.Reference,
                        count: r.ItemCount,
                        sum: euro(r.SumCents),
                        date: day(r.ExecutionDate)
                      }
                }}
              </p>
              <button type="button" (click)="fetchFile()">{{ 'kasse.runs.showFile' | transloco }}</button>
              <button type="button" [disabled]="exporting()" (click)="queueExport()">
                {{ 'kasse.runs.handOver' | transloco }}
              </button>
              @if (file(); as f) {
                <pre [attr.data-testid]="'run-file-' + r.Reference">{{ f }}</pre>
              }
            </section>
          }
        }
      </section>
    </app-page>
  `,
})
export class PaymentRunsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly accounts = signal<CashAccount[]>([]);
  readonly account = signal('');
  readonly orders = signal<KasseOrderSummary[]>([]);
  /**
   * One page of approved orders, sent explicitly rather than left to the server's default: a
   * picker that reads the first 50 and offers no way to reach the rest silently hides orders the
   * operator came here to put in a run (#179).
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
  readonly loading = signal(true);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  readonly reference = signal('');
  readonly executionDate = signal('');
  /** The ids of the approved orders this run is to gather. */
  readonly selected = signal<number[]>([]);
  readonly submitting = signal(false);
  readonly exporting = signal(false);
  readonly created = signal<PaymentRun | null>(null);
  readonly file = signal<string | null>(null);

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listCashAccounts().subscribe({
      next: (accounts) => {
        const list = accounts ?? [];
        this.accounts.set(list);
        const id = this.account() || String(list[0]?.ID ?? '');
        this.account.set(id);
        if (id) {
          this.loadOrders();
        } else {
          this.loading.set(false);
        }
      },
      error: () => {
        this.loadError.set(this.i18n.translate('kasse.accounts.loadFailed'));
        this.loading.set(false);
      },
    });
  }

  /** Loads the orders a run may gather — only the approved ones, the server's filter. */
  loadOrders(): void {
    this.api.listPaymentOrders('approved', String(this.pageSize), '0', 'true').subscribe({
      next: (orders) => {
        const page = orders ?? [];
        this.orders.set(page);
        // A full page means there may be more; a short one is the end of the list.
        this.hasMore.set(page.length === this.pageSize);
        // An order that is no longer offered cannot be gathered, so a stale selection is dropped.
        this.selected.set(
          this.selected().filter((id) => this.orders().some((o) => o.ID === id))
        );
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
   * Reads the next page of approved orders and appends it. The offset is the number of rows the
   * server has handed over so far, which is why the list is never re-sorted in the browser.
   */
  loadMoreOrders(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api
      .listPaymentOrders('approved', String(this.pageSize), String(this.orders().length), 'true')
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

  isSelected(id: number): boolean {
    return this.selected().includes(id);
  }

  toggle(id: number): void {
    this.selected.set(
      this.isSelected(id) ? this.selected().filter((s) => s !== id) : [...this.selected(), id]
    );
  }

  /** The sum of the selected orders, so the operator sees what the run will move. */
  selectedSum(): number {
    return this.orders()
      .filter((o) => this.isSelected(o.ID))
      .reduce((sum, o) => sum + o.AmountCents, 0);
  }

  create(event: Event): void {
    event.preventDefault();
    const reference = this.reference().trim();
    if (reference === '') {
      this.actionError.set(this.i18n.translate('kasse.runs.referenceRequired'));
      return;
    }
    if (this.executionDate().trim() === '') {
      this.actionError.set(this.i18n.translate('kasse.runs.executionDateRequired'));
      return;
    }
    if (this.selected().length === 0) {
      this.actionError.set(this.i18n.translate('kasse.runs.selectionRequired'));
      return;
    }
    this.submitting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.created.set(null);
    this.file.set(null);
    this.api
      .createPaymentRun({
        reference,
        account_id: Number(this.account()),
        execution_date: this.executionDate().trim(),
        order_ids: this.selected(),
      })
      .subscribe({
        next: (run) => {
          this.submitting.set(false);
          this.created.set(run);
          this.notice.set(this.i18n.translate('kasse.runs.created'));
          // The orders just gathered are no longer available, so the picker is re-read: leaving
          // them ticked offers a second run that the server refuses (#180).
          this.selected.set([]);
          this.loadOrders();
        },
        error: (err) => {
          this.submitting.set(false);
          // An order already in a run is the one refusal the operator can act on — it names the
          // reason instead of the generic "could not create".
          this.actionError.set(
            problemCode(err) === 'kasse.order_already_paid'
              ? this.i18n.translate('kasse.runs.orderAlreadyPaid')
              : this.i18n.translate('kasse.runs.createFailed')
          );
        },
      });
  }

  /** Reads the file the run produced (the stored pain.001). */
  fetchFile(): void {
    const run = this.created();
    if (!run) {
      return;
    }
    this.api.getPaymentRunFile(run.ID).subscribe({
      next: (file) => this.file.set(file),
      error: () => this.actionError.set(this.i18n.translate('kasse.runs.fileFailed')),
    });
  }

  /** Hands the run's file to the bank channel; the API queues the work and answers with the job. */
  queueExport(): void {
    const run = this.created();
    if (!run) {
      return;
    }
    this.exporting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.requestSepaExport(run.ID).subscribe({
      next: (queued) => {
        this.exporting.set(false);
        this.notice.set(
          this.i18n.translate('kasse.runs.exportQueued', { job: queued.job_id })
        );
      },
      error: () => {
        this.exporting.set(false);
        this.actionError.set(this.i18n.translate('kasse.runs.exportFailed'));
      },
    });
  }

  /** Amounts and dates are rendered by the shared formatters, not by each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;
}
