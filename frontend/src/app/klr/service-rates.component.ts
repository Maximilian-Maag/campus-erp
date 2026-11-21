import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, CostCenter, CostType, ServiceRate } from '../core/api/generated';
import { euroToCents, formatCents, formatDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * KLR — Verrechnungspreise (interne Leistungsverrechnung).
 *
 * Where the Umlage and the Kostenträgerrechnung move *actual* costs, this prices a service and
 * charges it out: a rate per unit of a cost type's service, and a charge that prices a quantity at
 * the rate in force on the day the service was rendered. Both activities are audits of the books,
 * so the screen reports a refusal as it comes back rather than re-deriving the rules.
 *
 * The copy is message keys, so the screen reads in the clerk's language; the API's own refusals are
 * turned into the message that names the rule that was broken.
 */
@Component({
  selector: 'app-klr-service-rates',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.klrVerrechnungspreise.title" subtitleKey="page.klrVerrechnungspreise.subtitle">
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
          <p role="status">{{ 'klr.serviceRates.loading' | transloco }}</p>
        } @else if (!loadError()) {
          @if (rates().length === 0) {
            <p role="status" data-testid="no-rates">{{ 'klr.serviceRates.empty' | transloco }}</p>
          } @else {
            <h2 id="klr-rate-list-heading">{{ 'klr.serviceRates.listHeading' | transloco }}</h2>
            <table aria-labelledby="klr-rate-list-heading">
              <caption>
                {{ 'klr.serviceRates.listCaption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'common.costType' | transloco }}</th>
                  <th scope="col" class="amount">{{ 'klr.serviceRates.priceColumn' | transloco }}</th>
                  <th scope="col">{{ 'common.validFrom' | transloco }}</th>
                  <th scope="col">{{ 'common.validUntil' | transloco }}</th>
                </tr>
              </thead>
              <tbody>
                @for (rate of rates(); track rate.ID) {
                  <tr [attr.data-testid]="'rate-' + rate.ID">
                    <td [attr.data-testid]="'rate-cost-type-' + rate.ID">{{ costTypeLabel(rate.CostTypeID) }}</td>
                    <td class="amount" [attr.data-testid]="'rate-amount-' + rate.ID">{{ euro(rate.RateCents) }}</td>
                    <td>{{ day(rate.ValidFrom) }}</td>
                    <td>{{ rate.ValidTo ? day(rate.ValidTo) : ('klr.common.unbounded' | transloco) }}</td>
                  </tr>
                }
              </tbody>
            </table>
          }

          @if (costTypes().length === 0) {
            <p role="status">{{ 'klr.common.noCostTypes' | transloco }}</p>
          } @else {
            <form (submit)="defineRate($event)">
              <h2 id="klr-rate-define-heading">{{ 'klr.serviceRates.defineHeading' | transloco }}</h2>
              <div class="field">
                <label for="klr-rate-cost-type">{{ 'common.costType' | transloco }}</label>
                <select
                  id="klr-rate-cost-type"
                  [value]="rateCostType()"
                  (change)="rateCostType.set($any($event.target).value)"
                >
                  @for (type of costTypes(); track type.ID) {
                    <option [value]="type.ID">{{ type.Code }} {{ type.Name }}</option>
                  }
                </select>
              </div>
              @if (hasMore()) {
                <p>
                  <button
                    type="button"
                    data-testid="load-more-cost-types"
                    [disabled]="loadingMore()"
                    (click)="loadMore()"
                  >
                    {{
                      (loadingMore() ? 'klr.serviceRates.loadingMoreCostTypes' : 'klr.serviceRates.loadMoreCostTypes')
                        | transloco
                    }}
                  </button>
                </p>
              }
              <div class="field">
                <label for="klr-rate-amount">{{ 'klr.serviceRates.priceLabel' | transloco }}</label>
                <input
                  id="klr-rate-amount"
                  type="text"
                  inputmode="decimal"
                  [value]="rateAmount()"
                  (input)="rateAmount.set($any($event.target).value)"
                />
              </div>
              <div class="field">
                <label for="klr-rate-from">{{ 'common.validFrom' | transloco }}</label>
                <input id="klr-rate-from" type="date" [value]="rateFrom()" (input)="rateFrom.set($any($event.target).value)" />
              </div>
              <div class="field">
                <label for="klr-rate-until">{{ 'common.validUntil' | transloco }}</label>
                <input
                  id="klr-rate-until"
                  type="date"
                  [value]="rateUntil()"
                  (input)="rateUntil.set($any($event.target).value)"
                />
              </div>
              <div class="field">
                <button type="submit" data-testid="define-rate" [disabled]="saving()">
                  {{ 'klr.serviceRates.defineButton' | transloco }}
                </button>
              </div>
            </form>
          }

          @if (centers().length < 2 || costTypes().length === 0) {
            <p role="status">
              {{ 'klr.serviceRates.chargeRequirements' | transloco }}
            </p>
          } @else {
            <form (submit)="charge($event)">
              <h2 id="klr-charge-heading">{{ 'klr.serviceRates.chargeHeading' | transloco }}</h2>
              <div class="field">
                <label for="klr-charge-provider">{{ 'klr.serviceRates.providerCentre' | transloco }}</label>
                <select
                  id="klr-charge-provider"
                  [value]="provider()"
                  (change)="provider.set($any($event.target).value)"
                >
                  @for (center of centers(); track center.ID) {
                    <option [value]="center.ID">{{ center.Number }} {{ center.Name }}</option>
                  }
                </select>
              </div>
              <div class="field">
                <label for="klr-charge-receiver">{{ 'klr.serviceRates.receiverCentre' | transloco }}</label>
                <select
                  id="klr-charge-receiver"
                  [value]="receiver()"
                  (change)="receiver.set($any($event.target).value)"
                >
                  @for (center of centers(); track center.ID) {
                    <option [value]="center.ID">{{ center.Number }} {{ center.Name }}</option>
                  }
                </select>
              </div>
              <div class="field">
                <label for="klr-charge-cost-type">{{ 'common.costType' | transloco }}</label>
                <select
                  id="klr-charge-cost-type"
                  [value]="chargeCostType()"
                  (change)="chargeCostType.set($any($event.target).value)"
                >
                  @for (type of costTypes(); track type.ID) {
                    <option [value]="type.ID">{{ type.Code }} {{ type.Name }}</option>
                  }
                </select>
              </div>
              <div class="field">
                <label for="klr-charge-quantity">{{ 'common.quantity' | transloco }}</label>
                <input
                  id="klr-charge-quantity"
                  type="text"
                  inputmode="numeric"
                  [value]="quantity()"
                  (input)="quantity.set($any($event.target).value)"
                />
              </div>
              <div class="field">
                <label for="klr-charge-on">{{ 'klr.serviceRates.serviceDate' | transloco }}</label>
                <input id="klr-charge-on" type="date" [value]="chargedOn()" (input)="chargedOn.set($any($event.target).value)" />
              </div>
              <div class="field">
                <button type="submit" data-testid="charge-service" [disabled]="charging()">
                  {{ 'klr.serviceRates.chargeButton' | transloco }}
                </button>
              </div>
              @if (result(); as r) {
                <p role="status" data-testid="charge-result">
                  {{
                    'klr.serviceRates.chargeResult'
                      | transloco: { amount: euro(r.AmountCents), quantity: r.Quantity }
                  }}
                </p>
              }
            </form>
          }
        }
      </section>
    </app-page>
  `,
})
export class KlrServiceRatesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

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
  /**
   * One page of cost types, sent explicitly rather than left to the server's default: both pickers
   * below read this list, so a page size that changed under the screen would silently hide cost
   * types from both.
   */
  private readonly pageSize = 50;
  /** True while the last page of cost types came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  readonly result = signal<{ AmountCents: number; Quantity: number } | null>(null);

  ngOnInit(): void {
    this.load();
  }

  /**
   * Reads the next page of cost types and appends it to both pickers, since they are the same list.
   * A failed page leaves what is already offered where it is — those cost types are still priceable.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api.listCostTypes(String(this.pageSize), String(this.costTypes().length)).subscribe({
      next: (types) => {
        const page = types ?? [];
        this.costTypes.set([...this.costTypes(), ...page]);
        this.hasMore.set(page.length === this.pageSize);
        this.loadingMore.set(false);
      },
      error: () => {
        this.loadingMore.set(false);
        this.actionError.set(this.i18n.translate('klr.common.loadMoreCostTypesFailed'));
      },
    });
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listCostTypes(String(this.pageSize), '0').subscribe({
      next: (types) => {
        const list = types ?? [];
        this.costTypes.set(list);
        // A full page means there may be more; a short one is the end of the catalogue.
        this.hasMore.set(list.length === this.pageSize);
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
        this.loadError.set(this.i18n.translate('klr.common.loadCostTypesFailed'));
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
        this.loadError.set(this.i18n.translate('klr.common.loadCostCentresFailed'));
        this.loading.set(false);
      },
    });
    this.api.listServiceRates().subscribe({
      next: (rates) => this.rates.set(rates ?? []),
      error: () => this.loadError.set(this.i18n.translate('klr.serviceRates.loadRatesFailed')),
    });
  }

  defineRate(event: Event): void {
    event.preventDefault();
    const cents = euroToCents(this.rateAmount());
    if (cents === null) {
      this.actionError.set(this.i18n.translate('klr.serviceRates.priceInvalid'));
      return;
    }
    if (!this.rateFrom()) {
      this.actionError.set(this.i18n.translate('klr.common.validFromRequired'));
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
          this.notice.set(this.i18n.translate('klr.serviceRates.rateSaved'));
          this.rateAmount.set('');
          this.api.listServiceRates().subscribe({
            next: (rates) => this.rates.set(rates ?? []),
            error: () => this.loadError.set(this.i18n.translate('klr.serviceRates.loadRatesFailed')),
          });
        },
        error: () => {
          this.saving.set(false);
          this.actionError.set(this.i18n.translate('klr.serviceRates.saveFailed'));
        },
      });
  }

  charge(event: Event): void {
    event.preventDefault();
    // Number() already ignores surrounding whitespace, so there is no trim here to do it twice.
    const quantity = Number(this.quantity());
    if (!Number.isInteger(quantity) || quantity <= 0) {
      this.actionError.set(this.i18n.translate('klr.serviceRates.quantityInvalid'));
      return;
    }
    if (!this.chargedOn()) {
      this.actionError.set(this.i18n.translate('klr.serviceRates.serviceDateRequired'));
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
          this.actionError.set(this.i18n.translate('klr.serviceRates.chargeFailed'));
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
