import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, EtlReport, EtlRun } from '../core/api/generated';
import { formatDay } from '../core/format';

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
  template: `
    <section aria-labelledby="imports-heading">
      <h2 id="imports-heading">Importe</h2>

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
        <p role="status">Datenübernahmen werden geladen.</p>
      } @else if (datasets().length === 0 && !loadError()) {
        <p role="status">Es sind keine Datenübernahmen eingerichtet.</p>
      } @else if (!loadError()) {
        <p>
          <label for="import-dataset">Datensatz</label>
          <select id="import-dataset" [value]="dataset()" (change)="choose($any($event.target).value)">
            <option value="">alle</option>
            @for (name of datasets(); track name) {
              <option [value]="name">{{ name }}</option>
            }
          </select>
        </p>
        <p>
          <button type="button" [disabled]="dataset() === '' || running()" (click)="run()">
            Datenübernahme starten
          </button>
          @if (dataset() === '') {
            <span class="hint">Für eine Datenübernahme ist ein Datensatz zu wählen.</span>
          }
        </p>

        @if (runs().length === 0) {
          <p role="status">Es ist noch keine Datenübernahme gelaufen.</p>
        } @else {
          <table>
            <caption>
              Die letzten Datenübernahmen mit Zeitpunkt, Person und Ergebnis
            </caption>
            <thead>
              <tr>
                <th scope="col">Zeitpunkt (UTC)</th>
                <th scope="col">Person</th>
                <th scope="col">Ergebnis</th>
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
  `,
})
export class ImportsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

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
      error: () => this.loadError.set('Die Datenübernahmen konnten nicht geladen werden.'),
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
        this.loadError.set('Die Läufe konnten nicht gelesen werden.');
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
      this.actionError.set('Es ist kein Datensatz gewählt.');
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
        this.actionError.set(`Die Datenübernahme „${dataset}“ ist fehlgeschlagen.`);
      },
    });
  }

  /** What a run did, in prose: read, loaded, and whether the source moved under it. */
  report(report: EtlReport): string {
    const drift = report.drift
      ? 'Die Quelle hat sich seit dem letzten Lauf geändert.'
      : report.previous
        ? 'Die Quelle ist gegenüber dem letzten Lauf unverändert.'
        : 'Es war der erste Lauf dieses Datensatzes.';
    return `Datenübernahme „${report.dataset}“: ${report.source} Zeilen gelesen, ${report.loaded} übernommen, Prüfsumme ${report.checksum}. ${drift}`;
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
