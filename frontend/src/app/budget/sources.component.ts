import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, BudgetPositionResponse, Center, ImportResult, Source } from '../core/api/generated';
import { formatCents } from '../core/format';
import { PageComponent } from '../ui/page.component';

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
 *
 * The headings and each subsection's h3 are message keys, so the screen keeps the vocabulary of the
 * navigation — "Bewirtschaftungsstelle" is the same word here as in the dictionary, not a synonym
 * chosen by this template.
 */
@Component({
  selector: 'app-budget-sources',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.haushaltHerkunft.title" subtitleKey="page.haushaltHerkunft.subtitle">
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
          <p role="status">{{ 'haushalt.sources.loading' | transloco }}</p>
        } @else if (!loadError()) {
          <table>
            <caption>
              {{ 'haushalt.sources.sourcesCaption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.code' | transloco }}</th>
                <th scope="col">{{ 'common.description' | transloco }}</th>
                <th scope="col">{{ 'common.chapter' | transloco }}</th>
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
                  <td colspan="3">{{ 'haushalt.sources.emptySources' | transloco }}</td>
                </tr>
              }
            </tbody>
          </table>

          <form class="form" (submit)="createSource($event)">
            <div class="field">
              <label for="source-code">{{ 'common.code' | transloco }}</label>
              <input id="source-code" [value]="sourceCode()" (input)="sourceCode.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="source-name">{{ 'common.description' | transloco }}</label>
              <input id="source-name" [value]="sourceName()" (input)="sourceName.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="source-chapter">{{ 'common.chapter' | transloco }}</label>
              <input id="source-chapter" [value]="sourceChapter()" (input)="sourceChapter.set($any($event.target).value)" />
            </div>
            <div class="field">
              <button type="submit" [disabled]="busy()">{{ 'haushalt.sources.createSource' | transloco }}</button>
            </div>
          </form>

          <table>
            <caption>
              {{ 'haushalt.sources.centresCaption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.code' | transloco }}</th>
                <th scope="col">{{ 'common.description' | transloco }}</th>
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
                  <td colspan="2">{{ 'haushalt.sources.emptyCentres' | transloco }}</td>
                </tr>
              }
            </tbody>
          </table>

          <form class="form" (submit)="createCenter($event)">
            <div class="field">
              <label for="center-code">{{ 'common.code' | transloco }}</label>
              <input id="center-code" [value]="centerCode()" (input)="centerCode.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="center-name">{{ 'common.description' | transloco }}</label>
              <input id="center-name" [value]="centerName()" (input)="centerName.set($any($event.target).value)" />
            </div>
            <div class="field">
              <button type="submit" [disabled]="busy()">{{ 'haushalt.sources.createCentre' | transloco }}</button>
            </div>
          </form>

          @if (positions().length === 0) {
            <p role="status">{{ 'haushalt.sources.emptyPositions' | transloco }}</p>
          } @else {
            <form class="form" (submit)="assign($event)">
              <div class="field">
                <label for="assign-position">{{ 'haushalt.sources.position' | transloco }}</label>
                <select id="assign-position" [value]="position()" (change)="position.set($any($event.target).value)">
                  <option value="">{{ 'common.choose' | transloco }}</option>
                  @for (p of positions(); track p.position.ID) {
                    <option [value]="p.position.ID">
                      {{ p.position.Chapter }}/{{ p.position.Title }} — {{ p.position.Description }}
                    </option>
                  }
                </select>
              </div>
              <div class="field">
                <label for="assign-source">{{ 'haushalt.sources.source' | transloco }}</label>
                <select id="assign-source" [value]="source()" (change)="source.set($any($event.target).value)">
                  <option value="">{{ 'haushalt.sources.noneChoice' | transloco }}</option>
                  @for (s of sources(); track s.ID) {
                    <option [value]="s.ID">{{ s.Code }} — {{ s.Name }}</option>
                  }
                </select>
              </div>
              <div class="field">
                <label for="assign-center">{{ 'haushalt.sources.centre' | transloco }}</label>
                <select id="assign-center" [value]="center()" (change)="center.set($any($event.target).value)">
                  <option value="">{{ 'haushalt.sources.noneChoice' | transloco }}</option>
                  @for (c of centers(); track c.ID) {
                    <option [value]="c.ID">{{ c.Code }} — {{ c.Name }}</option>
                  }
                </select>
              </div>
              <div class="field">
                <button type="submit" [disabled]="busy()">{{ 'haushalt.sources.assign' | transloco }}</button>
              </div>
            </form>

            <table>
              <caption>
                {{ 'haushalt.sources.positionsCaption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'haushalt.sources.position' | transloco }}</th>
                  <th scope="col">{{ 'common.description' | transloco }}</th>
                  <th scope="col" class="amount">{{ 'haushalt.common.allocated' | transloco }}</th>
                  <th scope="col">{{ 'haushalt.sources.source' | transloco }}</th>
                  <th scope="col">{{ 'haushalt.sources.administration' | transloco }}</th>
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
            @if (hasMore()) {
              <p>
                <button
                  type="button"
                  data-testid="load-more-positions"
                  [disabled]="loadingMore()"
                  (click)="loadMore()"
                >
                  {{
                    (loadingMore() ? 'haushalt.sources.loadingMore' : 'haushalt.sources.loadMorePositions')
                      | transloco
                  }}
                </button>
              </p>
            }
          }

          <form class="form" (submit)="runImport($event)">
            <h2 id="budget-import-heading">{{ 'haushalt.sources.importHeading' | transloco }}</h2>
            <div class="field">
              <label for="import-layout">{{ 'haushalt.sources.format' | transloco }}</label>
              <input
                id="import-layout"
                placeholder="budget-import/v1"
                [value]="layout()"
                (input)="layout.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="import-content">{{ 'haushalt.sources.fileContent' | transloco }}</label>
              <textarea
                id="import-content"
                rows="6"
                [value]="file()"
                (input)="file.set($any($event.target).value)"
              ></textarea>
            </div>
            <div class="field">
              <button type="submit" [disabled]="busy()">{{ 'haushalt.sources.import' | transloco }}</button>
            </div>
          </form>
        }
      </section>
    </app-page>
  `,
})
export class BudgetSourcesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly sources = signal<Source[]>([]);
  readonly centers = signal<Center[]>([]);
  readonly positions = signal<BudgetPositionResponse[]>([]);
  /**
   * One page of Haushaltsstellen, sent explicitly rather than left to the server's default: the
   * picker and the table below it both read this list, so a page size that changed under the screen
   * would silently hide positions from both.
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
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
        this.loadError.set(this.i18n.translate('haushalt.sources.loadSourcesFailed'));
        this.loading.set(false);
      },
    });
    this.api.listBudgetCenters().subscribe({
      next: (centers) => this.centers.set(centers),
      error: () => this.loadError.set(this.i18n.translate('haushalt.sources.loadCentresFailed')),
    });
    this.api.listBudgetPositions(String(this.pageSize), '0').subscribe({
      next: (positions) => {
        const page = positions ?? [];
        this.positions.set(page);
        // A full page means there may be more; a short one is the end of the budget.
        this.hasMore.set(page.length === this.pageSize);
      },
      error: () => {
        this.hasMore.set(false);
        this.loadError.set(this.i18n.translate('haushalt.sources.loadPositionsFailed'));
      },
    });
  }

  /**
   * Reads the next page of Haushaltsstellen and appends it to both the picker and the table, since
   * they are the same list: a position must not be assignable without being shown as assigned. A
   * failed page leaves what is already offered where it is.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api
      .listBudgetPositions(String(this.pageSize), String(this.positions().length))
      .subscribe({
        next: (positions) => {
          const page = positions ?? [];
          this.positions.set([...this.positions(), ...page]);
          this.hasMore.set(page.length === this.pageSize);
          this.loadingMore.set(false);
        },
        error: () => {
          this.loadingMore.set(false);
          this.actionError.set(this.i18n.translate('haushalt.sources.loadMoreFailed'));
        },
      });
  }

  createSource(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    if (this.sourceCode().trim() === '' || this.sourceName().trim() === '') {
      this.actionError.set(this.i18n.translate('haushalt.sources.sourceRequired'));
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
          this.notice.set(this.i18n.translate('haushalt.sources.sourceCreated', { code: saved.Code }));
          this.busy.set(false);
        },
        error: () => {
          this.actionError.set(this.i18n.translate('haushalt.sources.sourceCreateFailed'));
          this.busy.set(false);
        },
      });
  }

  createCenter(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    if (this.centerCode().trim() === '' || this.centerName().trim() === '') {
      this.actionError.set(this.i18n.translate('haushalt.sources.centreRequired'));
      return;
    }
    this.busy.set(true);
    this.api.createBudgetCenter({ code: this.centerCode(), name: this.centerName() }).subscribe({
      next: (saved) => {
        this.centers.update((list) => [...list, saved].sort((a, b) => a.Code.localeCompare(b.Code)));
        this.centerCode.set('');
        this.centerName.set('');
        this.notice.set(this.i18n.translate('haushalt.sources.centreCreated', { code: saved.Code }));
        this.busy.set(false);
      },
      error: () => {
        this.actionError.set(this.i18n.translate('haushalt.sources.centreCreateFailed'));
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
      this.actionError.set(this.i18n.translate('haushalt.sources.noPosition'));
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
        this.notice.set(this.i18n.translate('haushalt.sources.assignSaved'));
        this.busy.set(false);
      },
      error: () => {
        this.actionError.set(this.i18n.translate('haushalt.sources.assignFailed'));
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
      this.actionError.set(this.i18n.translate('haushalt.sources.noFile'));
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
        this.actionError.set(this.i18n.translate('haushalt.sources.importFailed'));
        this.busy.set(false);
      },
    });
  }

  /** What an import did, in prose: created and already-there read differently. */
  importNotice(res: ImportResult): string {
    return this.i18n.translate('haushalt.sources.importNotice', {
      sourcesCreated: res.SourcesCreated,
      centresCreated: res.CentersCreated,
      positionsCreated: res.PositionsCreated,
      sourcesKept: res.SourcesKept,
      positionsKept: res.PositionsKept,
      positionsNamed: res.PositionsNamed,
    });
  }

  /** The name of a position's source, or a plain "keine" when it names none. */
  sourceNameOf(p: BudgetPositionResponse): string {
    if (p.position.SourceID === null) {
      return this.i18n.translate('haushalt.sources.none');
    }
    const found = this.sources().find((s) => s.ID === p.position.SourceID);
    return found === undefined ? String(p.position.SourceID) : `${found.Code} — ${found.Name}`;
  }

  /** The name of a position's centre, or a plain "keine" when it names none. */
  centerNameOf(p: BudgetPositionResponse): string {
    if (p.position.CenterID === null) {
      return this.i18n.translate('haushalt.sources.none');
    }
    const found = this.centers().find((c) => c.ID === p.position.CenterID);
    return found === undefined ? String(p.position.CenterID) : `${found.Code} — ${found.Name}`;
  }

  amount(cents: number): string {
    return formatCents(cents);
  }
}
