import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TaxesComponent } from './taxes.component';
import { TaxOffice, VATType } from '../core/api/generated';

describe('TaxesComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [TaxesComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const vat = (Code: string, RateBasisPoints: number, over: Partial<VATType> = {}): VATType => ({
    ID: 1,
    Code,
    Description: 'Regelsteuersatz',
    RateBasisPoints,
    Kind: 'output',
    ...over,
  });

  const office = (ID: number, Year: number, Number: string, over: Partial<TaxOffice> = {}): TaxOffice => ({
    ID,
    Year,
    Number,
    Name: 'Finanzamt Musterstadt',
    Street: 'Amtsweg 1',
    PostalCode: '12345',
    City: 'Musterstadt',
    AccountHolder: 'Landeshauptkasse',
    IBAN: 'DE02120300000000202051',
    BIC: 'BYLADEM1001',
    ...over,
  });

  /** The two reads the screen opens with; the office read is matched by path, since the year rides
   * in the query string. */
  function open(fixture: ReturnType<typeof TestBed.createComponent<TaxesComponent>>, vats: VATType[], offices: TaxOffice[]): void {
    http.expectOne('/api/vat-types').flush(vats);
    http.expectOne((r) => r.url.startsWith('/api/tax-offices')).flush(offices);
    fixture.detectChanges();
  }

  function openEmpty(): ReturnType<typeof TestBed.createComponent<TaxesComponent>> {
    const fixture = TestBed.createComponent(TaxesComponent);
    fixture.detectChanges();
    open(fixture, [], []);
    return fixture;
  }

  it('lists the VAT types with their rate as a percentage and their kind in words', () => {
    const fixture = TestBed.createComponent(TaxesComponent);
    fixture.detectChanges();
    open(fixture, [vat('V19', 1900), vat('V7', 700, { Description: 'ermaessigt', Kind: 'input' })], []);
    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('V19');
    expect(text).toContain('19 %');
    // The kind is in the row itself, not only among the form's options: the row, not the page,
    // is what proves the catalogue carries the word.
    const cells = (id: string): string[] =>
      Array.from((fixture.nativeElement as HTMLElement).querySelectorAll(`[data-testid="${id}"] td`)).map(
        (td) => (td.textContent ?? '').trim(),
      );
    expect(cells('vat-V19')).toEqual(['V19', 'Regelsteuersatz', '19 %', 'Umsatzsteuer']);
    expect(cells('vat-V7')).toEqual(['V7', 'ermaessigt', '7 %', 'Vorsteuer']);
  });

  it('says so when a catalogue is empty instead of showing a blank table', () => {
    const fixture = openEmpty();
    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('Es ist kein Steuersatz erfasst.');
    expect(text).toContain('Es ist kein Finanzamt erfasst.');
  });

  it('records a VAT type and puts it in the catalogue in code order', () => {
    const fixture = openEmpty();
    const c = fixture.componentInstance;
    c.vatCode.set('V19');
    c.vatDescription.set('Regelsteuersatz');
    c.vatRate.set('19');
    c.createVAT(new Event('submit'));
    http.expectOne('/api/vat-types').flush(vat('V19', 1900));
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Steuersatz V19 aufgenommen.');

    // A key that sorts before the first lands ahead of it, so the catalogue reads in the same
    // order the server's ORDER BY code does rather than in the order rows arrived.
    c.vatCode.set('V07');
    c.vatDescription.set('ermaessigt');
    c.vatRate.set('7');
    c.createVAT(new Event('submit'));
    http.expectOne('/api/vat-types').flush(vat('V07', 700));
    fixture.detectChanges();
    const codes = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('[data-testid^="vat-"]'),
    ).map((row) => row.getAttribute('data-testid'));
    expect(codes).toEqual(['vat-V07', 'vat-V19']);
  });

  it('sends a decimal rate as basis points rather than as a percentage', () => {
    const fixture = openEmpty();
    fixture.componentInstance.vatCode.set('V75');
    fixture.componentInstance.vatDescription.set('ermaessigt');
    fixture.componentInstance.vatRate.set('7,5');
    // Nothing was chosen, so the rate is filed as Umsatzsteuer — the kind the select starts on.
    const select = (fixture.nativeElement as HTMLElement).querySelector('#vat-kind') as HTMLSelectElement;
    expect(select.selectedOptions[0]?.textContent?.trim()).toBe('Umsatzsteuer');
    fixture.componentInstance.createVAT(new Event('submit'));
    const req = http.expectOne('/api/vat-types');
    expect(req.request.body.rate_basis_points).toBe(750);
    expect(req.request.body.kind).toBe('output');
    req.flush(vat('V75', 750));
  });

  it('refuses a rate that is not a percentage and a VAT type without a key', () => {
    const fixture = openEmpty();
    // A rate is only judged once the type it belongs to is named.
    fixture.componentInstance.vatCode.set('V17');
    fixture.componentInstance.vatDescription.set('erhoht');
    fixture.componentInstance.vatRate.set('siebzehn');
    fixture.componentInstance.createVAT(new Event('submit'));
    expect(fixture.componentInstance.actionError()).toContain('Prozentangabe');
    http.expectNone('/api/vat-types');

    fixture.componentInstance.vatCode.set('   ');
    fixture.componentInstance.vatRate.set('17');
    fixture.componentInstance.createVAT(new Event('submit'));
    expect(fixture.componentInstance.actionError()).toContain('erforderlich');
    http.expectNone('/api/vat-types');
  });

  it('reads the Finanzämter of the stated year, leaving the narrowing to the server', () => {
    const fixture = openEmpty();
    fixture.componentInstance.year.set('2025');
    fixture.componentInstance.loadOffices();
    const req = http.expectOne((r) => r.url.startsWith('/api/tax-offices'));
    // The generated client appends the query to the URL, so the year is read from the full URL.
    expect(req.request.urlWithParams).toContain('year=2025');
    req.flush([office(4, 2025, 'F-4')]);
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('F-4');
  });

  it('records a Finanzamt with its account and clears the form afterwards', () => {
    const fixture = openEmpty();
    const c = fixture.componentInstance;
    c.officeYear.set('2026');
    c.officeNumber.set('F-9');
    c.officeName.set('Finanzamt Musterstadt');
    c.officeStreet.set('Amtsweg 1');
    c.officePostal.set('12345');
    c.officeCity.set('Musterstadt');
    c.officeHolder.set('Landeshauptkasse');
    c.officeIBAN.set('DE02120300000000202051');
    c.officeBIC.set('BYLADEM1001');
    c.createOffice(new Event('submit'));
    const req = http.expectOne('/api/tax-offices');
    expect(req.request.body).toEqual({
      year: 2026,
      number: 'F-9',
      name: 'Finanzamt Musterstadt',
      street: 'Amtsweg 1',
      postal_code: '12345',
      city: 'Musterstadt',
      account_holder: 'Landeshauptkasse',
      iban: 'DE02120300000000202051',
      bic: 'BYLADEM1001',
    });
    req.flush(office(9, 2026, 'F-9'));
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Finanzamt F-9 aufgenommen.');
    expect(c.officeNumber()).toBe('');
    expect(c.officeIBAN()).toBe('');
    // The year stays: an operator filing several offices of one year does not retype it.
    expect(c.officeYear()).toBe('2026');
  });

  it('refuses a Finanzamt without a year, a number or a name', () => {
    const fixture = openEmpty();
    const c = fixture.componentInstance;
    c.officeNumber.set('F-9');
    c.officeName.set('Finanzamt Musterstadt');
    c.createOffice(new Event('submit'));
    expect(c.actionError()).toContain('Jahr');
    c.officeYear.set('heuer');
    c.createOffice(new Event('submit'));
    expect(c.actionError()).toContain('Jahr');
    c.officeYear.set('2026');
    c.officeName.set('  ');
    c.createOffice(new Event('submit'));
    expect(c.actionError()).toContain('erforderlich');
    http.expectNone('/api/tax-offices');
  });

  it('reports each read that fails on its own, and still shows the readable catalogue', () => {
    const fixture = TestBed.createComponent(TaxesComponent);
    fixture.detectChanges();
    http.expectOne('/api/vat-types').flush({}, { status: 500, statusText: 'Server Error' });
    http.expectOne((r) => r.url.startsWith('/api/tax-offices')).flush([office(4, 2026, 'F-4')]);
    fixture.detectChanges();
    expect(fixture.componentInstance.vatLoadError()).toContain('Steuersätze');
    expect(fixture.componentInstance.officeLoadError()).toBeNull();
    // The read that worked is still shown: one unreadable catalogue does not hide the other.
    expect(fixture.nativeElement.textContent).toContain('F-4');
  });

  it('reports a Finanzamt read that fails', () => {
    const fixture = TestBed.createComponent(TaxesComponent);
    fixture.detectChanges();
    http.expectOne('/api/vat-types').flush([]);
    http.expectOne((r) => r.url.startsWith('/api/tax-offices')).flush(
      {},
      { status: 500, statusText: 'Server Error' },
    );
    fixture.detectChanges();
    expect(fixture.componentInstance.officeLoadError()).toContain('Finanzämter');
    expect(fixture.componentInstance.vatLoadError()).toBeNull();
  });

  it('shows both failures when both reads fail rather than only the last one', () => {
    const fixture = TestBed.createComponent(TaxesComponent);
    fixture.detectChanges();
    http.expectOne('/api/vat-types').flush({}, { status: 500, statusText: 'Server Error' });
    http.expectOne((r) => r.url.startsWith('/api/tax-offices')).flush(
      {},
      { status: 500, statusText: 'Server Error' },
    );
    fixture.detectChanges();
    const alerts = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('[role="alert"]'),
    ).map((el) => el.textContent ?? '');
    expect(alerts.some((a) => a.includes('Steuersätze'))).toBe(true);
    expect(alerts.some((a) => a.includes('Finanzämter'))).toBe(true);
  });

  /** The DOM value of one field, for asserting what an operator actually sees. */
  function value(fixture: ReturnType<typeof TestBed.createComponent<TaxesComponent>>, id: string): string {
    const el = (fixture.nativeElement as HTMLElement).querySelector('#' + id) as HTMLInputElement | HTMLSelectElement;
    return el.value;
  }

  function button(
    fixture: ReturnType<typeof TestBed.createComponent<TaxesComponent>>,
    name: string,
  ): HTMLButtonElement {
    const found = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('button'),
    ).find((b) => (b.textContent ?? '').includes(name));
    if (!found) {
      throw new Error('no button ' + name);
    }
    return found as HTMLButtonElement;
  }

  const VAT_FIELDS = ['vat-code', 'vat-description', 'vat-rate'];
  /** The Finanzamt fields a successful write clears — not the year, which is kept on purpose. */
  const OFFICE_FIELDS = [
    'office-number',
    'office-name',
    'office-street',
    'office-postal',
    'office-city',
    'office-holder',
    'office-iban',
    'office-bic',
  ];

  it('opens with empty fields, the filter blank and both submits available', () => {
    const fixture = openEmpty();
    // Nothing is claimed before anything happened.
    expect(fixture.componentInstance.notice()).toBeNull();
    expect(fixture.componentInstance.actionError()).toBeNull();
    expect(fixture.componentInstance.vatLoadError()).toBeNull();
    expect(fixture.componentInstance.officeLoadError()).toBeNull();
    // Every field starts empty, and the kind starts on Umsatzsteuer rather than on nothing.
    const all = ['office-year', 'office-new-year', ...VAT_FIELDS, ...OFFICE_FIELDS];
    const blank = all.filter((id) => value(fixture, id) !== '');
    expect(blank).toEqual([]);
    expect(value(fixture, 'vat-kind')).toBe('output');
    // Nothing blocks a submission before anything is in flight.
    expect(button(fixture, 'Steuersatz aufnehmen').disabled).toBe(false);
    expect(button(fixture, 'Finanzamt aufnehmen').disabled).toBe(false);
  });

  it('says it is reading the catalogue while the first read is in flight', () => {
    const fixture = TestBed.createComponent(TaxesComponent);
    fixture.detectChanges();
    // Before the answer arrives the screen reports the read rather than showing an empty table.
    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('Steuersätze werden geladen.');
    expect(text).not.toContain('Es ist kein Steuersatz erfasst.');
    open(fixture, [], []);
    expect(fixture.nativeElement.textContent).not.toContain('Steuersätze werden geladen.');
  });

  it('shows an answer that arrives as null as an empty catalogue rather than crashing', () => {
    const fixture = TestBed.createComponent(TaxesComponent);
    fixture.detectChanges();
    // A nil Go slice serialises as null; the screen must show the empty state, not throw.
    http.expectOne('/api/vat-types').flush(null);
    http.expectOne((r) => r.url.startsWith('/api/tax-offices')).flush(null);
    fixture.detectChanges();
    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('Es ist kein Steuersatz erfasst.');
    expect(text).toContain('Es ist kein Finanzamt erfasst.');
  });

  it('clears the VAT form and frees the submit after a successful write', () => {
    const fixture = openEmpty();
    const c = fixture.componentInstance;
    c.vatCode.set('V19');
    c.vatDescription.set('Regelsteuersatz');
    c.vatRate.set('19');
    c.createVAT(new Event('submit'));
    // The submit is held while the write is in flight, so a rate cannot be filed twice.
    fixture.detectChanges();
    expect(button(fixture, 'Steuersatz aufnehmen').disabled).toBe(true);
    http.expectOne('/api/vat-types').flush(vat('V19', 1900));
    fixture.detectChanges();
    expect(VAT_FIELDS.filter((id) => value(fixture, id) !== '')).toEqual([]);
    expect(button(fixture, 'Steuersatz aufnehmen').disabled).toBe(false);
  });

  it('clears the Finanzamt form and frees the submit after a successful write', () => {
    const fixture = openEmpty();
    const c = fixture.componentInstance;
    c.officeYear.set('2026');
    c.officeNumber.set('F-9');
    c.officeName.set('Finanzamt Musterstadt');
    c.officeStreet.set('Amtsweg 1');
    c.officePostal.set('12345');
    c.officeCity.set('Musterstadt');
    c.officeHolder.set('Landeshauptkasse');
    c.officeIBAN.set('DE02120300000000202051');
    c.officeBIC.set('BYLADEM1001');
    c.createOffice(new Event('submit'));
    fixture.detectChanges();
    expect(button(fixture, 'Finanzamt aufnehmen').disabled).toBe(true);
    http.expectOne('/api/tax-offices').flush(office(9, 2026, 'F-9'));
    fixture.detectChanges();
    expect(OFFICE_FIELDS.filter((id) => value(fixture, id) !== '')).toEqual([]);
    // The year stays, so an operator filing several offices of one year does not retype it.
    expect(value(fixture, 'office-new-year')).toBe('2026');
    expect(c.officeYear()).toBe('2026');
    expect(button(fixture, 'Finanzamt aufnehmen').disabled).toBe(false);
  });

  it('reads in number order, not in the order the rows arrived', () => {
    const fixture = openEmpty();
    const c = fixture.componentInstance;
    const file = (number: string, id: number): void => {
      c.officeYear.set('2026');
      c.officeNumber.set(number);
      c.officeName.set('Finanzamt Musterstadt');
      c.createOffice(new Event('submit'));
      http.expectOne('/api/tax-offices').flush(office(id, 2026, number));
      fixture.detectChanges();
    };
    file('F-9', 9);
    file('F-2', 2);
    const rows = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('[data-testid^="office-"]'),
    ).map((row) => row.getAttribute('data-testid'));
    // Both were filed and both are there — the later number does not drop the earlier row.
    expect(rows).toEqual(['office-2', 'office-9']);
  });

  it('refuses a VAT type whose description is missing or only spaces', () => {
    const fixture = openEmpty();
    const c = fixture.componentInstance;
    c.vatCode.set('V19');
    c.vatRate.set('19');
    c.vatDescription.set('');
    c.createVAT(new Event('submit'));
    expect(c.actionError()).toContain('erforderlich');
    // A description of blanks is not a description.
    c.vatDescription.set('   ');
    c.createVAT(new Event('submit'));
    expect(c.actionError()).toContain('erforderlich');
    http.expectNone('/api/vat-types');
  });

  it('refuses a Finanzamt with no number, and a year that is not a year', () => {
    const fixture = openEmpty();
    const c = fixture.componentInstance;
    c.officeYear.set('2026');
    c.officeName.set('Finanzamt Musterstadt');
    c.createOffice(new Event('submit'));
    expect(c.actionError()).toContain('erforderlich');
    // A number of blanks is not a number.
    c.officeNumber.set('   ');
    c.createOffice(new Event('submit'));
    expect(c.actionError()).toContain('erforderlich');
    // Year zero and a negative year are not years either.
    for (const year of ['0', '-1']) {
      c.officeYear.set(year);
      c.officeNumber.set('F-9');
      c.createOffice(new Event('submit'));
      expect(c.actionError()).toContain('Jahr');
      expect(c.officeYear()).toBe(year);
    }
    http.expectNone('/api/tax-offices');
  });

  it('reports a write that the server refuses', () => {
    const fixture = openEmpty();
    fixture.componentInstance.vatCode.set('V19');
    fixture.componentInstance.vatDescription.set('Regelsteuersatz');
    fixture.componentInstance.vatRate.set('19');
    fixture.componentInstance.createVAT(new Event('submit'));
    http.expectOne('/api/vat-types').flush({}, { status: 422, statusText: 'Unprocessable Entity' });
    fixture.detectChanges();
    expect(fixture.componentInstance.actionError()).toContain('Steuersatz');
    expect(fixture.componentInstance.busy()).toBe(false);
  });

  it('reports a Finanzamt the server refuses', () => {
    const fixture = openEmpty();
    const c = fixture.componentInstance;
    c.officeYear.set('2026');
    c.officeNumber.set('F-9');
    c.officeName.set('Finanzamt Musterstadt');
    c.createOffice(new Event('submit'));
    http.expectOne('/api/tax-offices').flush({}, { status: 422, statusText: 'Unprocessable Entity' });
    fixture.detectChanges();
    expect(c.actionError()).toContain('Finanzamt');
    expect(c.busy()).toBe(false);
  });
});
