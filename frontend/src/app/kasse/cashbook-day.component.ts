import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, Day } from '../core/api/generated';
import { PageComponent } from '../ui/page.component';

/**
 * Kasse — open a cash-book day (Tagesabschluss).
 *
 * The API is consumed through the generated client (ADR-0003): no hand-written
 * URLs or payload shapes. This is the first screen of the rebuild and the proof
 * that the contract-first pipeline works end to end.
 *
 * The heading and the labels are message keys, so the screen names itself in whatever
 * language the clerk picked; the API's own status values are shown as the API reports them.
 */
@Component({
  selector: 'app-cashbook-day',
  imports: [FormsModule, PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.kasseTag.title" subtitleKey="page.kasseTag.subtitle">
      <section>
        <div class="form">
          <div class="field">
            <label for="business-date">{{ 'kasse.tag.businessDate' | transloco }}</label>
            <input
              id="business-date"
              name="businessDate"
              type="date"
              [ngModel]="businessDate()"
              (ngModelChange)="businessDate.set($event)"
            />
          </div>

          <div class="field">
            <label for="opening-cents">{{ 'kasse.tag.openingCents' | transloco }}</label>
            <input
              id="opening-cents"
              name="openingCents"
              type="number"
              [ngModel]="openingCents()"
              (ngModelChange)="openingCents.set(+$event)"
            />
          </div>

          <div class="field">
            <button type="button" (click)="open()">{{ 'kasse.tag.open' | transloco }}</button>
          </div>
        </div>

        @if (error(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }

        @if (day(); as d) {
          <dl>
            <dt>{{ 'kasse.tag.dayId' | transloco }}</dt>
            <dd data-testid="day-id">{{ d.ID }}</dd>
            <dt>{{ 'common.status' | transloco }}</dt>
            <dd data-testid="day-status">{{ d.Status }}</dd>
            <dt>{{ 'kasse.tag.openingCents' | transloco }}</dt>
            <dd>{{ d.OpeningCents }} {{ 'common.cents' | transloco }}</dd>
          </dl>
        }
      </section>
    </app-page>
  `,
})
export class CashbookDayComponent {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly businessDate = signal('2026-01-02');
  readonly openingCents = signal(0);
  readonly day = signal<Day | null>(null);
  readonly error = signal<string | null>(null);

  open(): void {
    this.error.set(null);
    this.api
      .openCashbookDay({
        business_date: this.businessDate(),
        opening_cents: this.openingCents(),
      })
      .subscribe({
        next: (d) => this.day.set(d),
        error: () => this.error.set(this.i18n.translate('kasse.tag.openFailed')),
      });
  }
}
