import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, CostCenter, CostUnit, CostUnitKey } from '../core/api/generated';
import { basisPointsToPercent, formatCents, formatDay, percentToBasisPoints } from '../core/format';

/**
 * KLR — Kostenträgerrechnung.
 *
 * A cost centre's costs are attributed to its cost units by keys, the same way the Umlage hands
 * them to other cost centres. The screen defines a key, shows the keys of the chosen centre, and
 * runs a period — the shares are typed as percentages and stored as basis points, and the API owns
 * the rule that they must add up to 100 %.
 */
@Component({
  selector: 'app-klr-cost-units',
  template: `
    <section aria-labelledby="klr-units-heading">
      <h2 id="klr-units-heading">Kostenträgerrechnung</h2>

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
        <p role="status">Kostenstellen und Kostenträger werden geladen.</p>
      } @else if (centers().length === 0 && !loadError()) {
        <p role="status">Es sind keine Kostenstellen angelegt.</p>
      } @else if (!loadError()) {
        <div class="picker">
          <label for="klr-unit-source">Kostenstelle</label>
          <select id="klr-unit-source" [value]="source()" (change)="chooseSource($any($event.target).value)">
            @for (center of centers(); track center.ID) {
              <option [value]="center.ID">{{ center.Number }} {{ center.Name }}</option>
            }
          </select>
        </div>

        <h3 id="klr-unit-keys-heading">Verteilerschlüssel</h3>
        @if (keys().length === 0) {
          <p role="status" data-testid="no-unit-keys">
            Für diese Kostenstelle ist kein Kostenträger-Schlüssel hinterlegt.
          </p>
        } @else {
          <table aria-labelledby="klr-unit-keys-heading">
            <caption>
              Kostenträger der gewählten Kostenstelle
            </caption>
            <thead>
              <tr>
                <th scope="col">Kostenträger</th>
                <th scope="col" class="amount">Anteil</th>
                <th scope="col">Gültig ab</th>
                <th scope="col">Gültig bis</th>
              </tr>
            </thead>
            <tbody>
              @for (key of keys(); track key.ID) {
                <tr [attr.data-testid]="'unit-key-' + key.ID">
                  <td [attr.data-testid]="'unit-key-target-' + key.ID">{{ unitLabel(key.TargetUnitID) }}</td>
                  <td class="amount" [attr.data-testid]="'unit-key-share-' + key.ID">{{ share(key.ShareBP) }}</td>
                  <td>{{ day(key.ValidFrom) }}</td>
                  <td>{{ key.ValidTo ? day(key.ValidTo) : 'unbegrenzt' }}</td>
                </tr>
              }
            </tbody>
          </table>
        }

        @if (units().length === 0) {
          <p role="status">Es sind keine Kostenträger angelegt.</p>
        } @else {
          <form (submit)="define($event)">
            <h3 id="klr-unit-define-heading">Schlüssel anlegen</h3>
            <p>
              <label for="klr-unit-target">Kostenträger</label>
              <select id="klr-unit-target" [value]="target()" (change)="target.set($any($event.target).value)">
                @for (unit of units(); track unit.ID) {
                  <option [value]="unit.ID">{{ unit.Number }} {{ unit.Name }}</option>
                }
              </select>
            </p>
            <p>
              <label for="klr-unit-share">Anteil in Prozent</label>
              <input
                id="klr-unit-share"
                type="text"
                inputmode="decimal"
                [value]="sharePercent()"
                (input)="sharePercent.set($any($event.target).value)"
              />
            </p>
            <p>
              <label for="klr-unit-from">Gültig ab</label>
              <input id="klr-unit-from" type="date" [value]="validFrom()" (input)="validFrom.set($any($event.target).value)" />
            </p>
            <p>
              <label for="klr-unit-until">Gültig bis</label>
              <input id="klr-unit-until" type="date" [value]="validUntil()" (input)="validUntil.set($any($event.target).value)" />
            </p>
            <button type="submit" data-testid="define-unit-key" [disabled]="saving()">Anlegen</button>
          </form>
        }

        <form (submit)="run($event)">
          <h3 id="klr-unit-run-heading">Kostenträgerrechnung ausführen</h3>
          <p>
            <label for="klr-unit-run-from">Von</label>
            <input id="klr-unit-run-from" type="date" [value]="runFrom()" (input)="runFrom.set($any($event.target).value)" />
          </p>
          <p>
            <label for="klr-unit-run-to">Bis</label>
            <input id="klr-unit-run-to" type="date" [value]="runTo()" (input)="runTo.set($any($event.target).value)" />
          </p>
          <button type="submit" data-testid="run-unit-allocation" [disabled]="running()">Ausführen</button>
          @if (result(); as r) {
            <p role="status" data-testid="unit-run-result">
              {{ euro(r.AllocatedCents) }} auf {{ r.Receivers }} Kostenträger verteilt.
            </p>
          }
        </form>
      }
    </section>
  `,
})
export class KlrCostUnitsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

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
        this.loadError.set('Konnte die Kostenträger nicht laden.');
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
        this.loadError.set('Konnte die Kostenstellen nicht laden.');
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
      error: () => this.loadError.set('Konnte die Schlüssel nicht laden.'),
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
      this.actionError.set('Der Anteil muss eine Prozentzahl zwischen 0 und 100 sein.');
      return;
    }
    if (!this.validFrom()) {
      this.actionError.set('Ein Gültig-ab-Datum ist erforderlich.');
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
          this.notice.set('Schlüssel angelegt.');
          this.loadKeys();
        },
        error: () => {
          this.saving.set(false);
          this.actionError.set('Konnte den Schlüssel nicht anlegen.');
        },
      });
  }

  run(event: Event): void {
    event.preventDefault();
    if (!this.runFrom() || !this.runTo()) {
      this.actionError.set('Für die Kostenträgerrechnung sind Von und Bis erforderlich.');
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
        this.actionError.set('Konnte die Kostenträgerrechnung nicht ausführen.');
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
