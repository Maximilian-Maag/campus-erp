import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { CampusErpApi, Day } from '../core/api/generated';

/**
 * Kasse — open a cash-book day (Tagesabschluss).
 *
 * The API is consumed through the generated client (ADR-0003): no hand-written
 * URLs or payload shapes. This is the first screen of the rebuild and the proof
 * that the contract-first pipeline works end to end.
 */
@Component({
  selector: 'app-cashbook-day',
  imports: [FormsModule],
  template: `
    <section aria-labelledby="cashbook-heading">
      <h2 id="cashbook-heading">Kassenbuch — Tag eröffnen</h2>

      <div class="form">
        <label for="business-date">Geschäftstag</label>
        <input
          id="business-date"
          name="businessDate"
          type="date"
          [ngModel]="businessDate()"
          (ngModelChange)="businessDate.set($event)"
        />

        <label for="opening-cents">Anfangsbestand (Cent)</label>
        <input
          id="opening-cents"
          name="openingCents"
          type="number"
          [ngModel]="openingCents()"
          (ngModelChange)="openingCents.set(+$event)"
        />

        <button type="button" (click)="open()">Eröffnen</button>
      </div>

      @if (error(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }

      @if (day(); as d) {
        <dl>
          <dt>Tag-ID</dt>
          <dd data-testid="day-id">{{ d.ID }}</dd>
          <dt>Status</dt>
          <dd data-testid="day-status">{{ d.Status }}</dd>
          <dt>Anfangsbestand</dt>
          <dd>{{ d.OpeningCents }} Cent</dd>
        </dl>
      }
    </section>
  `,
})
export class CashbookDayComponent {
  private readonly api = inject(CampusErpApi);

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
        error: () => this.error.set('Konnte den Kassenbuch-Tag nicht eröffnen.'),
      });
  }
}
