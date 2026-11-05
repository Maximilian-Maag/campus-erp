import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, CostCenter, CostType, ServiceRate } from '../core/api/generated';
import { euroToCents, formatCents, formatDay } from '../core/format';

/**
 * KLR — Verrechnungspreise (interne Leistungsverrechnung).
 *
 * Where the Umlage and the Kostenträgerrechnung move *actual* costs, this prices a service and
 * charges it out: a rate per unit of a cost type's service, and a charge that prices a quantity at
 * the rate in force on the day the service was rendered. Both activities are audits of the books,
 * so the screen reports a refusal as it comes back rather than re-deriving the rules.
 */
@Component({
  selector: 'app-klr-service-rates',
  template: `
    <section aria-labelledby="klr-rates-heading">
      <h2 id="klr-rates-heading">Verrechnungspreise</h2>

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
        <p role="status">Verrechnungspreise werden geladen.</p>
      } @else if (!loadError()) {
        @if (rates().length === 0) {
          <p role="status" data-testid="no-rates">Es ist noch kein Verrechnungspreis hinterlegt.</p>
        } @else {
          <h3 id="klr-rate-list-heading">Hinterlegte Preise</h3>
          <table aria-labelledby="klr-rate-list-heading">
            <caption>
              Preis je Einheit einer Kostenart
            </caption>
            <thead>
              <tr>
                <th scope="col">Kostenart</th>
                <th scope="col" class="amount">Preis</th>
                <th scope="col">Gültig ab</th>
                <th scope="col">Gültig bis</th>
              </tr>
            </thead>
            <tbody>
              @for (rate of rates(); track rate.ID) {
                <tr [attr.data-testid]="'rate-' + rate.ID">
                  <td [attr.data-testid]="'rate-cost-type-' + rate.ID">{{ costTypeLabel(rate.CostTypeID) }}</td>
                  <td class="amount" [attr.data-testid]="'rate-amount-' + rate.ID">{{ euro(rate.RateCents) }}</td>
                  <td>{{ day(rate.ValidFrom) }}</td>
                  <td>{{ rate.ValidTo ? day(rate.ValidTo) : 'unbegrenzt' }}</td>
                </tr>
              }
            </tbody>
          </table>
        }

        @if (costTypes().length === 0) {
          <p role="status">Es sind keine Kostenarten angelegt.</p>
        } @else {
          <form (submit)="defineRate($event)">
            <h3 id="klr-rate-define-heading">Preis hinterlegen</h3>
            <p>
              <label for="klr-rate-cost-type">Kostenart</label>
              <select
                id="klr-rate-cost-type"
                [value]="rateCostType()"
                (change)="rateCostType.set($any($event.target).value)"
              >
                @for (type of costTypes(); track type.ID) {
                  <option [value]="type.ID">{{ type.Code }} {{ type.Name }}</option>
                }
              </select>
            </p>
            <p>
              <label for="klr-rate-amount">Preis je Einheit in Euro</label>
              <input
                id="klr-rate-amount"
                type="text"
                inputmode="decimal"
                [value]="rateAmount()"
                (input)="rateAmount.set($any($event.target).value)"
              />
            </p>
            <p>
              <label for="klr-rate-from">Gültig ab</label>
              <input id="klr-rate-from" type="date" [value]="rateFrom()" (input)="rateFrom.set($any($event.target).value)" />
            </p>
            <p>
              <label for="klr-rate-until">Gültig bis</label>
              <input
                id="klr-rate-until"
                type="date"
                [value]="rateUntil()"
                (input)="rateUntil.set($any($event.target).value)"
              />
            </p>
            <button type="submit" data-testid="define-rate" [disabled]="saving()">Hinterlegen</button>
          </form>
        }

        @if (centers().length < 2 || costTypes().length === 0) {
          <p role="status">
            Für eine Verrechnung sind mindestens zwei Kostenstellen und eine Kostenart erforderlich.
          </p>
        } @else {
          <form (submit)="charge($event)">
            <h3 id="klr-charge-heading">Leistung verrechnen</h3>
            <p>
              <label for="klr-charge-provider">Leistende Kostenstelle</label>
              <select
                id="klr-charge-provider"
                [value]="provider()"
                (change)="provider.set($any($event.target).value)"
              >
                @for (center of centers(); track center.ID) {
                  <option [value]="center.ID">{{ center.Number }} {{ center.Name }}</option>
                }
              </select>
            </p>
            <p>
              <label for="klr-charge-receiver">Empfangende Kostenstelle</label>
              <select
                id="klr-charge-receiver"
                [value]="receiver()"
                (change)="receiver.set($any($event.target).value)"
              >
                @for (center of centers(); track center.ID) {
                  <option [value]="center.ID">{{ center.Number }} {{ center.Name }}</option>
                }
              </select>
            </p>
            <p>
              <label for="klr-charge-cost-type">Kostenart</label>
              <select
                id="klr-charge-cost-type"
                [value]="chargeCostType()"
                (change)="chargeCostType.set($any($event.target).value)"
              >
                @for (type of costTypes(); track type.ID) {
                  <option [value]="type.ID">{{ type.Code }} {{ type.Name }}</option>
                }
              </select>
            </p>
            <p>
              <label for="klr-charge-quantity">Menge</label>
              <input
                id="klr-charge-quantity"
                type="text"
                inputmode="numeric"
                [value]="quantity()"
                (input)="quantity.set($any($event.target).value)"
              />
            </p>
            <p>
              <label for="klr-charge-on">Leistungsdatum</label>
              <input id="klr-charge-on" type="date" [value]="chargedOn()" (input)="chargedOn.set($any($event.target).value)" />
            </p>
            <button type="submit" data-testid="charge-service" [disabled]="charging()">Verrechnen</button>
            @if (result(); as r) {
              <p role="status" data-testid="charge-result">
                {{ euro(r.AmountCents) }} für {{ r.Quantity }} Einheiten verrechnet.
              </p>
            }
          </form>
        }
      }
    </section>
  `,
})
export class KlrServiceRatesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly rates = signal<ServiceRate[]>([]);
  readonly costTypes = signal<CostType[]>([]);
  readonly centers = signal<CostCenter[]>([]);
  readonly rateCostType = signal('');
  readonly rateAmount = signal('');
  readonly rateFrom = signal('');
  readonly rateUntil = signal('');
  readonly provider = signal('');
  readonly receiver = signal('');
  readonly chargeCostType = signal('');
  readonly quantity = signal('1');
  readonly chargedOn = signal('');
  readonly loading = signal(true);
  readonly saving = signal(false);
  readonly charging = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  readonly result = signal<{ AmountCents: number; Quantity: number } | null>(null);

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listCostTypes().subscribe({
      next: (types) => {
        const list = types ?? [];
        this.costTypes.set(list);
        // The first cost type is the one the operator most likely means, and seeding it here
        // means neither picker can be submitted empty. A choice already made is left alone.
        const first = list.length > 0 ? String(list[0].ID) : '';
        if (!this.rateCostType()) {
          this.rateCostType.set(first);
        }
        if (!this.chargeCostType()) {
          this.chargeCostType.set(first);
        }
      },
      error: () => {
        this.loadError.set('Konnte die Kostenarten nicht laden.');
        this.loading.set(false);
      },
    });
    this.api.listCostCenters().subscribe({
      next: (centers) => {
        const list = centers ?? [];
        this.centers.set(list);
        // Two centres and no more: the one that is not the provider is the receiver. Only seeded
        // when the operator has not chosen, so a reload does not undo their pick.
        if (!this.provider() && list.length > 0) {
          this.provider.set(String(list[0].ID));
        }
        if (!this.receiver() && list.length > 1) {
          this.receiver.set(String(list[1].ID));
        }
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Konnte die Kostenstellen nicht laden.');
        this.loading.set(false);
      },
    });
    this.api.listServiceRates().subscribe({
      next: (rates) => this.rates.set(rates ?? []),
      error: () => this.loadError.set('Konnte die Verrechnungspreise nicht laden.'),
    });
  }

  defineRate(event: Event): void {
    event.preventDefault();
    const cents = euroToCents(this.rateAmount());
    if (cents === null) {
      this.actionError.set('Der Preis muss ein Betrag über 0 sein.');
      return;
    }
    if (!this.rateFrom()) {
      this.actionError.set('Ein Gültig-ab-Datum ist erforderlich.');
      return;
    }
    this.saving.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api
      .defineServiceRate({
        cost_type_id: Number(this.rateCostType()),
        rate_cents: cents,
        valid_from: this.rateFrom(),
        valid_to: this.rateUntil(),
      })
      .subscribe({
        next: () => {
          this.saving.set(false);
          this.notice.set('Verrechnungspreis hinterlegt.');
          this.rateAmount.set('');
          this.api.listServiceRates().subscribe({
            next: (rates) => this.rates.set(rates ?? []),
            error: () => this.loadError.set('Konnte die Verrechnungspreise nicht laden.'),
          });
        },
        error: () => {
          this.saving.set(false);
          this.actionError.set('Konnte den Verrechnungspreis nicht hinterlegen.');
        },
      });
  }

  charge(event: Event): void {
    event.preventDefault();
    // Number() already ignores surrounding whitespace, so there is no trim here to do it twice.
    const quantity = Number(this.quantity());
    if (!Number.isInteger(quantity) || quantity <= 0) {
      this.actionError.set('Die Menge muss eine ganze Zahl über 0 sein.');
      return;
    }
    if (!this.chargedOn()) {
      this.actionError.set('Ein Leistungsdatum ist erforderlich.');
      return;
    }
    this.charging.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.result.set(null);
    this.api
      .chargeService({
        provider_center_id: Number(this.provider()),
        receiver_center_id: Number(this.receiver()),
        cost_type_id: Number(this.chargeCostType()),
        quantity,
        charged_on: this.chargedOn(),
      })
      .subscribe({
        next: (charge) => {
          this.charging.set(false);
          this.result.set(charge);
        },
        error: () => {
          this.charging.set(false);
          this.actionError.set('Konnte die Leistung nicht verrechnen.');
        },
      });
  }

  /** A cost type as the operator reads it, falling back to the id if it is not loaded. */
  costTypeLabel(id: number): string {
    const type = this.costTypes().find((t) => t.ID === id);
    return type ? type.Code + ' ' + type.Name : String(id);
  }

  readonly euro = formatCents;
  readonly day = formatDay;
}
