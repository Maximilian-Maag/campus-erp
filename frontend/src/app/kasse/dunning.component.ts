import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, DunningBand, DunningBandRequest, DunningLevel, DunningNotice } from '../core/api/generated';
import {
  basisPointsToPercent,
  euroToCents,
  formatCents,
  formatDay,
  percentToBasisPoints,
} from '../core/format';

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
 * claim. The charge is the API's to compute from the level's bands and to store as it was
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
  template: `
    <section aria-labelledby="dunning-heading">
      <h2 id="dunning-heading">Mahnwesen</h2>

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
        <p role="status">Mahnstufen werden geladen.</p>
      } @else if (!loadError()) {
        <div class="filter">
          <label for="dunning-year-filter">Jahr</label>
          <input
            id="dunning-year-filter"
            type="number"
            [value]="year()"
            (change)="filterTo($any($event.target).value)"
          />
          <button type="button" (click)="filterTo('')">Alle Jahre</button>
        </div>

        @if (levels().length === 0) {
          <p role="status">Es ist keine Mahnstufe angelegt.</p>
        } @else {
          <table>
            <caption>
              Mahnstufen mit Kürzel, Bezeichnung, Zahlungsfrist, Staffeln und Status
            </caption>
            <thead>
              <tr>
                <th scope="col">Jahr</th>
                <th scope="col">Kürzel</th>
                <th scope="col">Bezeichnung</th>
                <th scope="col">Frist (Tage)</th>
                <th scope="col">Staffeln</th>
                <th scope="col">Status</th>
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
                      <span>keine</span>
                    } @else {
                      <ul class="bands">
                        @for (band of level.Bands; track $index) {
                          <li>{{ bandLabel(band) }}</li>
                        }
                      </ul>
                    }
                  </td>
                  <td>{{ level.Active ? 'aktiv' : 'inaktiv' }}</td>
                </tr>
              }
            </tbody>
          </table>
        }

        <form (submit)="defineLevel($event)">
          <h3 id="dunning-level-heading">Mahnstufe anlegen</h3>
          <p>
            <label for="level-year">Jahr der Mahnstufe</label>
            <input
              id="level-year"
              type="number"
              [value]="levelYear()"
              (input)="levelYear.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="level-code">Kürzel</label>
            <input
              id="level-code"
              type="text"
              [value]="levelCode()"
              (input)="levelCode.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="level-description">Bezeichnung</label>
            <input
              id="level-description"
              type="text"
              [value]="levelDescription()"
              (input)="levelDescription.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="level-days">Frist in Tagen</label>
            <input
              id="level-days"
              type="number"
              [value]="levelDays()"
              (input)="levelDays.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="level-next">Nächste Stufe</label>
            <input
              id="level-next"
              type="text"
              [value]="levelNext()"
              (input)="levelNext.set($any($event.target).value)"
            />
          </p>
          <p>
            <label>
              <input
                type="checkbox"
                [checked]="levelActive()"
                (change)="levelActive.set($any($event.target).checked)"
              />
              aktiv
            </label>
          </p>

          <h4 id="level-bands-heading">Staffeln</h4>
          <p>Beträge in Euro; eine Obergrenze von 0 gilt für jeden Betrag (offen).</p>
          @for (row of bands(); track $index) {
            <fieldset [attr.data-testid]="'band-' + $index">
              <legend>Staffel {{ $index + 1 }}</legend>
              <p>
                <label [attr.for]="'band-upto-' + $index">Obergrenze</label>
                <input
                  [id]="'band-upto-' + $index"
                  type="text"
                  inputmode="decimal"
                  [value]="row.upTo"
                  (input)="setBand($index, 'upTo', $any($event.target).value)"
                />
              </p>
              <p>
                <label [attr.for]="'band-fee-' + $index">Gebühr</label>
                <input
                  [id]="'band-fee-' + $index"
                  type="text"
                  inputmode="decimal"
                  [value]="row.fee"
                  (input)="setBand($index, 'fee', $any($event.target).value)"
                />
              </p>
              <p>
                <label [attr.for]="'band-rate-' + $index">Zinssatz in Prozent</label>
                <input
                  [id]="'band-rate-' + $index"
                  type="text"
                  inputmode="decimal"
                  [value]="row.rate"
                  (input)="setBand($index, 'rate', $any($event.target).value)"
                />
              </p>
              @if (bands().length > 1) {
                <button type="button" (click)="removeBand($index)">Staffel entfernen</button>
              }
            </fieldset>
          }
          <p>
            <button type="button" (click)="addBand()">Staffel hinzufügen</button>
          </p>
          <button type="submit" [disabled]="submitting()">Mahnstufe anlegen</button>
        </form>

        @if (levels().length > 0) {
          <form (submit)="issue($event)">
            <h3 id="dunning-notice-heading">Mahnung ausstellen</h3>
            <p>
              <label for="notice-level">Mahnstufe</label>
              <select
                id="notice-level"
                [value]="noticeLevelKey()"
                (change)="noticeLevelKey.set($any($event.target).value)"
              >
                @for (level of levels(); track level.ID) {
                  <option [value]="levelKey(level)">{{ levelLabel(level) }}</option>
                }
              </select>
            </p>
            <p>
              <label for="notice-claim">Vorgangsnummer</label>
              <input
                id="notice-claim"
                type="text"
                [value]="claimReference()"
                (input)="claimReference.set($any($event.target).value)"
              />
            </p>
            <p>
              <label for="notice-amount">Offener Betrag</label>
              <input
                id="notice-amount"
                type="text"
                inputmode="decimal"
                [value]="outstanding()"
                (input)="outstanding.set($any($event.target).value)"
              />
            </p>
            <button type="submit" [disabled]="submitting()">Mahnung ausstellen</button>
          </form>
        }

        @if (issued(); as n) {
          <section aria-labelledby="dunning-issued-heading">
            <h3 id="dunning-issued-heading">Ausgestellte Mahnung</h3>
            <p role="status" [attr.data-testid]="'notice-' + n.ID">
              Mahnung {{ n.ID }} zu {{ n.ClaimReference }} (Stufe {{ n.LevelCode }}): Gebühr und
              Zinsen {{ euro(n.ChargeCents) }}, ausgestellt am {{ day(n.IssuedOn) }}
            </p>
            @if (n.DocumentKey) {
              <a [href]="documentUrl(n.ID)">Mahnung als PDF</a>
            } @else {
              <p role="status">Zu dieser Mahnung wurde kein Dokument erzeugt.</p>
            }
          </section>
        }
      }
    </section>
  `,
})
export class DunningComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

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
  readonly claimReference = signal('');
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
        this.loadError.set('Konnte die Mahnstufen nicht laden.');
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
      this.actionError.set('Ein Kürzel ist erforderlich.');
      return;
    }
    // Number ignores surrounding whitespace, so a padded year needs no trim of its own.
    const year = Number(this.levelYear());
    if (!Number.isInteger(year) || year <= 0) {
      this.actionError.set('Ein Jahr ist erforderlich.');
      return;
    }
    // A blank period is no period (Number("") is 0); anything else has to be a whole number of
    // days, or the screen would send a NaN the API can only reject.
    const days = Number(this.levelDays());
    if (!Number.isInteger(days) || days < 0) {
      this.actionError.set('Die Frist muss eine ganze Zahl von Tagen sein.');
      return;
    }
    const parsed: DunningBandRequest[] = [];
    for (const row of this.bands()) {
      const upTo = amountOrZero(row.upTo);
      const fee = amountOrZero(row.fee);
      const rate = rateOrZero(row.rate);
      if (upTo === null || fee === null || rate === null) {
        this.actionError.set('Jede Staffel braucht Beträge und einen Zinssatz zwischen 0 und 100 %.');
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
          this.notice.set('Mahnstufe angelegt.');
          this.levelCode.set('');
          this.levelDescription.set('');
          this.bands.set([{ upTo: '', fee: '', rate: '' }]);
          // The list is the server's; reload so the new level (and any band the API stored
          // differently) is shown as stored rather than as it was typed.
          this.load();
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set('Konnte die Mahnstufe nicht anlegen.');
        },
      });
  }

  /** Issues a notice for the chosen level and claim; the charge comes back from the API. */
  issue(event: Event): void {
    event.preventDefault();
    const { year, code } = this.parseLevelKey(this.noticeLevelKey());
    if (year === 0 || code === '') {
      this.actionError.set('Eine Mahnstufe ist erforderlich.');
      return;
    }
    const claim = this.claimReference().trim();
    if (claim === '') {
      this.actionError.set('Eine Vorgangsnummer ist erforderlich.');
      return;
    }
    const outstanding = euroToCents(this.outstanding());
    if (outstanding === null) {
      this.actionError.set('Ein positiver offener Betrag ist erforderlich.');
      return;
    }

    this.submitting.set(true);
    this.clearMessages();
    this.api
      .issueDunningNotice({ year, level_code: code, claim_reference: claim, outstanding_cents: outstanding })
      .subscribe({
        next: (issued) => {
          this.submitting.set(false);
          this.issued.set(issued);
          this.notice.set('Mahnung ausgestellt.');
          this.claimReference.set('');
          this.outstanding.set('');
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set('Konnte die Mahnung nicht ausstellen.');
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
    const bound = band.UpToCents === 0 ? 'offen' : 'bis ' + this.euro(band.UpToCents);
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
