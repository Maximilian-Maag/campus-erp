import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, CostCenter, CostUnit, CostUnitKey } from '../core/api/generated';
import { basisPointsToPercent, formatCents, formatDay, percentToBasisPoints } from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * KLR — Kostenträgerrechnung.
 *
 * A cost centre's costs are attributed to its cost units by keys, the same way the Umlage hands
 * them to other cost centres. The screen defines a key, shows the keys of the chosen centre, and
 * runs a period — the shares are typed as percentages and stored as basis points, and the API owns
 * the rule that they must add up to 100 %.
 *
 * The words are message keys, so the screen reads in the clerk's language; the shares, dates and
 * amounts are still spelled by the shared formatters.
 */
@Component({
  selector: 'app-klr-cost-units',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.klrKostentraeger.title" subtitleKey="page.klrKostentraeger.subtitle">
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
          <p role="status">{{ 'klr.costUnits.loading' | transloco }}</p>
        } @else if (centers().length === 0 && !loadError()) {
          <p role="status">{{ 'klr.common.noCostCentres' | transloco }}</p>
        } @else if (!loadError()) {
          <div class="picker">
            <label for="klr-unit-source">{{ 'common.costCenter' | transloco }}</label>
            <select id="klr-unit-source" [value]="source()" (change)="chooseSource($any($event.target).value)">
              @for (center of centers(); track center.ID) {
                <option [value]="center.ID" [selected]="'' + center.ID === source()">{{ center.Number }} {{ center.Name }}</option>
              }
            </select>
          </div>

          <h2 id="klr-unit-keys-heading">{{ 'common.distributionKey' | transloco }}</h2>
          @if (keys().length === 0) {
            <p role="status" data-testid="no-unit-keys">
              {{ 'klr.costUnits.noKeys' | transloco }}
            </p>
          } @else {
            <table aria-labelledby="klr-unit-keys-heading">
              <caption>
                {{ 'klr.costUnits.keysCaption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'klr.common.costUnit' | transloco }}</th>
                  <th scope="col" class="amount">{{ 'klr.common.share' | transloco }}</th>
                  <th scope="col">{{ 'common.validFrom' | transloco }}</th>
                  <th scope="col">{{ 'common.validUntil' | transloco }}</th>
                </tr>
              </thead>
              <tbody>
                @for (key of keys(); track key.ID) {
                  <tr [attr.data-testid]="'unit-key-' + key.ID">
                    <td [attr.data-testid]="'unit-key-target-' + key.ID">{{ unitLabel(key.TargetUnitID) }}</td>
                    <td class="amount" [attr.data-testid]="'unit-key-share-' + key.ID">{{ share(key.ShareBP) }}</td>
                    <td>{{ day(key.ValidFrom) }}</td>
                    <td>{{ key.ValidTo ? day(key.ValidTo) : ('klr.common.unbounded' | transloco) }}</td>
                  </tr>
                }
              </tbody>
            </table>
          }

          @if (units().length === 0) {
            <p role="status">{{ 'klr.common.noCostUnits' | transloco }}</p>
          } @else {
            <form (submit)="define($event)">
              <h2 id="klr-unit-define-heading">{{ 'klr.common.createKey' | transloco }}</h2>
              <div class="field">
                <label for="klr-unit-target">{{ 'klr.common.costUnit' | transloco }}</label>
                <select id="klr-unit-target" [value]="target()" (change)="target.set($any($event.target).value)">
                  @for (unit of units(); track unit.ID) {
                    <option [value]="unit.ID" [selected]="'' + unit.ID === target()">{{ unit.Number }} {{ unit.Name }}</option>
                  }
                </select>
              </div>
              <div class="field">
                <label for="klr-unit-share">{{ 'klr.common.sharePercent' | transloco }}</label>
                <input
                  id="klr-unit-share"
                  type="text"
                  inputmode="decimal"
                  [value]="sharePercent()"
                  (input)="sharePercent.set($any($event.target).value)"
                />
              </div>
              <div class="field">
                <label for="klr-unit-from">{{ 'common.validFrom' | transloco }}</label>
                <input id="klr-unit-from" type="date" [value]="validFrom()" (input)="validFrom.set($any($event.target).value)" />
              </div>
              <div class="field">
                <label for="klr-unit-until">{{ 'common.validUntil' | transloco }}</label>
                <input id="klr-unit-until" type="date" [value]="validUntil()" (input)="validUntil.set($any($event.target).value)" />
              </div>
              <div class="field">
                <button type="submit" data-testid="define-unit-key" [disabled]="saving()">
                  {{ 'klr.common.create' | transloco }}
                </button>
              </div>
            </form>
          }

          <form (submit)="run($event)">
            <h2 id="klr-unit-run-heading">{{ 'klr.costUnits.runHeading' | transloco }}</h2>
            <div class="field">
              <label for="klr-unit-run-from">{{ 'common.from' | transloco }}</label>
              <input id="klr-unit-run-from" type="date" [value]="runFrom()" (input)="runFrom.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="klr-unit-run-to">{{ 'common.to' | transloco }}</label>
              <input id="klr-unit-run-to" type="date" [value]="runTo()" (input)="runTo.set($any($event.target).value)" />
            </div>
            <div class="field">
              <button type="submit" data-testid="run-unit-allocation" [disabled]="running()">
                {{ 'klr.common.run' | transloco }}
              </button>
            </div>
            @if (result(); as r) {
              <p role="status" data-testid="unit-run-result">
                {{
                  'klr.costUnits.runResult'
                    | transloco: { amount: euro(r.AllocatedCents), receivers: r.Receivers }
                }}
              </p>
            }
          </form>
        }
      </section>
    </app-page>
  `,
})
export class KlrCostUnitsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly centers = signal<CostCenter[]>([]);
  readonly units = signal<CostUnit[]>([]);
  readonly keys = signal<CostUnitKey[]>([]);
  readonly source = signal('');
  readonly target = signal('');
  readonly sharePercent = signal('25');
  readonly validFrom = signal('');
  readonly validUntil = signal('');
  readonly runFrom = signal('');
  readonly runTo = signal('');
  readonly loading = signal(true);
  readonly saving = signal(false);
  readonly running = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  readonly result = signal<{ AllocatedCents: number; Receivers: number } | null>(null);

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listCostUnits().subscribe({
      next: (units) => this.units.set(units ?? []),
      error: () => {
        this.loadError.set(this.i18n.translate('klr.common.loadCostUnitsFailed'));
        this.loading.set(false);
      },
    });
    this.api.listCostCenters().subscribe({
      next: (centers) => {
        const list = centers ?? [];
        this.centers.set(list);
        // A centre chosen before a reload stays chosen; a first load takes the first one.
        const stillThere = list.some((c) => String(c.ID) === this.source());
        if (!stillThere) {
          this.source.set(list.length > 0 ? String(list[0].ID) : '');
          this.target.set(String(this.units()[0]?.ID ?? ''));
        }
        // The keys follow the selection, on a first load and on a reload alike: with nothing
        // chosen the list is empty, which is what loadKeys already does.
        this.loadKeys();
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('klr.common.loadCostCentresFailed'));
        this.loading.set(false);
      },
    });
  }

  loadKeys(): void {
    const id = this.source();
    if (!id) {
      this.keys.set([]);
      return;
    }
    this.api.listCostUnitKeys(id).subscribe({
      next: (keys) => this.keys.set(keys ?? []),
      error: () => this.loadError.set(this.i18n.translate('klr.common.loadKeysFailed')),
    });
  }

  chooseSource(id: string): void {
    this.source.set(id);
    this.result.set(null);
    this.actionError.set(null);
    this.target.set(String(this.units()[0]?.ID ?? ''));
    this.loadKeys();
  }

  define(event: Event): void {
    event.preventDefault();
    const shareBP = percentToBasisPoints(this.sharePercent());
    if (shareBP === null) {
      this.actionError.set(this.i18n.translate('klr.common.shareInvalid'));
      return;
    }
    if (!this.validFrom()) {
      this.actionError.set(this.i18n.translate('klr.common.validFromRequired'));
      return;
    }
    this.saving.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api
      .defineCostUnitKey(this.source(), {
        target_unit_id: Number(this.target()),
        share_bp: shareBP,
        valid_from: this.validFrom(),
        valid_to: this.validUntil(),
      })
      .subscribe({
        next: () => {
          this.saving.set(false);
          this.notice.set(this.i18n.translate('klr.common.keyCreated'));
          this.loadKeys();
        },
        error: () => {
          this.saving.set(false);
          this.actionError.set(this.i18n.translate('klr.common.keyCreateFailed'));
        },
      });
  }

  run(event: Event): void {
    event.preventDefault();
    if (!this.runFrom() || !this.runTo()) {
      this.actionError.set(this.i18n.translate('klr.costUnits.periodRequired'));
      return;
    }
    this.running.set(true);
    this.actionError.set(null);
    this.result.set(null);
    this.api.runCostUnitAllocation(this.source(), { from: this.runFrom(), to: this.runTo() }).subscribe({
      next: (run) => {
        this.running.set(false);
        this.result.set(run);
      },
      error: () => {
        this.running.set(false);
        this.actionError.set(this.i18n.translate('klr.costUnits.runFailed'));
      },
    });
  }

  /** A cost unit as the operator reads it, falling back to the id if it is not loaded. */
  unitLabel(id: number): string {
    const unit = this.units().find((u) => u.ID === id);
    return unit ? unit.Number + ' ' + unit.Name : String(id);
  }

  readonly share = basisPointsToPercent;
  readonly day = formatDay;
  readonly euro = formatCents;
}
