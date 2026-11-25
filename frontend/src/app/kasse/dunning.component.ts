import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, DunningBand, DunningBandRequest, DunningLevel, DunningNotice } from '../core/api/generated';
import {
  basisPointsToPercent,
  euroToCents,
  formatCents,
  formatDay,
  percentToBasisPoints,
} from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * An amount that may legitimately be zero, as minor units, or null when it is not an amount.
 *
 * `euroToCents` is defined for positive amounts only, because every amount a screen used to take
 * was one. A dunning band is different: an upper bound of 0 means "open-ended" and a fee of 0 is a
 * free reminder, so zero has to parse. It is a thin wrapper rather than a second parser: the
 * parsing itself still goes through the shared rule.
 */
function amountOrZero(input: string): number | null {
  const trimmed = input.trim();
  if (trimmed === '' || /^0+([.,]0+)?$/.test(trimmed)) {
    return 0;
  }
  return euroToCents(trimmed);
}

/** A rate that may be zero, as basis points, or null when it is not a percentage. */
function rateOrZero(input: string): number | null {
  const trimmed = input.trim();
  if (trimmed === '') {
    return 0;
  }
  return percentToBasisPoints(trimmed);
}

/** One band as the form holds it: what the operator typed, not yet parsed. */
interface BandRow {
  upTo: string;
  fee: string;
  rate: string;
}

/**
 * Kasse — Mahnwesen (dunning).
 *
 * Lists a year's dunning levels with their bands, defines one, and issues a notice against a
 * receivable. The charge is the API's to compute from the level's bands and to store as it was
 * computed — the screen states the outstanding amount and reports what came back, rather than
 * re-deriving fee and interest in TypeScript.
 *
 * Two values are typed the way an operator thinks and stored the way the API works: amounts in
 * euros (minor units) and a band's rate in percent (basis points). Both converters live in
 * `core/format`, next to the cents pair they belong with.
 *
 * A notice's PDF is a link to its document route rather than a call through the generated client:
 * the client reads that route as text, and a PDF is not text. The link is shown only when the
 * notice actually carries a document, since a notice issued without a document service has none.
 */
