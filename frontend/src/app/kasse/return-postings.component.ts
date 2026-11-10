import { Component, inject, signal } from '@angular/core';
import { CampusErpApi, StatementLine } from '../core/api/generated';
import { formatCents, formatDay, isoDay } from '../core/format';

/**
 * Kasse — Rückläufer (return postings to the ledger side).
 *
 * Lists the bank-statement lines the bank took back in a period — a refused direct debit, a
 * payment-out that came back — and hands them to the financial ledger as the counterpart postings
 * that reverse the original movement (`docs/30`).
 *
 * The period is the server's: the screen sends the two days as typed and the server answers with
 * the returning lines of that period, so an empty table means "none came back" and a refused
 * period (422) is a failed load rather than an empty list. The read and the queued export cover
 * the same set, because both ask the server for the same period.
 */
@Component({
  selector: 'app-return-postings',
  template: `
    <section aria-labelledby="returns-heading">
      <h2 id="returns-heading">Rückläufer</h2>

      @if (loadError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }
      @if (actionError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }
      @if (notice(); as n) {
        <p role="status" class="notice">{{ n }}</p>
      }

      <form (submit)="load($event)">
        <p>
          <label for="returns-from">Von</label>
          <input
            id="returns-from"
            type="date"
            [value]="from()"
            (input)="from.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="returns-to">Bis</label>
          <input
            id="returns-to"
            type="date"
            [value]="to()"
            (input)="to.set($any($event.target).value)"
          />
        </p>
        <button type="submit" [disabled]="loading()">
          {{ loading() ? 'Rückläufer werden geladen.' : 'Rückläufer laden' }}
        </button>
      </form>

      @if (loaded()) {
        @if (lines().length === 0) {
          <p role="status">Für den Zeitraum sind keine Rückläufer eingelesen.</p>
        } @else {
          <table>
            <caption>
              Rückläufer im Zeitraum {{ day(from()) }} bis {{ day(to()) }}
            </caption>
            <thead>
              <tr>
                <th scope="col">Wertstellung</th>
                <th scope="col">Buchungsschlüssel</th>
                <th scope="col">Auftrag</th>
                <th scope="col" class="amount">Betrag</th>
                <th scope="col">Richtung</th>
                <th scope="col">Zweck</th>
              </tr>
            </thead>
            <tbody>
              @for (l of lines(); track l.ID) {
                <tr [attr.data-testid]="'return-' + l.ID">
                  <td>{{ day(l.ValueDate) }}</td>
                  <td>{{ l.PostingKey }}</td>
                  <td>{{ l.ClientRef }}</td>
                  <td class="amount">{{ amount(l.AmountCents) }}</td>
                  <td>{{ direction(l) }}</td>
                  <td>{{ l.Purpose }}</td>
                </tr>
              }
            </tbody>
          </table>

          <form (submit)="queue($event)">
            <button type="submit" [disabled]="queueing()">
              {{ queueing() ? 'Übergabe wird eingeplant.' : 'An FiBu übergeben' }}
            </button>
          </form>
        }
      }
    </section>
  `,
})
export class ReturnPostingsComponent {
  private readonly api = inject(CampusErpApi);

  readonly from = signal(isoDay(new Date(new Date().getFullYear(), new Date().getMonth(), 1)));
  readonly to = signal(isoDay(new Date()));
  readonly loading = signal(false);
  readonly queueing = signal(false);
  readonly loaded = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  readonly lines = signal<StatementLine[]>([]);

  /** Loads the returning lines of the stated period. */
  load(event: Event): void {
    event.preventDefault();
    this.loadError.set(null);
    this.actionError.set(null);
    this.notice.set(null);

    const from = this.from();
    const to = this.to();
    if (!isDay(from) || !isDay(to)) {
      this.loadError.set('Ein Zeitraum aus zwei Tagen ist erforderlich.');
      this.loaded.set(false);
      return;
    }

    this.loading.set(true);
    this.api.listReturnedLines(from, to).subscribe({
      next: (lines) => {
        this.lines.set(lines);
        this.loaded.set(true);
        this.loading.set(false);
      },
      error: () => {
        // A refused period must not look like a period nothing came back in.
        this.loadError.set('Die Rückläufer konnten nicht geladen werden.');
        this.loaded.set(false);
        this.loading.set(false);
      },
    });
  }

  /** Queues the return-postings file for the loaded period. */
  queue(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);

    this.queueing.set(true);
    this.api.requestReturnPostings({ from: this.from(), to: this.to() }).subscribe({
      next: (queued) => {
        this.notice.set(`Übergabe an die FiBu eingeplant (Auftrag ${queued.job_id}).`);
        this.queueing.set(false);
      },
      error: () => {
        this.actionError.set('Die Übergabe konnte nicht eingeplant werden.');
        this.queueing.set(false);
      },
    });
  }

  amount(cents: number): string {
    return formatCents(cents);
  }

  day(iso: string): string {
    return formatDay(iso);
  }

  /** Money the bank returned arrives in the account: a credit line is an incoming return. */
  direction(l: StatementLine): string {
    return l.Credit ? 'Eingang' : 'Ausgang';
  }
}

/** Whether a value is a calendar day in the form the API reads. */
export function isDay(value: string): boolean {
  return /^\d{4}-\d{2}-\d{2}$/.test(value);
}
