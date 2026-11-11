import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, Definition } from '../core/api/generated';
import { isoDay } from '../core/format';

/** The name the render job stores a report under: `reports/<id>-<from>-<to>.pdf` (docs/25). */
export function artefactName(reportID: string, from: string, to: string): string {
  return `${reportID}-${compactDay(from)}-${compactDay(to)}.pdf`;
}

/** A calendar day without its separators, the form the render job's file name uses. */
export function compactDay(day: string): string {
  return day.replace(/-/g, '');
}

/**
 * Auswertungen (reports).
 *
 * Lists the reports this system renders — the definitions come from the API's own catalogue
 * (`GET /api/reports`), so the screen offers what actually renders instead of a hardcoded list —
 * and queues one for a period. The render happens on the worker role and stores a PDF, so the
 * screen reports the job it was queued as and links the file it will produce: it does not wait for
 * a document it cannot see.
 */
@Component({
  selector: 'app-auswertungen',
  template: `
    <section aria-labelledby="reports-heading">
      <h2 id="reports-heading">Auswertungen</h2>

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
        <p role="status">Auswertungen werden geladen.</p>
      } @else if (reports().length === 0 && !loadError()) {
        <p role="status">Es ist keine Auswertung eingerichtet.</p>
      } @else if (!loadError()) {
        <table>
          <caption>
            Auswertungen mit ihren Spalten
          </caption>
          <thead>
            <tr>
              <th scope="col">Kennung</th>
              <th scope="col">Titel</th>
              <th scope="col">Spalten</th>
            </tr>
          </thead>
          <tbody>
            @for (r of reports(); track r.ID) {
              <tr [attr.data-testid]="'report-' + r.ID">
                <td>{{ r.ID }}</td>
                <td>{{ r.Title }}</td>
                <td>{{ columnList(r) }}</td>
              </tr>
            }
          </tbody>
        </table>

        <form (submit)="render($event)">
          <p>
            <label for="report-id">Auswertung</label>
            <select id="report-id" [value]="report()" (change)="report.set($any($event.target).value)">
              <option value="">Bitte wählen</option>
              @for (r of reports(); track r.ID) {
                <option [value]="r.ID">{{ r.Title }}</option>
              }
            </select>
          </p>
          <p>
            <label for="report-from">Von</label>
            <input id="report-from" type="date" [value]="from()" (input)="from.set($any($event.target).value)" />
          </p>
          <p>
            <label for="report-to">Bis</label>
            <input id="report-to" type="date" [value]="to()" (input)="to.set($any($event.target).value)" />
          </p>
          <button type="submit" [disabled]="busy()">
            {{ busy() ? 'Auswertung wird eingeplant.' : 'Auswertung erzeugen' }}
          </button>
        </form>

        @if (document(); as doc) {
          <p>
            <a [href]="doc.href">{{ doc.name }}</a>
          </p>
        }
      }
    </section>
  `,
})
export class AuswertungenComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly reports = signal<Definition[]>([]);
  readonly loading = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  readonly document = signal<{ name: string; href: string } | null>(null);

  readonly report = signal('');
  readonly from = signal(isoDay(new Date(new Date().getFullYear(), new Date().getMonth(), 1)));
  readonly to = signal(isoDay(new Date()));
  readonly busy = signal(false);

  /** Loads the catalogue: the reports this system renders. */
  ngOnInit(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listReports().subscribe({
      next: (reports) => {
        this.reports.set(reports);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Die Auswertungen konnten nicht geladen werden.');
        this.loading.set(false);
      },
    });
  }

  /** Queues the render of the chosen report over the stated period. */
  render(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    this.document.set(null);

    const id = this.report();
    if (id === '') {
      this.actionError.set('Es ist keine Auswertung gewählt.');
      return;
    }
    if (!isDay(this.from()) || !isDay(this.to())) {
      this.actionError.set('Ein Zeitraum aus zwei Tagen ist erforderlich.');
      return;
    }

    this.busy.set(true);
    this.api.requestReportRender({ report_id: id, from: this.from(), to: this.to() }).subscribe({
      next: (queued) => {
        const name = artefactName(id, this.from(), this.to());
        this.notice.set(
          `Auswertung eingeplant (Auftrag ${queued.job_id}). Die Datei liegt unter reports/${name}, sobald der Worker sie erzeugt hat.`
        );
        this.document.set({ name, href: `/api/artefacts/reports/${name}` });
        this.busy.set(false);
      },
      error: () => {
        this.actionError.set('Die Auswertung konnte nicht eingeplant werden.');
        this.busy.set(false);
      },
    });
  }

  /** A report's columns as an operator reads them (they are what the print view shows). */
  columnList(r: Definition): string {
    return r.Columns.map((c) => c.Title).join(', ');
  }
}

/** Whether a value is a calendar day in the form the API reads. */
export function isDay(value: string): boolean {
  return /^\d{4}-\d{2}-\d{2}$/.test(value);
}
