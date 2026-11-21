import { Component, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, DunningRunOutcome, DunningRunResult } from '../core/api/generated';
import { formatCents, formatDay, isoDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

/** The message key of each reason a claim does not advance in a run. */
const SKIP_REASON_KEYS: Record<string, string> = {
  already_dunned: 'kasse.dunningRun.reason.alreadyDunned',
  not_due: 'kasse.dunningRun.reason.notDue',
  unknown_level: 'kasse.dunningRun.reason.unknownLevel',
  no_next_level: 'kasse.dunningRun.reason.noNextLevel',
  next_level_missing: 'kasse.dunningRun.reason.nextLevelMissing',
  issue_failed: 'kasse.dunningRun.reason.issueFailed',
};

/** The message key of the reason a claim did not advance, or null for one this build does not know. */
export function skipReasonKey(reason: string): string | null {
  return SKIP_REASON_KEYS[reason] ?? null;
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
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.kasseMahnlauf.title" subtitleKey="page.kasseMahnlauf.subtitle">
      <section>
        @if (actionError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (notice(); as n) {
          <p role="status" class="notice">{{ n }}</p>
        }

        <form (submit)="run($event)">
          <div class="field">
            <label for="run-year">{{ 'common.year' | transloco }}</label>
            <input
              id="run-year"
              type="number"
              [value]="year()"
              (input)="year.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <label for="run-as-of">{{ 'kasse.dunningRun.asOf' | transloco }}</label>
            <input
              id="run-as-of"
              type="date"
              [value]="asOf()"
              (input)="asOf.set($any($event.target).value)"
            />
          </div>
          <button type="submit" [disabled]="running()">
            {{ (running() ? 'kasse.dunningRun.running' : 'kasse.dunningRun.start') | transloco }}
          </button>
        </form>

        @if (result(); as r) {
          @if (r.Outcomes.length === 0) {
            <p role="status">{{ 'kasse.dunningRun.nothingDue' | transloco }}</p>
          } @else {
            <table>
              <caption>
                {{ 'kasse.dunningRun.caption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'kasse.dunningRun.item' | transloco }}</th>
                  <th scope="col">{{ 'common.mandate' | transloco }}</th>
                  <th scope="col" class="amount">{{ 'common.amount' | transloco }}</th>
                  <th scope="col">{{ 'common.dueDate' | transloco }}</th>
                  <th scope="col">{{ 'kasse.dunningRun.level' | transloco }}</th>
                  <th scope="col" class="amount">{{ 'kasse.dunningRun.charge' | transloco }}</th>
                  <th scope="col">{{ 'kasse.dunningRun.outcome' | transloco }}</th>
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
                        <a [href]="'/api/dunning/notices/' + o.NoticeID + '/document'">
                          {{ 'kasse.dunningRun.notice' | transloco }}
                        </a>
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
    </app-page>
  `,
})
export class DunningRunComponent {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

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
      this.actionError.set(this.i18n.translate('kasse.dunningRun.yearInvalid'));
      return;
    }
    if (!/^\d{4}-\d{2}-\d{2}$/.test(this.asOf())) {
      this.actionError.set(this.i18n.translate('kasse.dunningRun.asOfRequired'));
      return;
    }

    this.running.set(true);
    this.api.runDunning({ year, as_of: this.asOf() }).subscribe({
      next: (result) => {
        this.result.set(result);
        this.notice.set(
          this.i18n.translate('kasse.dunningRun.completed', {
            issued: result.Issued,
            skipped: result.Skipped,
            failed: result.Failed,
          })
        );
        this.running.set(false);
      },
      error: () => {
        this.actionError.set(this.i18n.translate('kasse.dunningRun.runFailed'));
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
    const key = skipReasonKey(reason);
    return key === null ? reason : this.i18n.translate(key);
  }

  outcomeLabel(o: DunningRunOutcome): string {
    return o.Issued ? this.i18n.translate('kasse.dunningRun.issued') : this.reason(o.Reason);
  }
}