@Component({
  selector: 'app-dunning',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.kasseMahnwesen.title" subtitleKey="page.kasseMahnwesen.subtitle">
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
          <p role="status">{{ 'kasse.dunning.loading' | transloco }}</p>
        } @else if (!loadError()) {
          <div class="filter">
            <label for="dunning-year-filter">{{ 'common.year' | transloco }}</label>
            <input
              id="dunning-year-filter"
              type="number"
              [value]="year()"
              (change)="filterTo($any($event.target).value)"
            />
            <button type="button" (click)="filterTo('')">{{ 'kasse.dunning.allYears' | transloco }}</button>
          </div>

          @if (levels().length === 0) {
            <p role="status">{{ 'kasse.dunning.empty' | transloco }}</p>
          } @else {
            <table>
              <caption>
                {{ 'kasse.dunning.caption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'common.year' | transloco }}</th>
                  <th scope="col">{{ 'kasse.dunning.codeColumn' | transloco }}</th>
                  <th scope="col">{{ 'common.description' | transloco }}</th>
                  <th scope="col">{{ 'kasse.dunning.daysColumn' | transloco }}</th>
                  <th scope="col">{{ 'common.tariff' | transloco }}</th>
                  <th scope="col">{{ 'common.status' | transloco }}</th>
                </tr>
              </thead>
              <tbody>
                @for (level of levels(); track level.ID) {
                  <tr [attr.data-testid]="'level-' + level.Year + '-' + level.Code">
                    <td>{{ level.Year }}</td>
                    <td>{{ level.Code }}</td>
                    <td>{{ level.Description }}</td>
                    <td>{{ level.DaysAllowed }}</td>
                    <td>
                      @if (level.Bands.length === 0) {
                        <span>{{ 'kasse.dunning.bands.empty' | transloco }}</span>
                      } @else {
                        <ul class="bands">
                          @for (band of level.Bands; track $index) {
                            <li>{{ bandLabel(band) }}</li>
                          }
                        </ul>
                      }
                    </td>
                    <td>{{ (level.Active ? 'kasse.dunning.active' : 'kasse.dunning.inactive') | transloco }}</td>
                  </tr>
                }
              </tbody>
            </table>
          }

          <form (submit)="defineLevel($event)">
            <h2 id="dunning-level-heading">{{ 'kasse.dunning.createHeading' | transloco }}</h2>
            <div class="field">
              <label for="level-year">{{ 'kasse.dunning.levelYear' | transloco }}</label>
              <input
                id="level-year"
                type="number"
                [value]="levelYear()"
                (input)="levelYear.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="level-code">{{ 'kasse.dunning.code' | transloco }}</label>
              <input
                id="level-code"
                type="text"
                [value]="levelCode()"
                (input)="levelCode.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="level-description">{{ 'common.description' | transloco }}</label>
              <input
                id="level-description"
                type="text"
                [value]="levelDescription()"
                (input)="levelDescription.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="level-days">{{ 'kasse.dunning.days' | transloco }}</label>
              <input
                id="level-days"
                type="number"
                [value]="levelDays()"
                (input)="levelDays.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="level-next">{{ 'kasse.dunning.nextLevel' | transloco }}</label>
              <input
                id="level-next"
                type="text"
                [value]="levelNext()"
                (input)="levelNext.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label>
                <input
                  type="checkbox"
                  [checked]="levelActive()"
                  (change)="levelActive.set($any($event.target).checked)"
                />
                {{ 'kasse.dunning.active' | transloco }}
              </label>
            </div>

            <h3 id="level-bands-heading">{{ 'common.tariff' | transloco }}</h3>
            <p>{{ 'kasse.dunning.bands.hint' | transloco }}</p>
            @for (row of bands(); track $index) {
              <fieldset [attr.data-testid]="'band-' + $index">
                <legend>
                  {{ 'kasse.dunning.bands.legend' | transloco: { number: $index + 1 } }}
                </legend>
                <div class="field">
                  <label [attr.for]="'band-upto-' + $index">{{ 'kasse.dunning.bands.upperBound' | transloco }}</label>
                  <input
                    [id]="'band-upto-' + $index"
                    type="text"
                    inputmode="decimal"
                    [value]="row.upTo"
                    (input)="setBand($index, 'upTo', $any($event.target).value)"
                  />
                </div>
                <div class="field">
                  <label [attr.for]="'band-fee-' + $index">{{ 'kasse.dunning.bands.fee' | transloco }}</label>
                  <input
                    [id]="'band-fee-' + $index"
                    type="text"
                    inputmode="decimal"
                    [value]="row.fee"
                    (input)="setBand($index, 'fee', $any($event.target).value)"
                  />
                </div>
                <div class="field">
                  <label [attr.for]="'band-rate-' + $index">{{ 'kasse.dunning.bands.rate' | transloco }}</label>
                  <input
                    [id]="'band-rate-' + $index"
                    type="text"
                    inputmode="decimal"
                    [value]="row.rate"
                    (input)="setBand($index, 'rate', $any($event.target).value)"
                  />
                </div>
                @if (bands().length > 1) {
                  <button type="button" (click)="removeBand($index)">
                    {{ 'kasse.dunning.bands.remove' | transloco }}
                  </button>
                }
              </fieldset>
            }
            <p>
              <button type="button" (click)="addBand()">{{ 'kasse.dunning.bands.add' | transloco }}</button>
            </p>
            <button type="submit" [disabled]="submitting()">{{ 'kasse.dunning.create' | transloco }}</button>
          </form>

          @if (levels().length > 0) {
            <form (submit)="issue($event)">
              <h2 id="dunning-notice-heading">{{ 'kasse.dunning.noticeHeading' | transloco }}</h2>
              <div class="field">
                <label for="notice-level">{{ 'kasse.dunning.level' | transloco }}</label>
                <select
                  id="notice-level"
                  [value]="noticeLevelKey()"
                  (change)="noticeLevelKey.set($any($event.target).value)"
                >
                  @for (level of levels(); track level.ID) {
                    <option [value]="levelKey(level)" [selected]="'' + levelKey(level) === noticeLevelKey()">{{ levelLabel(level) }}</option>
                  }
                </select>
              </div>
              <div class="field">
                <label for="notice-receivable">{{ 'kasse.dunning.receivableReference' | transloco }}</label>
                <input
                  id="notice-receivable"
                  type="text"
                  [value]="receivableReference()"
                  (input)="receivableReference.set($any($event.target).value)"
                />
              </div>
              <div class="field">
                <label for="notice-amount">{{ 'kasse.dunning.outstanding' | transloco }}</label>
                <input
                  id="notice-amount"
                  type="text"
                  inputmode="decimal"
                  [value]="outstanding()"
                  (input)="outstanding.set($any($event.target).value)"
                />
              </div>
              <button type="submit" [disabled]="submitting()">{{ 'kasse.dunning.issueNotice' | transloco }}</button>
            </form>
          }

          @if (issued(); as n) {
            <section aria-labelledby="dunning-issued-heading">
              <h2 id="dunning-issued-heading">{{ 'kasse.dunning.issuedHeading' | transloco }}</h2>
              <p role="status" [attr.data-testid]="'notice-' + n.ID">
                {{
                  'kasse.dunning.issuedText'
                    | transloco
                      : {
                          id: n.ID,
                          receivable: n.ReceivableReference,
                          level: n.LevelCode,
                          charge: euro(n.ChargeCents),
                          day: day(n.IssuedOn),
                        }
                }}
              </p>
              @if (n.DocumentKey) {
                <a [href]="documentUrl(n.ID)">{{ 'kasse.dunning.pdf' | transloco }}</a>
              } @else {
                <p role="status">{{ 'kasse.dunning.noDocument' | transloco }}</p>
              }
            </section>
          }
        }
      </section>
    </app-page>
  `,
})
export class DunningComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly levels = signal<DunningLevel[]>([]);
  readonly year = signal('');
  readonly loading = signal(true);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  // The level form.
  readonly levelYear = signal(String(new Date().getFullYear()));
  readonly levelCode = signal('');
  readonly levelDescription = signal('');
  readonly levelDays = signal('14');
  readonly levelNext = signal('');
  readonly levelActive = signal(true);
  /** The bands as typed, so a half-filled form survives a reload of the list. */
  readonly bands = signal<BandRow[]>([{ upTo: '', fee: '', rate: '' }]);

  // The notice form.
  readonly noticeLevelKey = signal('');
  readonly receivableReference = signal('');
  readonly outstanding = signal('');
  /** True while a form is in flight, so neither can be submitted twice. */
  readonly submitting = signal(false);
  readonly issued = signal<DunningNotice | null>(null);

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listDunningLevels(this.year().trim() || undefined).subscribe({
      next: (levels) => {
        this.levels.set(levels ?? []);
        // A notice can only be issued against a level that is on screen, so the picker follows
        // the list: the first level by default, and nothing when there is none.
        const keys = this.levels().map((l) => this.levelKey(l));
        if (!keys.includes(this.noticeLevelKey())) {
          this.noticeLevelKey.set(keys[0] ?? '');
        }
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('kasse.dunning.loadFailed'));
        this.loading.set(false);
      },
    });
  }

  /** Narrows to one year ("" for every year). The server does the filtering. */
  filterTo(year: string): void {
    this.year.set(year);
    this.load();
  }

  addBand(): void {
    this.bands.set([...this.bands(), { upTo: '', fee: '', rate: '' }]);
  }

  removeBand(index: number): void {
    this.bands.set(this.bands().filter((_, i) => i !== index));
  }

  setBand(index: number, field: keyof BandRow, value: string): void {
    this.bands.set(this.bands().map((row, i) => (i === index ? { ...row, [field]: value } : row)));
  }

  /** Defines the level from the form. Every band is parsed before anything is sent. */
  defineLevel(event: Event): void {
    event.preventDefault();
    const code = this.levelCode().trim();
    if (code === '') {
      this.actionError.set(this.i18n.translate('kasse.dunning.codeRequired'));
      return;
    }
    // Number ignores surrounding whitespace, so a padded year needs no trim of its own.
    const year = Number(this.levelYear());
    if (!Number.isInteger(year) || year <= 0) {
      this.actionError.set(this.i18n.translate('kasse.dunning.yearRequired'));
      return;
    }
    // A blank period is no period (Number("") is 0); anything else has to be a whole number of
    // days, or the screen would send a NaN the API can only reject.
    const days = Number(this.levelDays());
    if (!Number.isInteger(days) || days < 0) {
      this.actionError.set(this.i18n.translate('kasse.dunning.daysInvalid'));
      return;
    }
    const parsed: DunningBandRequest[] = [];
    for (const row of this.bands()) {
      const upTo = amountOrZero(row.upTo);
      const fee = amountOrZero(row.fee);
      const rate = rateOrZero(row.rate);
      if (upTo === null || fee === null || rate === null) {
        this.actionError.set(this.i18n.translate('kasse.dunning.bandsInvalid'));
        return;
      }
      parsed.push({ up_to_cents: upTo, fee_cents: fee, rate_basis_points: rate });
    }

    this.submitting.set(true);
    this.clearMessages();
    this.api
      .createDunningLevel({
        year,
        code,
        description: this.levelDescription().trim(),
        days_allowed: days,
        active: this.levelActive(),
        next_code: this.levelNext().trim(),
        bands: parsed,
      })
      .subscribe({
        next: () => {
          this.submitting.set(false);
          this.notice.set(this.i18n.translate('kasse.dunning.created'));
          this.levelCode.set('');
          this.levelDescription.set('');
          this.bands.set([{ upTo: '', fee: '', rate: '' }]);
          // The list is the server's; reload so the new level (and any band the API stored
          // differently) is shown as stored rather than as it was typed.
          this.load();
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set(this.i18n.translate('kasse.dunning.createFailed'));
        },
      });
  }

  /** Issues a notice for the chosen level and receivable; the charge comes back from the API. */
  issue(event: Event): void {
    event.preventDefault();
    const { year, code } = this.parseLevelKey(this.noticeLevelKey());
    if (year === 0 || code === '') {
      this.actionError.set(this.i18n.translate('kasse.dunning.levelRequired'));
      return;
    }
    const receivable = this.receivableReference().trim();
    if (receivable === '') {
      this.actionError.set(this.i18n.translate('kasse.dunning.receivableRequired'));
      return;
    }
    const outstanding = euroToCents(this.outstanding());
    if (outstanding === null) {
      this.actionError.set(this.i18n.translate('kasse.dunning.outstandingRequired'));
      return;
    }

    this.submitting.set(true);
    this.clearMessages();
    this.api
      .issueDunningNotice({ year, level_code: code, receivable_reference: receivable, outstanding_cents: outstanding })
      .subscribe({
        next: (issued) => {
          this.submitting.set(false);
          this.issued.set(issued);
          this.notice.set(this.i18n.translate('kasse.dunning.issued'));
          this.receivableReference.set('');
          this.outstanding.set('');
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set(this.i18n.translate('kasse.dunning.issueFailed'));
        },
      });
  }

  private clearMessages(): void {
    this.actionError.set(null);
    this.notice.set(null);
    this.issued.set(null);
  }

  /** One option's value: the level's year and code, so the notice carries both. */
  levelKey(level: DunningLevel): string {
    return level.Year + '|' + level.Code;
  }

  private parseLevelKey(key: string): { year: number; code: string } {
    const [year, code = ''] = key.split('|');
    return { year: Number(year), code };
  }

  levelLabel(level: DunningLevel): string {
    return level.Year + ' / ' + level.Code + (level.Description ? ' — ' + level.Description : '');
  }

  /** A band as one line: its bound, its fee and its rate. */
  bandLabel(band: DunningBand): string {
    const bound =
      band.UpToCents === 0
        ? this.i18n.translate('kasse.dunning.bands.open')
        : this.i18n.translate('kasse.dunning.bands.upTo', { amount: this.euro(band.UpToCents) });
    return bound + ': ' + this.euro(band.FeeCents) + ' + ' + this.percent(band.RateBasisPoints);
  }

  /** The document route of a notice, for a link the browser can open as a PDF. */
  documentUrl(id: number): string {
    return '/api/dunning/notices/' + id + '/document';
  }

  readonly euro = formatCents;
  readonly percent = basisPointsToPercent;
  readonly day = formatDay;
}
