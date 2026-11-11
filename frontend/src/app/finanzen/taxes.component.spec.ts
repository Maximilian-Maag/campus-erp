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
    expect(text).toContain('Umsatzsteuer');
    expect(text).toContain('Vorsteuer');
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
    fixture.componentInstance.createVAT(new Event('submit'));
    const req = http.expectOne('/api/vat-types');
    expect(req.request.body.rate_basis_points).toBe(750);
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
