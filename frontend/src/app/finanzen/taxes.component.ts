import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, TaxOffice, VATType } from '../core/api/generated';
import { basisPointsToPercent, percentToBasisPoints } from '../core/format';
import { VAT_KINDS } from '../core/tax';
import { PageComponent } from '../ui/page.component';

/**
 * Finanzen — Steuersätze und Finanzämter (docs/07).
 *
 * The two catalogues the exact VAT arithmetic is driven from: **VAT types** (a rate and whether it
 * is Umsatzsteuer or Vorsteuer) and the **Finanzämter** the tax is declared to, with the account it
 * is paid to. Both were write-only routes before this screen: a VAT type could be created and read
 * back by code, but no screen could show the catalogue, so the rates lived only in the API.
 *
 * The tax offices are read per year — the filter is the server's (`?year=`), the same rule the
 * foundation references follow — because a Finanzamt is a master record of one year.
 */
@Component({
  selector: 'app-taxes',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.finanzenSteuern.title" subtitleKey="page.finanzenSteuern.subtitle">
      <section>
        @if (vatLoadError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (officeLoadError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (actionError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (notice(); as n) {
          <p role="status" class="notice">{{ n }}</p>
        }

        @if (loading()) {
          <p role="status">{{ 'finanzen.taxes.loading' | transloco }}</p>
        } @else {
          <table>
            <caption>
              {{ 'finanzen.taxes.ratesCaption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.code' | transloco }}</th>
                <th scope="col">{{ 'common.description' | transloco }}</th>
                <th scope="col" class="amount">{{ 'finanzen.taxes.rate' | transloco }}</th>
                <th scope="col">{{ 'common.type' | transloco }}</th>
              </tr>
            </thead>
            <tbody>
              @for (vat of vatTypes(); track vat.ID) {
                <tr [attr.data-testid]="'vat-' + vat.Code">
                  <td>{{ vat.Code }}</td>
                  <td>{{ vat.Description }}</td>
                  <td class="amount">{{ rate(vat.RateBasisPoints) }}</td>
                  <td>{{ kind(vat.Kind) }}</td>
                </tr>
              } @empty {
                <tr>
                  <td colspan="4">{{ 'finanzen.taxes.ratesEmpty' | transloco }}</td>
                </tr>
              }
            </tbody>
          </table>

          <form (submit)="createVAT($event)">
            <h2 id="vat-heading">{{ 'finanzen.taxes.newRateTitle' | transloco }}</h2>
            <div class="field">
              <label for="vat-code">{{ 'common.code' | transloco }}</label>
              <input id="vat-code" [value]="vatCode()" (input)="vatCode.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="vat-description">{{ 'common.description' | transloco }}</label>
              <input
                id="vat-description"
                [value]="vatDescription()"
                (input)="vatDescription.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="vat-rate">{{ 'finanzen.taxes.ratePercent' | transloco }}</label>
              <input id="vat-rate" [value]="vatRate()" (input)="vatRate.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="vat-kind">{{ 'common.type' | transloco }}</label>
              <select id="vat-kind" [value]="vatKind()" (change)="vatKind.set($any($event.target).value)">
                @for (k of kinds; track k.value) {
                  <option [value]="k.value">{{ kind(k.value) }}</option>
                }
              </select>
            </div>
            <div class="field">
              <button type="submit" [disabled]="busy()">{{ 'finanzen.taxes.newRate' | transloco }}</button>
            </div>
          </form>

          <p>
            <label for="office-year">{{ 'finanzen.taxes.displayYear' | transloco }}</label>
            <input id="office-year" [value]="year()" (input)="year.set($any($event.target).value)" />
            <button type="button" (click)="loadOffices()">{{ 'finanzen.taxes.show' | transloco }}</button>
          </p>

          <table>
            <caption>
              {{ 'finanzen.taxes.officesCaption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.year' | transloco }}</th>
                <th scope="col">{{ 'common.number' | transloco }}</th>
                <th scope="col">{{ 'common.name' | transloco }}</th>
                <th scope="col">{{ 'finanzen.taxes.city' | transloco }}</th>
                <th scope="col">{{ 'common.iban' | transloco }}</th>
              </tr>
            </thead>
            <tbody>
              @for (office of offices(); track office.ID) {
                <tr [attr.data-testid]="'office-' + office.ID">
                  <td>{{ office.Year }}</td>
                  <td>{{ office.Number }}</td>
                  <td>{{ office.Name }}</td>
                  <td>{{ office.City }}</td>
                  <td>{{ office.IBAN || '—' }}</td>
                </tr>
              } @empty {
                <tr>
                  <td colspan="5">{{ 'finanzen.taxes.officesEmpty' | transloco }}</td>
                </tr>
              }
            </tbody>
          </table>

          <form (submit)="createOffice($event)">
            <h2 id="office-heading">{{ 'finanzen.taxes.newOfficeTitle' | transloco }}</h2>
            <div class="field">
              <label for="office-new-year">{{ 'common.year' | transloco }}</label>
              <input id="office-new-year" [value]="officeYear()" (input)="officeYear.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="office-number">{{ 'common.number' | transloco }}</label>
              <input id="office-number" [value]="officeNumber()" (input)="officeNumber.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="office-name">{{ 'common.name' | transloco }}</label>
              <input id="office-name" [value]="officeName()" (input)="officeName.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="office-street">{{ 'finanzen.taxes.street' | transloco }}</label>
              <input id="office-street" [value]="officeStreet()" (input)="officeStreet.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="office-postal">{{ 'finanzen.taxes.postalCode' | transloco }}</label>
              <input id="office-postal" [value]="officePostal()" (input)="officePostal.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="office-city">{{ 'finanzen.taxes.city' | transloco }}</label>
              <input id="office-city" [value]="officeCity()" (input)="officeCity.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="office-holder">{{ 'finanzen.taxes.accountHolder' | transloco }}</label>
              <input id="office-holder" [value]="officeHolder()" (input)="officeHolder.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="office-iban">{{ 'common.iban' | transloco }}</label>
              <input id="office-iban" [value]="officeIBAN()" (input)="officeIBAN.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="office-bic">{{ 'common.bic' | transloco }}</label>
              <input id="office-bic" [value]="officeBIC()" (input)="officeBIC.set($any($event.target).value)" />
            </div>
            <div class="field">
              <button type="submit" [disabled]="busy()">{{ 'finanzen.taxes.newOffice' | transloco }}</button>
            </div>
          </form>
        }
      </section>
    </app-page>
  `,
})
export class TaxesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly vatTypes = signal<VATType[]>([]);
  readonly offices = signal<TaxOffice[]>([]);
  readonly loading = signal(false);
  readonly busy = signal(false);
  /**
   * A read's failure is kept apart per catalogue: two failures must both be visible, because an
   * operator needs to know which of the two is unreadable — and the readable one still shows.
   */
  readonly vatLoadError = signal<string | null>(null);
  readonly officeLoadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  /** The year whose Finanzämter are shown; "" lists every year. */
  readonly year = signal('');

  readonly vatCode = signal('');
  readonly vatDescription = signal('');
  readonly vatRate = signal('');
  readonly vatKind = signal('output');

  readonly officeYear = signal('');
  readonly officeNumber = signal('');
  readonly officeName = signal('');
  readonly officeStreet = signal('');
  readonly officePostal = signal('');
  readonly officeCity = signal('');
  readonly officeHolder = signal('');
  readonly officeIBAN = signal('');
  readonly officeBIC = signal('');

  readonly kinds = VAT_KINDS;

  ngOnInit(): void {
    this.loadVATTypes();
    this.loadOffices();
  }

  /** The catalogue is small and complete in one read: it is not filtered by anything. */
  loadVATTypes(): void {
    this.loading.set(true);
    this.vatLoadError.set(null);
    this.api.listVATTypes().subscribe({
      next: (types) => {
        this.vatTypes.set(types ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.vatLoadError.set(this.i18n.translate('finanzen.taxes.ratesLoadFailed'));
        this.loading.set(false);
      },
    });
  }

  /** Reads the Finanzämter of the stated year — the server narrows, the screen does not. */
  loadOffices(): void {
    this.officeLoadError.set(null);
    this.api.listTaxOffices(this.year()).subscribe({
      next: (offices) => this.offices.set(offices ?? []),
      error: () => this.officeLoadError.set(this.i18n.translate('finanzen.taxes.officesLoadFailed')),
    });
  }

  createVAT(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    if (this.vatCode().trim() === '' || this.vatDescription().trim() === '') {
      this.actionError.set(this.i18n.translate('finanzen.taxes.rateKeyRequired'));
      return;
    }
    const rate = percentToBasisPoints(this.vatRate());
    if (rate === null) {
      this.actionError.set(this.i18n.translate('finanzen.taxes.rateInvalid'));
      return;
    }
    this.busy.set(true);
    this.api
      .createVATType({
        code: this.vatCode(),
        description: this.vatDescription(),
        rate_basis_points: rate,
        kind: this.vatKind(),
      })
      .subscribe({
        next: (saved) => {
          this.vatTypes.update((list) => [...list, saved].sort((a, b) => a.Code.localeCompare(b.Code)));
          this.vatCode.set('');
          this.vatDescription.set('');
          this.vatRate.set('');
          this.notice.set(this.i18n.translate('finanzen.taxes.rateCreated', { code: saved.Code }));
          this.busy.set(false);
        },
        error: () => {
          this.actionError.set(this.i18n.translate('finanzen.taxes.rateCreateFailed'));
          this.busy.set(false);
        },
      });
  }

  /**
   * Records a Finanzamt. The screen refuses what it can see is missing; the IBAN's checksum and the
   * BIC's shape are the server's, so a wrong one comes back as a refusal rather than being caught by
   * a second implementation here.
   */
  createOffice(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    // An empty field reads as zero here, so one test carries the blank case and the negative one:
    // there is no string that is a year and does not pass, which is why the blank is not checked
    // twice.
    const year = Number(this.officeYear());
    if (!Number.isInteger(year) || year <= 0) {
      this.actionError.set(this.i18n.translate('finanzen.taxes.officeYearRequired'));
      return;
    }
    if (this.officeNumber().trim() === '' || this.officeName().trim() === '') {
      this.actionError.set(this.i18n.translate('finanzen.taxes.officeNumberRequired'));
      return;
    }
    this.busy.set(true);
    this.api
      .createTaxOffice({
        year,
        number: this.officeNumber(),
        name: this.officeName(),
        street: this.officeStreet(),
        postal_code: this.officePostal(),
        city: this.officeCity(),
        account_holder: this.officeHolder(),
        iban: this.officeIBAN(),
        bic: this.officeBIC(),
      })
      .subscribe({
        next: (saved) => {
          this.offices.update((list) => [...list, saved].sort((a, b) => a.Number.localeCompare(b.Number)));
          this.officeNumber.set('');
          this.officeName.set('');
          this.officeStreet.set('');
          this.officePostal.set('');
          this.officeCity.set('');
          this.officeHolder.set('');
          this.officeIBAN.set('');
          this.officeBIC.set('');
          this.notice.set(this.i18n.translate('finanzen.taxes.officeCreated', { number: saved.Number }));
          this.busy.set(false);
        },
        error: () => {
          this.actionError.set(this.i18n.translate('finanzen.taxes.officeCreateFailed'));
          this.busy.set(false);
        },
      });
  }

  /** The rate as the operator reads it, from the shared formatter. */
  rate(basisPoints: number): string {
    return basisPointsToPercent(basisPoints);
  }

  /**
   * The word for a VAT kind, from the dictionary both this screen and the API's values share. A kind
   * this build does not know is shown as its own code rather than blanked out.
   */
  kind(value: string): string {
    const key = `finanzen.taxes.kind.${value}`;
    const label = this.i18n.translate(key);
    return label === key ? value : label;
  }
}
