import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, BudgetPositionResponse, Center, ImportResult, Source } from '../core/api/generated';
import { formatCents } from '../core/format';

/**
 * Budget — Mittelherkunft und Bewirtschaftung (sources and centres).
 *
 * A **source** (Mittelquelle, a Haushaltsstelle) is where a title's money comes from; a **centre**
 * (Bewirtschaftungsstelle) is who administers it (`docs/10`). Both are configuration, so the screen
 * creates and lists them, and then names a position's two halves — one form for the chosen
 * position rather than a form per row, because naming a position is an occasional act, not a
 * per-row one.
 *
 * A source's chapter is optional (a third-party funder has no state chapter), the codes are the
 * identity an import and a person share, and a position that is not named yet is shown as such —
 * "no source" and "source cleared" are the same state and must not read as a missing value.
 */
@Component({
  selector: 'app-budget-sources',
  template: `
    <section aria-labelledby="sources-heading">
      <h2 id="sources-heading">Mittelherkunft und Bewirtschaftung</h2>

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
        <p role="status">Mittelherkunft wird geladen.</p>
      } @else if (!loadError()) {
        <table>
          <caption>
            Mittelquellen
          </caption>
          <thead>
            <tr>
              <th scope="col">Schlüssel</th>
              <th scope="col">Bezeichnung</th>
              <th scope="col">Kapitel</th>
            </tr>
          </thead>
          <tbody>
            @for (s of sources(); track s.ID) {
              <tr [attr.data-testid]="'source-' + s.ID">
                <td>{{ s.Code }}</td>
                <td>{{ s.Name }}</td>
                <td>{{ s.Chapter }}</td>
              </tr>
            } @empty {
              <tr>
                <td colspan="3">Es ist keine Mittelquelle angelegt.</td>
              </tr>
            }
          </tbody>
        </table>

        <form (submit)="createSource($event)">
          <p>
            <label for="source-code">Schlüssel</label>
            <input id="source-code" [value]="sourceCode()" (input)="sourceCode.set($any($event.target).value)" />
          </p>
          <p>
            <label for="source-name">Bezeichnung</label>
            <input id="source-name" [value]="sourceName()" (input)="sourceName.set($any($event.target).value)" />
          </p>
          <p>
            <label for="source-chapter">Kapitel</label>
            <input id="source-chapter" [value]="sourceChapter()" (input)="sourceChapter.set($any($event.target).value)" />
          </p>
          <button type="submit" [disabled]="busy()">Mittelquelle anlegen</button>
        </form>

        <table>
          <caption>
            Bewirtschaftungsstellen
          </caption>
          <thead>
            <tr>
              <th scope="col">Schlüssel</th>
              <th scope="col">Bezeichnung</th>
            </tr>
          </thead>
          <tbody>
            @for (c of centers(); track c.ID) {
              <tr [attr.data-testid]="'center-' + c.ID">
                <td>{{ c.Code }}</td>
                <td>{{ c.Name }}</td>
              </tr>
            } @empty {
              <tr>
                <td colspan="2">Es ist keine Bewirtschaftungsstelle angelegt.</td>
              </tr>
            }
          </tbody>
        </table>

        <form (submit)="createCenter($event)">
          <p>
            <label for="center-code">Schlüssel</label>
            <input id="center-code" [value]="centerCode()" (input)="centerCode.set($any($event.target).value)" />
          </p>
          <p>
            <label for="center-name">Bezeichnung</label>
            <input id="center-name" [value]="centerName()" (input)="centerName.set($any($event.target).value)" />
          </p>
          <button type="submit" [disabled]="busy()">Bewirtschaftungsstelle anlegen</button>
        </form>

        @if (positions().length === 0) {
          <p role="status">Es ist keine Haushaltsstelle angelegt.</p>
        } @else {
          <form (submit)="assign($event)">
            <p>
              <label for="assign-position">Haushaltsstelle</label>
              <select id="assign-position" [value]="position()" (change)="position.set($any($event.target).value)">
                <option value="">Bitte wählen</option>
                @for (p of positions(); track p.position.ID) {
                  <option [value]="p.position.ID">
                    {{ p.position.Chapter }}/{{ p.position.Title }} — {{ p.position.Description }}
                  </option>
                }
              </select>
            </p>
            <p>
              <label for="assign-source">Mittelquelle</label>
              <select id="assign-source" [value]="source()" (change)="source.set($any($event.target).value)">
                <option value="">Keine</option>
                @for (s of sources(); track s.ID) {
                  <option [value]="s.ID">{{ s.Code }} — {{ s.Name }}</option>
                }
              </select>
            </p>
            <p>
              <label for="assign-center">Bewirtschaftungsstelle</label>
              <select id="assign-center" [value]="center()" (change)="center.set($any($event.target).value)">
                <option value="">Keine</option>
                @for (c of centers(); track c.ID) {
                  <option [value]="c.ID">{{ c.Code }} — {{ c.Name }}</option>
                }
              </select>
            </p>
            <button type="submit" [disabled]="busy()">Zuordnung speichern</button>
          </form>

          <table>
            <caption>
              Haushaltsstellen mit Mittelherkunft und Bewirtschaftung
            </caption>
            <thead>
              <tr>
                <th scope="col">Haushaltsstelle</th>
                <th scope="col">Bezeichnung</th>
                <th scope="col" class="amount">Ansatz</th>
                <th scope="col">Mittelquelle</th>
                <th scope="col">Bewirtschaftung</th>
              </tr>
            </thead>
            <tbody>
              @for (p of positions(); track p.position.ID) {
                <tr [attr.data-testid]="'position-' + p.position.ID">
                  <td>{{ p.position.Chapter }}/{{ p.position.Title }}</td>
                  <td>{{ p.position.Description }}</td>
                  <td class="amount">{{ amount(p.position.AllocatedCents) }}</td>
                  <td>{{ sourceNameOf(p) }}</td>
                  <td>{{ centerNameOf(p) }}</td>
                </tr>
              }
            </tbody>
          </table>
        }

        <form (submit)="runImport($event)">
          <h3 id="budget-import-heading">Budgetdatei einlesen</h3>
          <p>
            <label for="import-layout">Format</label>
            <input
              id="import-layout"
              placeholder="budget-import/v1"
              [value]="layout()"
              (input)="layout.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="import-content">Inhalt der Budgetdatei</label>
            <textarea
              id="import-content"
              rows="6"
              [value]="file()"
              (input)="file.set($any($event.target).value)"
            ></textarea>
          </p>
          <button type="submit" [disabled]="busy()">Einlesen</button>
        </form>
      }
    </section>
  `,
})
export class BudgetSourcesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly sources = signal<Source[]>([]);
  readonly centers = signal<Center[]>([]);
  readonly positions = signal<BudgetPositionResponse[]>([]);
  readonly loading = signal(false);
  readonly busy = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  readonly sourceCode = signal('');
  readonly sourceName = signal('');
  readonly sourceChapter = signal('');
  readonly centerCode = signal('');
  readonly centerName = signal('');
  readonly position = signal('');
  readonly source = signal('');
  readonly center = signal('');
  /** The file's format; empty means the standard layout, and the placeholder names it. */
  readonly layout = signal('');
  readonly file = signal('');

  /** Loads the three lists the screen shows: sources, centres and the positions they name. */
  ngOnInit(): void {
    this.reload(true);
  }

  /** Reads the lists again. `initial` distinguishes a failed first load from a failed refresh. */
  private reload(initial: boolean): void {
    this.loading.set(initial);
    this.loadError.set(null);
    this.api.listBudgetSources().subscribe({
      next: (sources) => {
        this.sources.set(sources);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Die Mittelherkunft konnte nicht geladen werden.');
        this.loading.set(false);
      },
    });
    this.api.listBudgetCenters().subscribe({
      next: (centers) => this.centers.set(centers),
      error: () => this.loadError.set('Die Bewirtschaftungsstellen konnten nicht geladen werden.'),
    });
    this.api.listBudgetPositions().subscribe({
      next: (positions) => this.positions.set(positions),
      error: () => this.loadError.set('Die Haushaltsstellen konnten nicht geladen werden.'),
    });
  }

  createSource(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    if (this.sourceCode().trim() === '' || this.sourceName().trim() === '') {
      this.actionError.set('Schlüssel und Bezeichnung der Mittelquelle sind erforderlich.');
      return;
    }
    this.busy.set(true);
    this.api
      .createBudgetSource({ code: this.sourceCode(), name: this.sourceName(), chapter: this.sourceChapter() })
      .subscribe({
        next: (saved) => {
          this.sources.update((list) => [...list, saved].sort((a, b) => a.Code.localeCompare(b.Code)));
          this.sourceCode.set('');
          this.sourceName.set('');
          this.sourceChapter.set('');
          this.notice.set(`Mittelquelle ${saved.Code} angelegt.`);
          this.busy.set(false);
        },
        error: () => {
          this.actionError.set('Die Mittelquelle konnte nicht angelegt werden.');
          this.busy.set(false);
        },
      });
  }

  createCenter(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    if (this.centerCode().trim() === '' || this.centerName().trim() === '') {
      this.actionError.set('Schlüssel und Bezeichnung der Bewirtschaftungsstelle sind erforderlich.');
      return;
    }
    this.busy.set(true);
    this.api.createBudgetCenter({ code: this.centerCode(), name: this.centerName() }).subscribe({
      next: (saved) => {
        this.centers.update((list) => [...list, saved].sort((a, b) => a.Code.localeCompare(b.Code)));
        this.centerCode.set('');
        this.centerName.set('');
        this.notice.set(`Bewirtschaftungsstelle ${saved.Code} angelegt.`);
        this.busy.set(false);
      },
      error: () => {
        this.actionError.set('Die Bewirtschaftungsstelle konnte nicht angelegt werden.');
        this.busy.set(false);
      },
    });
  }

  /** Names the chosen position's source and centre; an empty choice clears that half. */
  assign(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);

    const id = Number(this.position());
    if (id <= 0) {
      this.actionError.set('Es ist keine Haushaltsstelle gewählt.');
      return;
    }
    const body = {
      source_id: this.source() === '' ? null : Number(this.source()),
      center_id: this.center() === '' ? null : Number(this.center()),
    };
    this.busy.set(true);
    this.api.assignBudgetPosition(id, body).subscribe({
      next: (saved) => {
        this.positions.update((list) =>
          list.map((p) => (p.position.ID === saved.ID ? { ...p, position: saved } : p))
        );
        this.notice.set('Die Zuordnung ist gespeichert.');
        this.busy.set(false);
      },
      error: () => {
        this.actionError.set('Die Zuordnung konnte nicht gespeichert werden.');
        this.busy.set(false);
      },
    });
  }

  /**
   * Reads a budget file: it creates the sources, the centres and the positions the file carries and
   * names each position from its own line (`docs/10`). A file with an unreadable line is refused
   * whole, so a failed read changes nothing on screen either.
   */
  runImport(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    if (this.file().trim() === '') {
      this.actionError.set('Es ist keine Budgetdatei eingelesen.');
      return;
    }
    this.busy.set(true);
    // An empty format is the standard layout: the operator does not have to name it.
    this.api.importBudget({ layout: this.layout(), content: this.file() }).subscribe({
      next: (res) => {
        this.notice.set(this.importNotice(res));
        this.file.set('');
        this.busy.set(false);
        this.reload(false);
      },
      error: () => {
        this.actionError.set('Die Budgetdatei konnte nicht eingelesen werden.');
        this.busy.set(false);
      },
    });
  }

  /** What an import did, in prose: created and already-there read differently. */
  importNotice(res: ImportResult): string {
    return (
      `${res.SourcesCreated} Mittelquellen, ${res.CentersCreated} Bewirtschaftungsstellen und ` +
      `${res.PositionsCreated} Haushaltsstellen angelegt; ` +
      `${res.SourcesKept} Mittelquellen und ${res.PositionsKept} Haushaltsstellen waren schon vorhanden; ` +
      `${res.PositionsNamed} Haushaltsstellen sind zugeordnet.`
    );
  }

  /** The name of a position's source, or a plain "keine" when it names none. */
  sourceNameOf(p: BudgetPositionResponse): string {
    if (p.position.SourceID === null) {
      return 'keine';
    }
    const found = this.sources().find((s) => s.ID === p.position.SourceID);
    return found === undefined ? String(p.position.SourceID) : `${found.Code} — ${found.Name}`;
  }

  /** The name of a position's centre, or a plain "keine" when it names none. */
  centerNameOf(p: BudgetPositionResponse): string {
    if (p.position.CenterID === null) {
      return 'keine';
    }
    const found = this.centers().find((c) => c.ID === p.position.CenterID);
    return found === undefined ? String(p.position.CenterID) : `${found.Code} — ${found.Name}`;
  }

  amount(cents: number): string {
    return formatCents(cents);
  }
}
