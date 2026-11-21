import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, Receivable } from '../core/api/generated';
import { euroToCents, formatCents, formatDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

/** The message key of a state as the screen shows it, or null for one the screen has no word for. */
export function stateKey(state: string): string | null {
  switch (state) {
    case 'open':
      return 'kasse.receivables.state.open';
    case 'collected':
      return 'kasse.receivables.state.collected';
    case 'cancelled':
      return 'kasse.receivables.state.cancelled';
    default:
      return null;
  }
}

/**
 * Kasse — Offene Posten (open receivables).
 *
 * Lists the open items a direct-debit collection run selects from, and records one. The state
 * filter is the server's (`?state=`), so the screen shows one state rather than holding every
 * item and sifting. Whether an item may be collected — its mandate is active, the currency
 * matches — is the API's decision at collection time; the screen does not re-derive it.
 */
@Component({
  selector: 'app-receivables',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.kasseOffenePosten.title" subtitleKey="page.kasseOffenePosten.subtitle">
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

        <div class="picker">
          <label for="receivable-state">{{ 'common.status' | transloco }}</label>
          <select id="receivable-state" [value]="stateFilter()" (change)="choose($any($event.target).value)">
            <option value="open">{{ 'kasse.receivables.state.open' | transloco }}</option>
            <option value="collected">{{ 'kasse.receivables.state.collected' | transloco }}</option>
            <option value="cancelled">{{ 'kasse.receivables.state.cancelled' | transloco }}</option>
            <option value="">{{ 'common.all' | transloco }}</option>
          </select>
        </div>

        @if (loading()) {
          <p role="status">{{ 'kasse.receivables.loading' | transloco }}</p>
        } @else if (items().length === 0 && !loadError()) {
          <p role="status">{{ 'kasse.receivables.empty' | transloco }}</p>
        } @else if (!loadError()) {
          <table>
            <caption>
              {{ 'kasse.receivables.caption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.mandate' | transloco }}</th>
                <th scope="col" class="amount">{{ 'common.amount' | transloco }}</th>
                <th scope="col">{{ 'common.dueDate' | transloco }}</th>
                <th scope="col">{{ 'common.purpose' | transloco }}</th>
                <th scope="col">{{ 'common.status' | transloco }}</th>
              </tr>
            </thead>
            <tbody>
              @for (item of items(); track item.ID) {
                <tr [attr.data-testid]="'receivable-' + item.ID">
                  <td>{{ item.MandateReference }}</td>
                  <td class="amount">{{ euro(item.AmountCents) }} {{ item.Currency }}</td>
                  <td>{{ day(item.DueDate) }}</td>
                  <td>{{ item.Purpose }}</td>
                  <td>{{ label(item.State) }}</td>
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
                {{
                  (loadingMore()
                    ? 'kasse.receivables.loadingMore'
                    : 'kasse.receivables.loadMore'
                  ) | transloco
                }}
              </button>
            </p>
          }
        }

        <form (submit)="record($event)">
          <h2 id="receivable-new-heading">{{ 'kasse.receivables.recordHeading' | transloco }}</h2>
          <div class="field">
            <label for="receivable-mandate">{{ 'common.mandate' | transloco }}</label>
            <input
              id="receivable-mandate"
              type="text"
              [value]="mandate()"
              (input)="mandate.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <label for="receivable-amount">{{ 'kasse.receivables.amountEuro' | transloco }}</label>
            <input
              id="receivable-amount"
              type="text"
              inputmode="decimal"
              [value]="amount()"
              (input)="amount.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <label for="receivable-currency">{{ 'common.currency' | transloco }}</label>
            <input
              id="receivable-currency"
              type="text"
              maxlength="3"
              [value]="currency()"
              (input)="currency.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <label for="receivable-due">{{ 'common.dueDate' | transloco }}</label>
            <input
              id="receivable-due"
              type="date"
              [value]="dueDate()"
              (input)="dueDate.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <label for="receivable-purpose">{{ 'common.purpose' | transloco }}</label>
            <input
              id="receivable-purpose"
              type="text"
              [value]="purpose()"
              (input)="purpose.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <label for="receivable-source">{{ 'kasse.receivables.source' | transloco }}</label>
            <input
              id="receivable-source"
              type="text"
              [value]="source()"
              (input)="source.set($any($event.target).value)"
            />
          </div>
          <button type="submit" [disabled]="saving()">{{ 'kasse.receivables.record' | transloco }}</button>
        </form>
      </section>
    </app-page>
  `,
})
export class ReceivablesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly items = signal<Receivable[]>([]);
  readonly stateFilter = signal('open');
  /**
   * One page of items, sent explicitly rather than left to the server's default: a screen whose page
   * size changed when the server did would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
  readonly loading = signal(true);
  readonly saving = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  readonly mandate = signal('');
  readonly amount = signal('');
  readonly currency = signal('EUR');
  readonly dueDate = signal('');
  readonly purpose = signal('');
  readonly source = signal('');

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listReceivables(this.stateFilter() || undefined, String(this.pageSize), '0').subscribe({
      next: (items) => {
        const page = items ?? [];
        this.items.set(page);
        // A full page means there may be more; a short one is the end of the list.
        this.hasMore.set(page.length === this.pageSize);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('kasse.receivables.loadFailed'));
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
   * A failed page is reported without clearing what is already there — the items that were read are
   * still valid, and blanking the screen would suggest they are not.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api
      .listReceivables(
        this.stateFilter() || undefined,
        String(this.pageSize),
        String(this.items().length),
      )
      .subscribe({
        next: (items) => {
          const page = items ?? [];
          this.items.set([...this.items(), ...page]);
          this.hasMore.set(page.length === this.pageSize);
          this.loadingMore.set(false);
        },
        error: () => {
          this.loadingMore.set(false);
          this.actionError.set(this.i18n.translate('kasse.receivables.loadMoreFailed'));
        },
      });
  }

  /** Switches the shown state and reloads. The filter is the server's, so this asks again. */
  choose(state: string): void {
    this.stateFilter.set(state);
    this.notice.set(null);
    this.actionError.set(null);
    this.load();
  }

  record(event: Event): void {
    event.preventDefault();
    const cents = euroToCents(this.amount());
    if (cents === null) {
      this.actionError.set(this.i18n.translate('kasse.receivables.amountRequired'));
      return;
    }
    if (this.dueDate().trim() === '') {
      this.actionError.set(this.i18n.translate('kasse.receivables.dueDateRequired'));
      return;
    }
    this.saving.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api
      .createReceivable({
        mandate_reference: this.mandate().trim(),
        amount_cents: cents,
        currency: this.currency().trim().toUpperCase(),
        due_date: this.dueDate().trim(),
        purpose: this.purpose().trim(),
        source: this.source().trim(),
      })
      .subscribe({
        next: () => {
          this.saving.set(false);
          this.amount.set('');
          this.purpose.set('');
          this.notice.set(this.i18n.translate('kasse.receivables.recorded'));
          // A new item is open, so it belongs in the open list whatever was being shown.
          if (this.stateFilter() === 'open') {
            this.load();
          } else {
            this.choose('open');
          }
        },
        error: () => {
          this.saving.set(false);
          this.actionError.set(this.i18n.translate('kasse.receivables.recordFailed'));
        },
      });
  }

  /** Amounts, dates and states are rendered by the shared helpers, not each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;

  /** The word for a state, from the message key the screen names it by. */
  label(state: string): string {
    const key = stateKey(state);
    return key === null ? state : this.i18n.translate(key);
  }
}
