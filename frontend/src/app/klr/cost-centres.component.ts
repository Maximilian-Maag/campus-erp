import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, CostCentre, KlrReport } from '../core/api/generated';
import { formatCents } from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * KLR — Kostenstellen (cost centres).
 *
 * The anchor of the whole KLR: an accounting record, an allocation and a cost-unit key all hang off
 * a cost centre, and the centres were selectable in the other KLR screens but could not be seen or
 * created anywhere in the UI. The list and the create route existed; the screen did not.
 *
 * The report is read per centre on demand rather than for every row: it is a second request per
 * centre, and an operator looks at one centre's Kosten and Erlöse at a time. The balance is shown
 * with its meaning — a positive figure is an Überschuss, a negative one an Unterdeckung — because a
 * bare negative amount in a Kostenrechnung invites the wrong reading, and both words are message
 * keys so the reading holds in either language.
 */
@Component({
  selector: 'app-klr-cost-centres',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.klrKostenstellen.title" subtitleKey="page.klrKostenstellen.subtitle">
      <section>
        @if (listError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (reportError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (actionError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (notice(); as n) {
          <p role="status" class="notice">{{ n }}</p>
        }

        @if (loading()) {
          <p role="status">{{ 'klr.common.loadingCostCentres' | transloco }}</p>
        } @else if (!listError()) {
          <table>
            <caption>
              {{ 'klr.costCentres.listCaption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.number' | transloco }}</th>
                <th scope="col">{{ 'common.description' | transloco }}</th>
                <th scope="col">{{ 'klr.costCentres.reportColumn' | transloco }}</th>
              </tr>
            </thead>
            <tbody>
              @for (centre of centres(); track centre.ID) {
                <tr [attr.data-testid]="'centre-' + centre.ID">
                  <td>{{ centre.Number }}</td>
                  <td>{{ centre.Name }}</td>
                  <td>
                    <button type="button" (click)="showReport(centre)">{{ 'klr.common.show' | transloco }}</button>
                  </td>
                </tr>
              } @empty {
                <tr>
                  <td colspan="3">{{ 'klr.costCentres.empty' | transloco }}</td>
                </tr>
              }
            </tbody>
          </table>
        }

        @if (report(); as r) {
          <h2 id="klr-report-heading">{{ 'klr.costCentres.reportHeading' | transloco }}</h2>
          <dl aria-labelledby="klr-report-heading" [attr.data-testid]="'report-' + reporting()?.ID">
            <dt>{{ 'common.costCentre' | transloco }}</dt>
            <dd>{{ reporting()?.Number }} {{ reporting()?.Name }}</dd>
            <dt>{{ 'klr.common.costs' | transloco }}</dt>
            <dd>{{ euros(r.CostsCents) }}</dd>
            <dt>{{ 'klr.common.revenues' | transloco }}</dt>
            <dd>{{ euros(r.RevenuesCents) }}</dd>
            <dt>{{ 'klr.common.balance' | transloco }}</dt>
            <dd>{{ balanceLabel(r) }}</dd>
          </dl>
        }

        <form (submit)="create($event)">
          <h2 id="klr-centre-new-heading">{{ 'klr.costCentres.createHeading' | transloco }}</h2>
          <div class="field">
            <label for="centre-number">{{ 'common.number' | transloco }}</label>
            <input id="centre-number" [value]="number()" (input)="number.set($any($event.target).value)" />
          </div>
          <div class="field">
            <label for="centre-name">{{ 'common.description' | transloco }}</label>
            <input id="centre-name" [value]="name()" (input)="name.set($any($event.target).value)" />
          </div>
          <div class="field">
            <button type="submit" [disabled]="busy()">{{ 'klr.costCentres.createButton' | transloco }}</button>
          </div>
        </form>
      </section>
    </app-page>
  `,
})
export class CostCentresComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly centres = signal<CostCentre[]>([]);
  readonly loading = signal(true);
  readonly busy = signal(false);
  /** A read failure is kept apart per read: the list and one centre's report fail for themselves. */
  readonly listError = signal<string | null>(null);
  readonly reportError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  readonly report = signal<KlrReport | null>(null);
  /** The centre whose report is on screen, so the report is never shown without saying whose it is. */
  readonly reporting = signal<CostCentre | null>(null);

  readonly number = signal('');
  readonly name = signal('');

  ngOnInit(): void {
    this.load();
  }

  /** The catalogue is small and complete in one read: it is not filtered by anything. */
  load(): void {
    this.loading.set(true);
    this.listError.set(null);
    this.api.listCostCentres().subscribe({
      next: (centres) => {
        this.centres.set(centres ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.listError.set(this.i18n.translate('klr.common.loadCostCentresFailed'));
        this.loading.set(false);
      },
    });
  }

  /** Reads one centre's report — a second request per centre, so it is asked for on demand. */
  showReport(centre: CostCentre): void {
    this.reportError.set(null);
    this.reporting.set(centre);
    // The previous centre's figures go with its heading, not with the new one (#174).
    this.report.set(null);
    this.api.getKLRReport(centre.ID).subscribe({
      next: (report) => {
        // A response for a centre that is no longer shown belongs to nobody.
        if (this.reporting()?.ID !== centre.ID) return;
        this.report.set(report);
      },
      error: () => {
        if (this.reporting()?.ID !== centre.ID) return;
        this.reportError.set(this.i18n.translate('klr.costCentres.reportFailed'));
      },
    });
  }

  create(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    if (this.number().trim() === '' || this.name().trim() === '') {
      this.actionError.set(this.i18n.translate('klr.costCentres.createRequired'));
      return;
    }
    this.busy.set(true);
    this.api.createCostCentre({ number: this.number().trim(), name: this.name().trim() }).subscribe({
      next: (saved) => {
        this.centres.update((list) => [...list, saved].sort((a, b) => a.Number.localeCompare(b.Number)));
        this.number.set('');
        this.name.set('');
        this.notice.set(this.i18n.translate('klr.costCentres.created', { number: saved.Number }));
        this.busy.set(false);
      },
      error: () => {
        this.actionError.set(this.i18n.translate('klr.costCentres.createFailed'));
        this.busy.set(false);
      },
    });
  }

  readonly euros = formatCents;

  /**
   * The balance of a centre, with what its sign means. Erlöse above Kosten is a surplus; below is a
   * shortfall — a Kostenrechnung that shows a bare minus invites the wrong reading.
   */
  balanceLabel(report: KlrReport): string {
    const balance = report.RevenuesCents - report.CostsCents;
    // The zero case is decided first, so the sign test only ever sees a non-zero balance.
    if (balance === 0) {
      return this.i18n.translate('klr.common.balanced');
    }
    const word = this.i18n.translate(balance > 0 ? 'klr.common.surplus' : 'klr.common.shortfall');
    return word + ' ' + formatCents(Math.abs(balance));
  }
}
