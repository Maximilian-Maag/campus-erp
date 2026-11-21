import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, EtlReport, EtlRun } from '../core/api/generated';
import { formatDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * Importe — Datenübernahmen aus dem Altsystem (docs/19).
 *
 * An ETL dataset is a named source bound to a canonical sink; running one is an administrative act
 * that changes canonical data, so the routes behind this screen are the `erp-admin` ones and the
 * import is offered under the datasets the server registers — the screen lists what actually
 * exists, it does not carry its own list.
 *
 * The history is the audit stream read back (`GET /api/etl/runs`): a run's own event *is* its
 * record, so the screen shows what the run recorded (source rows, loaded rows, checksum, and
 * whether the source drifted from the previous run) rather than a second bookkeeping of it.
 */
@Component({
  selector: 'app-imports',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.importe.title" subtitleKey="page.importe.subtitle">
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

        @if (loading() && datasets().length === 0) {
          <p role="status">{{ 'auswertung.imports.loading' | transloco }}</p>
        } @else if (datasets().length === 0 && !loadError()) {
          <p role="status">{{ 'auswertung.imports.empty' | transloco }}</p>
        } @else if (!loadError()) {
          <div class="field">
            <label for="import-dataset">{{ 'auswertung.imports.dataset' | transloco }}</label>
            <select id="import-dataset" [value]="dataset()" (change)="choose($any($event.target).value)">
              <option value="">{{ 'auswertung.imports.allDatasets' | transloco }}</option>
              @for (name of datasets(); track name) {
                <option [value]="name">{{ name }}</option>
              }
            </select>
          </div>
          <p>
            <button type="button" [disabled]="dataset() === '' || running()" (click)="run()">
              {{ 'auswertung.imports.run' | transloco }}
            </button>
            @if (dataset() === '') {
              <span class="hint">{{ 'auswertung.imports.datasetRequiredHint' | transloco }}</span>
            }
          </p>

          @if (runs().length === 0) {
            <p role="status">{{ 'auswertung.imports.noRuns' | transloco }}</p>
          } @else {
            <table>
              <caption>
                {{ 'auswertung.imports.historyCaption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'auswertung.imports.timestampUtc' | transloco }}</th>
                  <th scope="col">{{ 'auswertung.imports.person' | transloco }}</th>
                  <th scope="col">{{ 'auswertung.imports.result' | transloco }}</th>
                </tr>
              </thead>
              <tbody>
                @for (run of runs(); track $index) {
                  <tr [attr.data-testid]="'run-' + $index">
                    <td>{{ at(run.at) }}</td>
                    <td>{{ run.actor }}</td>
                    <td>{{ run.summary }}</td>
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
export class ImportsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly datasets = signal<string[]>([]);
  readonly runs = signal<EtlRun[]>([]);
  readonly dataset = signal('');
  readonly loading = signal(false);
  readonly running = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  ngOnInit(): void {
    this.api.listETLDatasets().subscribe({
      next: (response) => this.datasets.set(response?.datasets ?? []),
      error: () => this.loadError.set(this.i18n.translate('auswertung.imports.datasetsLoadFailed')),
    });
    this.loadHistory();
  }

  /** The history of the chosen dataset — or of every dataset when none is chosen. */
  loadHistory(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listETLRuns(this.dataset(), '20').subscribe({
      next: (response) => {
        this.runs.set(response?.runs ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('auswertung.imports.runsLoadFailed'));
        this.loading.set(false);
      },
    });
  }

  /** Choosing a dataset narrows the history and offers the run for that one dataset. */
  choose(name: string): void {
    this.dataset.set(name);
    this.notice.set(null);
    this.actionError.set(null);
    this.loadHistory();
  }

  run(): void {
    const dataset = this.dataset();
    if (dataset === '') {
      this.actionError.set(this.i18n.translate('auswertung.imports.datasetRequired'));
      return;
    }
    this.running.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.runETLDataset({ dataset }).subscribe({
      next: (report) => {
        this.running.set(false);
        this.notice.set(this.report(report));
        // The run is now the newest entry in the history, so read it back rather than assuming it.
        this.loadHistory();
      },
      error: () => {
        this.running.set(false);
        this.actionError.set(this.i18n.translate('auswertung.imports.runFailed', { dataset }));
      },
    });
  }

  /** What a run did, in prose: read, loaded, and whether the source moved under it. */
  report(report: EtlReport): string {
    const drift = report.drift
      ? this.i18n.translate('auswertung.imports.driftChanged')
      : report.previous
        ? this.i18n.translate('auswertung.imports.driftUnchanged')
        : this.i18n.translate('auswertung.imports.firstRun');
    return this.i18n.translate('auswertung.imports.summary', {
      dataset: report.dataset,
      source: report.source,
      loaded: report.loaded,
      checksum: report.checksum,
      drift,
    });
  }

  /**
   * A run's timestamp in **UTC**, like `formatDay`: the API records runs in UTC, so showing them in
   * a viewer's own zone would make two people read two different histories of one import.
   */
  at(iso: string): string {
    const parsed = new Date(iso);
    if (Number.isNaN(parsed.getTime())) {
      return iso;
    }
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${formatDay(iso)}, ${pad(parsed.getUTCHours())}:${pad(parsed.getUTCMinutes())}`;
  }
}
