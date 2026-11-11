import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, TaxOffice, VATType } from '../core/api/generated';
import { basisPointsToPercent, percentToBasisPoints } from '../core/format';
import { VAT_KINDS, vatKindLabel } from '../core/tax';

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
  template: `
    <section aria-labelledby="taxes-heading">
      <h2 id="taxes-heading">Steuersätze und Finanzämter</h2>

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
        <p role="status">Steuersätze werden geladen.</p>
      } @else {
        <table>
          <caption>
            Steuersätze mit Bezeichnung, Satz und Art
          </caption>
          <thead>
            <tr>
              <th scope="col">Schlüssel</th>
              <th scope="col">Bezeichnung</th>
              <th scope="col" class="amount">Satz</th>
              <th scope="col">Art</th>
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
                <td colspan="4">Es ist kein Steuersatz erfasst.</td>
              </tr>
            }
          </tbody>
        </table>

        <form (submit)="createVAT($event)">
          <h3 id="vat-heading">Steuersatz aufnehmen</h3>
          <p>
            <label for="vat-code">Schlüssel</label>
            <input id="vat-code" [value]="vatCode()" (input)="vatCode.set($any($event.target).value)" />
          </p>
          <p>
            <label for="vat-description">Bezeichnung</label>
            <input
              id="vat-description"
              [value]="vatDescription()"
              (input)="vatDescription.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="vat-rate">Satz (in Prozent)</label>
            <input id="vat-rate" [value]="vatRate()" (input)="vatRate.set($any($event.target).value)" />
          </p>
          <p>
            <label for="vat-kind">Art</label>
            <select id="vat-kind" [value]="vatKind()" (change)="vatKind.set($any($event.target).value)">
              @for (k of kinds; track k.value) {
                <option [value]="k.value">{{ k.label }}</option>
              }
            </select>
          </p>
          <button type="submit" [disabled]="busy()">Steuersatz aufnehmen</button>
        </form>

        <p>
          <label for="office-year">Jahr der Anzeige</label>
          <input id="office-year" [value]="year()" (input)="year.set($any($event.target).value)" />
          <button type="button" (click)="loadOffices()">Anzeigen</button>
        </p>

        <table>
          <caption>
            Finanzämter mit Anschrift und Bankverbindung
          </caption>
          <thead>
            <tr>
              <th scope="col">Jahr</th>
              <th scope="col">Nummer</th>
              <th scope="col">Name</th>
              <th scope="col">Ort</th>
              <th scope="col">IBAN</th>
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
                <td colspan="5">Es ist kein Finanzamt erfasst.</td>
              </tr>
            }
          </tbody>
        </table>

        <form (submit)="createOffice($event)">
          <h3 id="office-heading">Finanzamt aufnehmen</h3>
          <p>
            <label for="office-new-year">Jahr</label>
            <input id="office-new-year" [value]="officeYear()" (input)="officeYear.set($any($event.target).value)" />
          </p>
          <p>
            <label for="office-number">Nummer</label>
            <input id="office-number" [value]="officeNumber()" (input)="officeNumber.set($any($event.target).value)" />
          </p>
          <p>
            <label for="office-name">Name</label>
            <input id="office-name" [value]="officeName()" (input)="officeName.set($any($event.target).value)" />
          </p>
          <p>
            <label for="office-street">Straße</label>
            <input id="office-street" [value]="officeStreet()" (input)="officeStreet.set($any($event.target).value)" />
          </p>
          <p>
            <label for="office-postal">PLZ</label>
            <input id="office-postal" [value]="officePostal()" (input)="officePostal.set($any($event.target).value)" />
          </p>
          <p>
            <label for="office-city">Ort</label>
            <input id="office-city" [value]="officeCity()" (input)="officeCity.set($any($event.target).value)" />
          </p>
          <p>
            <label for="office-holder">Kontoinhaber</label>
            <input id="office-holder" [value]="officeHolder()" (input)="officeHolder.set($any($event.target).value)" />
          </p>
          <p>
            <label for="office-iban">IBAN</label>
            <input id="office-iban" [value]="officeIBAN()" (input)="officeIBAN.set($any($event.target).value)" />
          </p>
          <p>
            <label for="office-bic">BIC</label>
            <input id="office-bic" [value]="officeBIC()" (input)="officeBIC.set($any($event.target).value)" />
          </p>
          <button type="submit" [disabled]="busy()">Finanzamt aufnehmen</button>
        </form>
      }
    </section>
  `,
})
export class TaxesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

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
        this.vatLoadError.set('Die Steuersätze konnten nicht geladen werden.');
        this.loading.set(false);
      },
    });
  }

  /** Reads the Finanzämter of the stated year — the server narrows, the screen does not. */
  loadOffices(): void {
    this.officeLoadError.set(null);
    this.api.listTaxOffices(this.year()).subscribe({
      next: (offices) => this.offices.set(offices ?? []),
      error: () => this.officeLoadError.set('Die Finanzämter konnten nicht geladen werden.'),
    });
  }

  createVAT(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);
    if (this.vatCode().trim() === '' || this.vatDescription().trim() === '') {
      this.actionError.set('Schlüssel und Bezeichnung des Steuersatzes sind erforderlich.');
      return;
    }
    const rate = percentToBasisPoints(this.vatRate());
    if (rate === null) {
      this.actionError.set('Der Satz muss eine Prozentangabe sein, zum Beispiel 19 oder 7,5.');
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
          this.notice.set(`Steuersatz ${saved.Code} aufgenommen.`);
          this.busy.set(false);
        },
        error: () => {
          this.actionError.set('Der Steuersatz konnte nicht aufgenommen werden.');
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
    const year = Number(this.officeYear());
    if (this.officeYear().trim() === '' || !Number.isInteger(year) || year <= 0) {
      this.actionError.set('Das Jahr des Finanzamts ist erforderlich.');
      return;
    }
    if (this.officeNumber().trim() === '' || this.officeName().trim() === '') {
      this.actionError.set('Nummer und Name des Finanzamts sind erforderlich.');
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
          this.notice.set(`Finanzamt ${saved.Number} aufgenommen.`);
          this.busy.set(false);
        },
        error: () => {
          this.actionError.set('Das Finanzamt konnte nicht aufgenommen werden.');
          this.busy.set(false);
        },
      });
  }

  /** The rate as the operator reads it, from the shared formatter. */
  rate(basisPoints: number): string {
    return basisPointsToPercent(basisPoints);
  }

  /** The German word for a VAT kind, from the list both this screen and the API's values share. */
  kind(value: string): string {
    return vatKindLabel(value);
  }
}
