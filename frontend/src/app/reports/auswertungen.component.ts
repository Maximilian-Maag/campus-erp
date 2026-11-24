import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, Definition } from '../core/api/generated';
import { isoDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

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
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.auswertungen.title" subtitleKey="page.auswertungen.subtitle">
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

        @if (loading()) {
          <p role="status">{{ 'auswertung.reports.loading' | transloco }}</p>
        } @else if (reports().length === 0 && !loadError()) {
          <p role="status">{{ 'auswertung.reports.empty' | transloco }}</p>
        } @else if (!loadError()) {
          <table>
            <caption>
              {{ 'auswertung.reports.listCaption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'auswertung.reports.id' | transloco }}</th>
                <th scope="col">{{ 'common.title' | transloco }}</th>
                <th scope="col">{{ 'auswertung.reports.columns' | transloco }}</th>
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
            <div class="field">
              <label for="report-id">{{ 'auswertung.reports.report' | transloco }}</label>
              <select id="report-id" [value]="report()" (change)="report.set($any($event.target).value)">
                <option value="">{{ 'common.choose' | transloco }}</option>
                @for (r of reports(); track r.ID) {
                  <option [value]="r.ID" [selected]="'' + r.ID === report()">{{ r.Title }}</option>
                }
              </select>
            </div>
            <div class="field">
              <label for="report-from">{{ 'common.from' | transloco }}</label>
              <input id="report-from" type="date" [value]="from()" (input)="from.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="report-to">{{ 'common.to' | transloco }}</label>
              <input id="report-to" type="date" [value]="to()" (input)="to.set($any($event.target).value)" />
            </div>
            <div class="field">
              <button type="submit" [disabled]="busy()">
                {{
                  busy()
                    ? ('auswertung.reports.rendering' | transloco)
                    : ('auswertung.reports.render' | transloco)
                }}
              </button>
            </div>
          </form>

          @if (document(); as doc) {
            <p>
              <a [href]="doc.href">{{ doc.name }}</a>
            </p>
          }
        }
      </section>
    </app-page>
  `,
})
export class AuswertungenComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

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
        this.loadError.set(this.i18n.translate('auswertung.reports.loadFailed'));
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
      this.actionError.set(this.i18n.translate('auswertung.reports.reportRequired'));
      return;
    }
    if (!isDay(this.from()) || !isDay(this.to())) {
      this.actionError.set(this.i18n.translate('auswertung.reports.periodRequired'));
      return;
    }

    this.busy.set(true);
    this.api.requestReportRender({ report_id: id, from: this.from(), to: this.to() }).subscribe({
      next: (queued) => {
        const name = artefactName(id, this.from(), this.to());
        this.notice.set(
          this.i18n.translate('auswertung.reports.queued', { job: queued.job_id, name }),
        );
        this.document.set({ name, href: `/api/artefacts/reports/${name}` });
        this.busy.set(false);
      },
      error: () => {
        this.actionError.set(this.i18n.translate('auswertung.reports.renderFailed'));
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
