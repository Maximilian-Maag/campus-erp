import { Component, OnInit, inject, signal } from '@angular/core';
import { Allocation, CampusErpApi, CostCenter } from '../core/api/generated';
import { basisPointsToPercent, formatCents, percentToBasisPoints } from '../core/format';

/**
 * KLR — Umlage (Kostenstellen-Umlage).
 *
 * A cost centre's costs are distributed over its receivers by keys. The screen defines a key,
 * shows the keys of the chosen centre, and runs a period's distribution — the shares are typed
 * as percentages and stored as basis points, and the API owns the rule that they must add up to
 * 100 %.
 */
@Component({
  selector: 'app-klr-allocation',
  template: `
    <section aria-labelledby="klr-heading">
      <h2 id="klr-heading">Umlage</h2>

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
        <p role="status">Kostenstellen werden geladen.</p>
      } @else if (centers().length === 0 && !loadError()) {
        <p role="status">Es sind keine Kostenstellen angelegt.</p>
      } @else if (!loadError()) {
        <div class="picker">
          <label for="klr-source">Kostenstelle</label>
          <select id="klr-source" [value]="source()" (change)="chooseSource($any($event.target).value)">
            @for (center of centers(); track center.ID) {
              <option [value]="center.ID">{{ center.Number }} {{ center.Name }}</option>
            }
          </select>
        </div>

        <h3 id="klr-keys-heading">Verteilerschlüssel</h3>
        @if (keys().length === 0) {
          <p role="status">Für diese Kostenstelle ist kein Schlüssel hinterlegt.</p>
        } @else {
          <table aria-labelledby="klr-keys-heading">
            <caption>
              Schlüssel der gewählten Kostenstelle
            </caption>
            <thead>
              <tr>
                <th scope="col">Empfänger</th>
                <th scope="col" class="amount">Anteil</th>
                <th scope="col">Gültig ab</th>
                <th scope="col">Gültig bis</th>
              </tr>
            </thead>
            <tbody>
              @for (key of keys(); track key.ID) {
                <tr [attr.data-testid]="'key-' + key.ID">
                  <td [attr.data-testid]="'key-target-' + key.ID">{{ centerLabel(key.TargetCenterID) }}</td>
                  <td class="amount" [attr.data-testid]="'key-share-' + key.ID">{{ share(key.ShareBP) }}</td>
                  <td>{{ day(key.ValidFrom) }}</td>
                  <td>{{ key.ValidTo ? day(key.ValidTo) : 'unbegrenzt' }}</td>
                </tr>
              }
            </tbody>
          </table>
        }

        <form (submit)="define($event)">
          <h3 id="klr-define-heading">Schlüssel anlegen</h3>
          <p>
            <label for="klr-target">Empfänger</label>
            <select id="klr-target" [value]="target()" (change)="target.set($any($event.target).value)">
              @for (center of receivers(); track center.ID) {
                <option [value]="center.ID">{{ center.Number }} {{ center.Name }}</option>
              }
            </select>
          </p>
          <p>
            <label for="klr-share">Anteil in Prozent</label>
            <input
              id="klr-share"
              type="text"
              inputmode="decimal"
              [value]="sharePercent()"
              (input)="sharePercent.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="klr-from">Gültig ab</label>
            <input id="klr-from" type="date" [value]="validFrom()" (input)="validFrom.set($any($event.target).value)" />
          </p>
          <p>
            <label for="klr-until">Gültig bis</label>
            <input id="klr-until" type="date" [value]="validUntil()" (input)="validUntil.set($any($event.target).value)" />
          </p>
          <button type="submit" [disabled]="saving()">Anlegen</button>
        </form>

        <form (submit)="run($event)">
          <h3 id="klr-run-heading">Umlage ausführen</h3>
          <p>
            <label for="klr-run-from">Von</label>
            <input id="klr-run-from" type="date" [value]="runFrom()" (input)="runFrom.set($any($event.target).value)" />
          </p>
          <p>
            <label for="klr-run-to">Bis</label>
            <input id="klr-run-to" type="date" [value]="runTo()" (input)="runTo.set($any($event.target).value)" />
          </p>
          <button type="submit" [disabled]="running()">Ausführen</button>
          @if (result(); as r) {
            <p role="status" data-testid="run-result">
              {{ euro(r.AllocatedCents) }} auf {{ r.Receivers }} Empfänger verteilt.
            </p>
          }
        </form>
      }
    </section>
  `,
})
export class KlrAllocationComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

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
    this.api.listAllocations(id).subscribe({
      next: (keys) => this.keys.set(keys ?? []),
      error: () => this.loadError.set('Konnte die Schlüssel nicht laden.'),
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
      .defineAllocation(this.source(), {
        target_center_id: Number(this.target()),
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
      this.actionError.set('Für die Umlage sind Von und Bis erforderlich.');
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
        this.actionError.set('Konnte die Umlage nicht ausführen.');
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
