import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { Allocation, CampusErpApi, CostCenter } from '../core/api/generated';
import { basisPointsToPercent, formatCents, percentToBasisPoints } from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * KLR — Umlage (Kostenstellen-Umlage).
 *
 * A cost centre's costs are distributed over its receivers by keys. The screen defines a key,
 * shows the keys of the chosen centre, and runs a period's distribution — the shares are typed
 * as percentages and stored as basis points, and the API owns the rule that they must add up to
 * 100 %.
 *
 * The copy is message keys rather than text, so the screen names itself in whatever language the
 * clerk picked; the shares and the amounts are still spelled by the shared formatters.
 */
@Component({
  selector: 'app-klr-allocation',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.klrUmlage.title" subtitleKey="page.klrUmlage.subtitle">
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
          <p role="status">{{ 'klr.common.loadingCostCentres' | transloco }}</p>
        } @else if (centers().length === 0 && !loadError()) {
          <p role="status">{{ 'klr.common.noCostCentres' | transloco }}</p>
        } @else if (!loadError()) {
          <div class="picker">
            <label for="klr-source">{{ 'common.costCenter' | transloco }}</label>
            <select id="klr-source" [value]="source()" (change)="chooseSource($any($event.target).value)">
              @for (center of centers(); track center.ID) {
                <option [value]="center.ID" [selected]="'' + center.ID === source()">{{ center.Number }} {{ center.Name }}</option>
              }
            </select>
          </div>

          <h2 id="klr-keys-heading">{{ 'common.distributionKey' | transloco }}</h2>
          @if (keys().length === 0) {
            <p role="status">{{ 'klr.allocation.noKeys' | transloco }}</p>
          } @else {
            <table aria-labelledby="klr-keys-heading">
              <caption>
                {{ 'klr.allocation.keysCaption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'klr.common.receiver' | transloco }}</th>
                  <th scope="col" class="amount">{{ 'klr.common.share' | transloco }}</th>
                  <th scope="col">{{ 'common.validFrom' | transloco }}</th>
                  <th scope="col">{{ 'common.validUntil' | transloco }}</th>
                </tr>
              </thead>
              <tbody>
                @for (key of keys(); track key.ID) {
                  <tr [attr.data-testid]="'key-' + key.ID">
                    <td [attr.data-testid]="'key-target-' + key.ID">{{ centerLabel(key.TargetCenterID) }}</td>
                    <td class="amount" [attr.data-testid]="'key-share-' + key.ID">{{ share(key.ShareBP) }}</td>
                    <td>{{ day(key.ValidFrom) }}</td>
                    <td>{{ key.ValidTo ? day(key.ValidTo) : ('klr.common.unbounded' | transloco) }}</td>
                  </tr>
                }
              </tbody>
            </table>
          }

          <form (submit)="define($event)">
            <h2 id="klr-define-heading">{{ 'klr.common.createKey' | transloco }}</h2>
            <div class="field">
              <label for="klr-target">{{ 'klr.common.receiver' | transloco }}</label>
              <select id="klr-target" [value]="target()" (change)="target.set($any($event.target).value)">
                @for (center of receivers(); track center.ID) {
                  <option [value]="center.ID" [selected]="'' + center.ID === target()">{{ center.Number }} {{ center.Name }}</option>
                }
              </select>
            </div>
            <div class="field">
              <label for="klr-share">{{ 'klr.common.sharePercent' | transloco }}</label>
              <input
                id="klr-share"
                type="text"
                inputmode="decimal"
                [value]="sharePercent()"
                (input)="sharePercent.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="klr-from">{{ 'common.validFrom' | transloco }}</label>
              <input id="klr-from" type="date" [value]="validFrom()" (input)="validFrom.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="klr-until">{{ 'common.validUntil' | transloco }}</label>
              <input id="klr-until" type="date" [value]="validUntil()" (input)="validUntil.set($any($event.target).value)" />
            </div>
            <div class="field">
              <button type="submit" [disabled]="saving()">{{ 'klr.common.create' | transloco }}</button>
            </div>
          </form>

          <form (submit)="run($event)">
            <h2 id="klr-run-heading">{{ 'klr.allocation.runHeading' | transloco }}</h2>
            <div class="field">
              <label for="klr-run-from">{{ 'common.from' | transloco }}</label>
              <input id="klr-run-from" type="date" [value]="runFrom()" (input)="runFrom.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="klr-run-to">{{ 'common.to' | transloco }}</label>
              <input id="klr-run-to" type="date" [value]="runTo()" (input)="runTo.set($any($event.target).value)" />
            </div>
            <div class="field">
              <button type="submit" [disabled]="running()">{{ 'klr.common.run' | transloco }}</button>
            </div>
            @if (result(); as r) {
              <p role="status" data-testid="run-result">
                {{
                  'klr.allocation.runResult'
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
export class KlrAllocationComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly centers = signal<CostCenter[]>([]);
  readonly keys = signal<Allocation[]>([]);
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
    this.api.listCostCenters().subscribe({
      next: (centers) => {
        const list = centers ?? [];
        this.centers.set(list);
        if (list.length > 0 && !this.source()) {
          // The first centre is the one the operator most likely means to start with, and
          // choosing it here means the keys below are never about an unselected centre.
          this.source.set(String(list[0].ID));
          this.target.set(String(this.receivers()[0]?.ID ?? ''));
          this.loadKeys();
        }
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
    this.api.listAllocations(id).subscribe({
      next: (keys) => this.keys.set(keys ?? []),
      error: () => this.loadError.set(this.i18n.translate('klr.common.loadKeysFailed')),
    });
  }

  /** Every centre but the chosen source can be a receiver. */
  receivers(): CostCenter[] {
    return this.centers().filter((c) => String(c.ID) !== this.source());
  }

  chooseSource(id: string): void {
    this.source.set(id);
    this.result.set(null);
    this.actionError.set(null);
    this.target.set(String(this.receivers()[0]?.ID ?? ''));
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
      .defineAllocation(this.source(), {
        target_center_id: Number(this.target()),
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
      this.actionError.set(this.i18n.translate('klr.allocation.periodRequired'));
      return;
    }
    this.running.set(true);
    this.actionError.set(null);
    this.result.set(null);
    this.api.runAllocation(this.source(), { from: this.runFrom(), to: this.runTo() }).subscribe({
      next: (run) => {
        this.running.set(false);
        this.result.set(run);
      },
      error: () => {
        this.running.set(false);
        this.actionError.set(this.i18n.translate('klr.allocation.runFailed'));
      },
    });
  }

  /** A centre as the operator reads it, falling back to the id if it is not loaded. */
  centerLabel(id: number): string {
    const center = this.centers().find((c) => c.ID === id);
    return center ? center.Number + ' ' + center.Name : String(id);
  }

  readonly share = basisPointsToPercent;
  readonly day = (value: string): string => value.slice(0, 10);
  readonly euro = formatCents;
}
