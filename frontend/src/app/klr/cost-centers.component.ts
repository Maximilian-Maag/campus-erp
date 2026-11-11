import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, CostCenter, KlrReport } from '../core/api/generated';
import { formatCents } from '../core/format';

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
 * bare negative amount in a Kostenrechnung invites the wrong reading.
 */
@Component({
  selector: 'app-klr-cost-centers',
  template: `
    <section aria-labelledby="klr-centers-heading">
      <h2 id="klr-centers-heading">Kostenstellen</h2>

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
        <p role="status">Kostenstellen werden geladen.</p>
      } @else if (!listError()) {
        <table>
          <caption>
            Kostenstellen mit Nummer und Bezeichnung
          </caption>
          <thead>
            <tr>
              <th scope="col">Nummer</th>
              <th scope="col">Bezeichnung</th>
              <th scope="col">Bericht</th>
            </tr>
          </thead>
          <tbody>
            @for (center of centers(); track center.ID) {
              <tr [attr.data-testid]="'center-' + center.ID">
                <td>{{ center.Number }}</td>
                <td>{{ center.Name }}</td>
                <td>
                  <button type="button" (click)="showReport(center)">Anzeigen</button>
                </td>
              </tr>
            } @empty {
              <tr>
                <td colspan="3">Es ist keine Kostenstelle angelegt.</td>
              </tr>
            }
          </tbody>
        </table>
      }

      @if (report(); as r) {
        <h3 id="klr-report-heading">Kosten und Erlöse</h3>
        <dl aria-labelledby="klr-report-heading" [attr.data-testid]="'report-' + reporting()?.ID">
          <dt>Kostenstelle</dt>
          <dd>{{ reporting()?.Number }} {{ reporting()?.Name }}</dd>
          <dt>Kosten</dt>
          <dd>{{ euros(r.CostsCents) }}</dd>
          <dt>Erlöse</dt>
          <dd>{{ euros(r.RevenuesCents) }}</dd>
          <dt>Saldo</dt>
          <dd>{{ balanceLabel(r) }}</dd>
        </dl>
      }

      <form (submit)="create($event)">
        <h3 id="klr-center-new-heading">Kostenstelle aufnehmen</h3>
        <p>
          <label for="center-number">Nummer</label>
          <input id="center-number" [value]="number()" (input)="number.set($any($event.target).value)" />
        </p>
        <p>
          <label for="center-name">Bezeichnung</label>
          <input id="center-name" [value]="name()" (input)="name.set($any($event.target).value)" />
        </p>
        <button type="submit" [disabled]="busy()">Kostenstelle aufnehmen</button>
      </form>
    </section>
  `,
})
export class CostCentersComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly centers = signal<CostCenter[]>([]);
  readonly loading = signal(true);
  readonly busy = signal(false);
  /** A read failure is kept apart per read: the list and one centre's report fail for themselves. */
  readonly listError = signal<string | null>(null);
  readonly reportError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  readonly report = signal<KlrReport | null>(null);
  /** The centre whose report is on screen, so the report is never shown without saying whose it is. */
  readonly reporting = signal<CostCenter | null>(null);

  readonly number = signal('');
  readonly name = signal('');

  ngOnInit(): void {
    this.load();
  }

  /** The catalogue is small and complete in one read: it is not filtered by anything. */
  load(): void {
    this.loading.set(true);
    this.listError.set(null);
    this.api.listCostCenters().subscribe({
      next: (centers) => {
        this.centers.set(centers ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.listError.set('Die Kostenstellen konnten nicht geladen werden.');
        this.loading.set(false);
      },
    });
  }

  /** Reads one centre's report — a second request per centre, so it is asked for on demand. */
  showReport(center: CostCenter): void {
    this.reportError.set(null);
    this.reporting.set(center);
    this.api.getKLRReport(center.ID).subscribe({
      next: (report) => this.report.set(report),
      error: () => {
        // The stale report of a previously shown centre must not stay on screen under a new heading.
        this.report.set(null);
        this.reportError.set('Der Bericht dieser Kostenstelle konnte nicht geladen werden.');
      },
    });
  }

  create(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    if (this.number().trim() === '' || this.name().trim() === '') {
      this.actionError.set('Nummer und Bezeichnung der Kostenstelle sind erforderlich.');
      return;
    }
    this.busy.set(true);
    this.api.createCostCenter({ number: this.number().trim(), name: this.name().trim() }).subscribe({
      next: (saved) => {
        this.centers.update((list) => [...list, saved].sort((a, b) => a.Number.localeCompare(b.Number)));
        this.number.set('');
        this.name.set('');
        this.notice.set(`Kostenstelle ${saved.Number} aufgenommen.`);
        this.busy.set(false);
      },
      error: () => {
        this.actionError.set('Die Kostenstelle konnte nicht aufgenommen werden.');
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
      return 'ausgeglichen';
    }
    return (balance > 0 ? 'Überschuss ' : 'Unterdeckung ') + formatCents(Math.abs(balance));
  }
}
