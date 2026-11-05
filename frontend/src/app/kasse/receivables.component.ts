import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, Receivable } from '../core/api/generated';
import { euroToCents, formatCents, formatDay } from '../core/format';

/** The state as the screen shows it. */
export function stateLabel(state: string): string {
  switch (state) {
    case 'open':
      return 'offen';
    case 'collected':
      return 'eingezogen';
    case 'cancelled':
      return 'storniert';
    default:
      return state;
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
  template: `
    <section aria-labelledby="receivables-heading">
      <h2 id="receivables-heading">Offene Posten</h2>

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
        <label for="receivable-state">Status</label>
        <select id="receivable-state" [value]="stateFilter()" (change)="choose($any($event.target).value)">
          <option value="open">offen</option>
          <option value="collected">eingezogen</option>
          <option value="cancelled">storniert</option>
          <option value="">alle</option>
        </select>
      </div>

      @if (loading()) {
        <p role="status">Offene Posten werden geladen.</p>
      } @else if (items().length === 0 && !loadError()) {
        <p role="status">Keine Posten in diesem Status.</p>
      } @else if (!loadError()) {
        <table>
          <caption>
            Offene Posten mit Mandat, Betrag und Fälligkeit
          </caption>
          <thead>
            <tr>
              <th scope="col">Mandat</th>
              <th scope="col" class="amount">Betrag</th>
              <th scope="col">Fälligkeit</th>
              <th scope="col">Zweck</th>
              <th scope="col">Status</th>
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
      }

      <form (submit)="record($event)">
        <h3 id="receivable-new-heading">Posten erfassen</h3>
        <p>
          <label for="receivable-mandate">Mandat</label>
          <input
            id="receivable-mandate"
            type="text"
            [value]="mandate()"
            (input)="mandate.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="receivable-amount">Betrag in Euro</label>
          <input
            id="receivable-amount"
            type="text"
            inputmode="decimal"
            [value]="amount()"
            (input)="amount.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="receivable-currency">Währung</label>
          <input
            id="receivable-currency"
            type="text"
            maxlength="3"
            [value]="currency()"
            (input)="currency.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="receivable-due">Fälligkeit</label>
          <input
            id="receivable-due"
            type="date"
            [value]="dueDate()"
            (input)="dueDate.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="receivable-purpose">Zweck</label>
          <input
            id="receivable-purpose"
            type="text"
            [value]="purpose()"
            (input)="purpose.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="receivable-source">Quelle</label>
          <input
            id="receivable-source"
            type="text"
            [value]="source()"
            (input)="source.set($any($event.target).value)"
          />
        </p>
        <button type="submit" [disabled]="saving()">Erfassen</button>
      </form>
    </section>
  `,
})
export class ReceivablesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly items = signal<Receivable[]>([]);
  readonly stateFilter = signal('open');
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
    this.api.listReceivables(this.stateFilter() || undefined).subscribe({
      next: (items) => {
        this.items.set(items ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Konnte die offenen Posten nicht laden.');
        this.loading.set(false);
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
      this.actionError.set('Der Betrag muss eine positive Zahl in Euro sein.');
      return;
    }
    if (this.dueDate().trim() === '') {
      this.actionError.set('Eine Fälligkeit ist erforderlich.');
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
          this.notice.set('Posten erfasst.');
          // A new item is open, so it belongs in the open list whatever was being shown.
          if (this.stateFilter() === 'open') {
            this.load();
          } else {
            this.choose('open');
          }
        },
        error: () => {
          this.saving.set(false);
          this.actionError.set('Konnte den Posten nicht erfassen.');
        },
      });
  }

  /** Amounts, dates and states are rendered by the shared helpers, not each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;
  readonly label = stateLabel;
}
