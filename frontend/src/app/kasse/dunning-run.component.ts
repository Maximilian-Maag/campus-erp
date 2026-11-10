import { Component, inject, signal } from '@angular/core';
import { CampusErpApi, DunningRunOutcome, DunningRunResult } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';

/** The German label of the reason a claim did not advance in a run. */
export function skipReasonLabel(reason: string): string {
  switch (reason) {
    case 'already_dunned':
      return 'am Stichtag bereits gemahnt';
    case 'not_due':
      return 'Frist nicht verstrichen';
    case 'unknown_level':
      return 'Stufe unbekannt';
    case 'no_next_level':
      return 'keine weitere Stufe';
    case 'next_level_missing':
      return 'nächste Stufe fehlt';
    case 'issue_failed':
      return 'Ausstellung fehlgeschlagen';
    default:
      return reason;
  }
}

/** A local calendar date as YYYY-MM-DD, the form the API reads. */
export function isoDay(d: Date): string {
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${month}-${day}`;
}

/**
 * Kasse — Mahnlauf (dunning run).
 *
 * Walks the open receivables due by a date and advances each claim one step along the year's
 * dunning chain. The screen states the year and the as-of date and reports what the run did:
 * which notices were issued, which claims were skipped and why, and which failed. The level,
 * the charge and the document are the API's answer — the screen does not re-derive them, just
 * as the hand-issued notice screen does not.
 *
 * The run never removes a row from the list: a claim that was skipped is reported with its
 * reason, so an operator sees why nothing happened to it rather than being left to guess.
 */
@Component({
  selector: 'app-dunning-run',
  template: `
    <section aria-labelledby="dunning-run-heading">
      <h2 id="dunning-run-heading">Mahnlauf</h2>

      @if (actionError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }
      @if (notice(); as n) {
        <p role="status" class="notice">{{ n }}</p>
      }

      <form (submit)="run($event)">
        <p>
          <label for="run-year">Jahr</label>
          <input
            id="run-year"
            type="number"
            [value]="year()"
            (input)="year.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="run-as-of">Stichtag</label>
          <input
            id="run-as-of"
            type="date"
            [value]="asOf()"
            (input)="asOf.set($any($event.target).value)"
          />
        </p>
        <button type="submit" [disabled]="running()">
          {{ running() ? 'Mahnlauf läuft.' : 'Mahnlauf starten' }}
        </button>
      </form>

      @if (result(); as r) {
        @if (r.Outcomes.length === 0) {
          <p role="status">Kein offener Posten war am Stichtag fällig.</p>
        } @else {
          <table>
            <caption>
              Ergebnis des Mahnlaufs
            </caption>
            <thead>
              <tr>
                <th scope="col">Posten</th>
                <th scope="col">Mandat</th>
                <th scope="col" class="amount">Betrag</th>
                <th scope="col">Fälligkeit</th>
                <th scope="col">Stufe</th>
                <th scope="col" class="amount">Mahngebühr</th>
                <th scope="col">Ergebnis</th>
              </tr>
            </thead>
            <tbody>
              @for (o of r.Outcomes; track o.ReceivableID) {
                <tr [attr.data-testid]="'run-' + o.ReceivableID">
                  <td>{{ o.ClaimReference }}</td>
                  <td>{{ o.MandateReference }}</td>
                  <td class="amount">{{ amount(o.AmountCents) }}</td>
                  <td>{{ day(o.DueDate) }}</td>
                  <td>{{ o.LevelCode }}</td>
                  <td class="amount">{{ o.Issued ? amount(o.ChargeCents) : '—' }}</td>
                  <td>
                    {{ outcomeLabel(o) }}
                    @if (o.Issued && o.DocumentKey) {
                      <a [href]="'/api/dunning/notices/' + o.NoticeID + '/document'">Mahnung</a>
                    }
                    @if (o.Detail) {
                      <span class="detail">{{ o.Detail }}</span>
                    }
                  </td>
                </tr>
              }
            </tbody>
          </table>
        }
      }
    </section>
  `,
})
export class DunningRunComponent {
  private readonly api = inject(CampusErpApi);

  readonly year = signal(String(new Date().getFullYear()));
  readonly asOf = signal(isoDay(new Date()));
  readonly running = signal(false);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  readonly result = signal<DunningRunResult | null>(null);

  /** Runs the dunning workflow over the open receivables due by the stated date. */
  run(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    this.result.set(null);

    const year = Number(this.year());
    if (!Number.isInteger(year) || year < 1900 || year > 2999) {
      this.actionError.set('Das Jahr muss zwischen 1900 und 2999 liegen.');
      return;
    }
    if (!/^\d{4}-\d{2}-\d{2}$/.test(this.asOf())) {
      this.actionError.set('Ein Stichtag ist erforderlich.');
      return;
    }

    this.running.set(true);
    this.api.runDunning({ year, as_of: this.asOf() }).subscribe({
      next: (result) => {
        this.result.set(result);
        this.notice.set(
          `Mahnlauf abgeschlossen: ${result.Issued} ausgestellt, ${result.Skipped} übersprungen, ` +
            `${result.Failed} fehlgeschlagen.`
        );
        this.running.set(false);
      },
      error: () => {
        this.actionError.set('Der Mahnlauf konnte nicht ausgeführt werden.');
        this.running.set(false);
      },
    });
  }

  amount(cents: number): string {
    return formatCents(cents);
  }

  day(iso: string): string {
    return formatDay(iso);
  }

  reason(reason: string): string {
    return skipReasonLabel(reason);
  }

  outcomeLabel(o: DunningRunOutcome): string {
    return o.Issued ? 'ausgestellt' : this.reason(o.Reason);
  }
}
