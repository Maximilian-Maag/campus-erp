import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { KlrServiceRatesComponent } from './service-rates.component';
import { CostCenter, CostType, ServiceRate } from '../core/api/generated';

describe('KlrServiceRatesComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [KlrServiceRatesComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const center = (ID: number, over: Partial<CostCenter> = {}): CostCenter => ({
    ID,
    Number: 'K-' + ID,
    Name: 'Verwaltung',
    CreatedBy: 'alice',
    ...over,
  });

  const costType = (ID: number, over: Partial<CostType> = {}): CostType => ({
    ID,
    Code: '4711',
    Name: 'Personalkosten',
    CreatedBy: 'alice',
    ...over,
  });

  const rate = (ID: number, over: Partial<ServiceRate> = {}): ServiceRate => ({
    ID,
    CostTypeID: 1,
    RateCents: 2500,
    ValidFrom: '2026-01-01T00:00:00Z',
    ValidTo: null,
    CreatedBy: 'alice',
    ...over,
  });

  /** The screen with types, centres and rates loaded. */
  function loaded(
    types: CostType[] = [costType(1)],
    centers: CostCenter[] = [center(1), center(2)],
    rates: ServiceRate[] = [rate(1)],
  ) {
    const fixture = TestBed.createComponent(KlrServiceRatesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-types').flush(types);
    http.expectOne('/api/klr/cost-centers').flush(centers);
    http.expectOne('/api/klr/service-rates').flush(rates);
    fixture.detectChanges();
    return { fixture, component };
  }

  it('starts unloaded and blank', () => {
    const fixture = TestBed.createComponent(KlrServiceRatesComponent);
    const component = fixture.componentInstance;

    expect(component.rates()).toEqual([]);
    expect(component.costTypes()).toEqual([]);
    expect(component.centers()).toEqual([]);
    expect(component.rateCostType()).toBe('');
    expect(component.chargeCostType()).toBe('');
    expect(component.provider()).toBe('');
    expect(component.receiver()).toBe('');
    expect(component.rateAmount()).toBe('');
    expect(component.rateFrom()).toBe('');
    expect(component.rateUntil()).toBe('');
    expect(component.chargedOn()).toBe('');
    // Only the quantity carries a seed value; every other field starts empty.
    expect(component.quantity()).toBe('1');
    expect(component.loading()).toBeTrue();
    expect(component.saving()).toBeFalse();
    expect(component.charging()).toBeFalse();
    expect(component.loadError()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.notice()).toBeNull();
    expect(component.result()).toBeNull();
  });

  it('loads the rates, seeds the pickers and renders the table', () => {
    const { fixture, component } = loaded(
      [costType(1), costType(2, { Code: '5000', Name: 'Sachkosten' })],
      [center(1), center(2)],
      [rate(1, { RateCents: 2500 }), rate(2, { CostTypeID: 2, ValidTo: '2026-06-30T00:00:00Z' })],
    );
    const html = fixture.nativeElement as HTMLElement;

    expect(component.loading()).toBeFalse();
    // The first cost type and the first two centres are seeded, so nothing is submitted empty.
    expect(component.rateCostType()).toBe('1');
    expect(component.chargeCostType()).toBe('1');
    expect(component.provider()).toBe('1');
    expect(component.receiver()).toBe('2');
    expect(component.rates().length).toBe(2);
    expect(html.querySelector('[data-testid="rate-cost-type-1"]')?.textContent?.trim()).toBe('4711 Personalkosten');
    expect(html.querySelector('[data-testid="rate-amount-1"]')?.textContent).toContain('25,00');
    expect(html.querySelector('[data-testid="rate-2"]')?.textContent).toContain('30.06.2026');
    // An open-ended rate reads as unbegrenzt.
    expect(html.querySelector('[data-testid="rate-1"]')?.textContent).toContain('unbegrenzt');
  });

  it('leaves a picker the operator has already set alone on a reload', () => {
    const { component } = loaded();

    component.rateCostType.set('9');
    component.chargeCostType.set('8');
    component.provider.set('7');
    component.receiver.set('6');

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/klr/cost-types').flush([costType(1)]);
    http.expectOne('/api/klr/cost-centers').flush([center(1), center(2)]);
    http.expectOne('/api/klr/service-rates').flush([]);
    expect(component.loading()).toBeFalse();

    expect(component.rateCostType()).toBe('9');
    expect(component.chargeCostType()).toBe('8');
    expect(component.provider()).toBe('7');
    expect(component.receiver()).toBe('6');
  });

  it('treats empty bodies as nothing to show, with the pickers left blank', () => {
    const fixture = TestBed.createComponent(KlrServiceRatesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-types').flush(null);
    http.expectOne('/api/klr/cost-centers').flush(null);
    http.expectOne('/api/klr/service-rates').flush(null);
    fixture.detectChanges();

    expect(component.rates()).toEqual([]);
    expect(component.costTypes()).toEqual([]);
    expect(component.centers()).toEqual([]);
    expect(component.rateCostType()).toBe('');
    expect(component.chargeCostType()).toBe('');
    expect(component.provider()).toBe('');
    expect(component.receiver()).toBe('');
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="no-rates"]')).not.toBeNull();
    expect(html.textContent).toContain('keine Kostenarten angelegt');
  });

  it('offers no charge form with fewer than two centres', () => {
    const { fixture, component } = loaded([costType(1)], [center(1)], []);
    const html = fixture.nativeElement as HTMLElement;

    // One centre cannot charge another, so the form is not offered at all.
    expect(html.querySelector('[data-testid="charge-service"]')).toBeNull();
    expect(html.textContent).toContain('mindestens zwei Kostenstellen');
    // The rate form is still there: a price needs no second centre.
    expect(html.querySelector('[data-testid="define-rate"]')).not.toBeNull();
    expect(component.receiver()).toBe('');
  });

  it('offers no rate form without a cost type, but still offers the charge form', () => {
    const { fixture } = loaded([], [center(1), center(2)], []);
    const html = fixture.nativeElement as HTMLElement;

    expect(html.querySelector('[data-testid="define-rate"]')).toBeNull();
    // The charge form needs a cost type too, so it is withheld as well.
    expect(html.querySelector('[data-testid="charge-service"]')).toBeNull();
    expect(html.textContent).toContain('mindestens zwei Kostenstellen');
  });

  it('falls back to the id for a cost type it has not loaded', () => {
    const { fixture } = loaded([costType(1)], [center(1), center(2)], [rate(1, { CostTypeID: 99 })]);
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-testid="rate-cost-type-1"]')?.textContent?.trim(),
    ).toBe('99');
  });

  it('reports cost types that could not be loaded', () => {
    const fixture = TestBed.createComponent(KlrServiceRatesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-types').error(new ProgressEvent('error'));
    expect(component.loading()).toBeFalse();
    http.expectOne('/api/klr/cost-centers').flush([center(1)]);
    http.expectOne('/api/klr/service-rates').flush([]);
    fixture.detectChanges();

    expect(component.loadError()).toBe('Konnte die Kostenarten nicht laden.');
  });

  it('reports centres that could not be loaded', () => {
    const fixture = TestBed.createComponent(KlrServiceRatesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-types').flush([costType(1)]);
    http.expectOne('/api/klr/cost-centers').error(new ProgressEvent('error'));
    expect(component.loading()).toBeFalse();
    http.expectOne('/api/klr/service-rates').flush([]);
    fixture.detectChanges();

    expect(component.loadError()).toBe('Konnte die Kostenstellen nicht laden.');
  });

  it('reports rates that could not be loaded', () => {
    const fixture = TestBed.createComponent(KlrServiceRatesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-types').flush([costType(1)]);
    http.expectOne('/api/klr/cost-centers').flush([center(1), center(2)]);
    http.expectOne('/api/klr/service-rates').error(new ProgressEvent('error'));
    fixture.detectChanges();

    expect(component.loadError()).toBe('Konnte die Verrechnungspreise nicht laden.');
  });

  it('refuses a price that is not an amount, and one with no start date', () => {
    const { component } = loaded();

    component.rateAmount.set('keine Zahl');
    component.defineRate(new Event('submit'));
    expect(component.actionError()).toBe('Der Preis muss ein Betrag über 0 sein.');
    expect(component.saving()).toBeFalse();

    component.rateAmount.set('25,50');
    component.rateFrom.set('');
    component.defineRate(new Event('submit'));
    expect(component.actionError()).toBe('Ein Gültig-ab-Datum ist erforderlich.');
    expect(component.saving()).toBeFalse();
  });

  it('defines a rate and reloads the list', () => {
    const { component } = loaded([costType(1)], [center(1), center(2)], []);

    component.rateCostType.set('1');
    component.rateAmount.set('25,50');
    component.rateFrom.set('2026-01-01');
    component.rateUntil.set('2026-12-31');
    component.defineRate(new Event('submit'));
    expect(component.saving()).toBeTrue();

    const request = http.expectOne('/api/klr/service-rates');
    expect(request.request.method).toBe('POST');
    // The price is typed in euros and goes out in cents.
    expect(request.request.body).toEqual({
      cost_type_id: 1,
      rate_cents: 2550,
      valid_from: '2026-01-01',
      valid_to: '2026-12-31',
    });
    request.flush(rate(9));

    expect(component.saving()).toBeFalse();
    expect(component.notice()).toBe('Verrechnungspreis hinterlegt.');
    expect(component.rateAmount()).toBe('');
    http.expectOne('/api/klr/service-rates').flush([rate(9)]);
    expect(component.rates().map((r) => r.ID)).toEqual([9]);
  });

  it('reports a rate that could not be defined', () => {
    const { component } = loaded([], [center(1), center(2)], []);

    component.rateAmount.set('25,50');
    component.rateFrom.set('2026-01-01');
    component.defineRate(new Event('submit'));
    http.expectOne('/api/klr/service-rates').error(new ProgressEvent('error'));

    expect(component.saving()).toBeFalse();
    expect(component.actionError()).toBe('Konnte den Verrechnungspreis nicht hinterlegen.');
    expect(component.notice()).toBeNull();
  });

  it('reports a rate list that could not be reloaded after defining one', () => {
    const { component } = loaded([], [center(1), center(2)], []);

    component.rateAmount.set('25,50');
    component.rateFrom.set('2026-01-01');
    component.defineRate(new Event('submit'));
    http.expectOne('/api/klr/service-rates').flush(rate(9));
    http.expectOne('/api/klr/service-rates').error(new ProgressEvent('error'));

    expect(component.loadError()).toBe('Konnte die Verrechnungspreise nicht laden.');
  });

  it('treats an empty rate list on a reload as no rates', () => {
    const { component } = loaded();

    component.rateAmount.set('25,50');
    component.rateFrom.set('2026-01-01');
    component.defineRate(new Event('submit'));
    http.expectOne('/api/klr/service-rates').flush(rate(9));
    http.expectOne('/api/klr/service-rates').flush(null);

    expect(component.rates()).toEqual([]);
    expect(component.notice()).toBe('Verrechnungspreis hinterlegt.');
  });

  it('accepts a quantity with surrounding whitespace', () => {
    // Number() ignores the whitespace itself, which is why the component does not trim first.
    const { component } = loaded();

    component.quantity.set('  3  ');
    component.chargedOn.set('2026-04-30');
    component.charge(new Event('submit'));

    const request = http.expectOne('/api/klr/service-charges');
    expect((request.request.body as { quantity: number }).quantity).toBe(3);
    request.flush({ ID: 4, AmountCents: 7500, Quantity: 3 });
  });

  it('refuses a quantity that is not a whole number above zero', () => {
    const { component } = loaded();

    component.quantity.set('1,5');
    component.charge(new Event('submit'));
    expect(component.actionError()).toBe('Die Menge muss eine ganze Zahl über 0 sein.');
    expect(component.charging()).toBeFalse();

    component.quantity.set('0');
    component.charge(new Event('submit'));
    expect(component.actionError()).toBe('Die Menge muss eine ganze Zahl über 0 sein.');

    component.quantity.set('-3');
    component.charge(new Event('submit'));
    expect(component.actionError()).toBe('Die Menge muss eine ganze Zahl über 0 sein.');
  });

  it('refuses a charge with no service date', () => {
    const { component } = loaded();

    component.quantity.set('3');
    component.charge(new Event('submit'));

    expect(component.actionError()).toBe('Ein Leistungsdatum ist erforderlich.');
  });

  it('charges a service and shows what it came to', () => {
    const { fixture, component } = loaded();

    component.result.set({ AmountCents: 1, Quantity: 9 });
    component.quantity.set('3');
    component.chargedOn.set('2026-04-30');
    component.charge(new Event('submit'));
    expect(component.charging()).toBeTrue();
    // The previous answer is cleared before the new one is asked for.
    expect(component.result()).toBeNull();

    const request = http.expectOne('/api/klr/service-charges');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({
      provider_center_id: 1,
      receiver_center_id: 2,
      cost_type_id: 1,
      quantity: 3,
      charged_on: '2026-04-30',
    });
    request.flush({ ID: 4, AmountCents: 7500, Quantity: 3 });
    fixture.detectChanges();

    expect(component.charging()).toBeFalse();
    expect(component.result()?.AmountCents).toBe(7500);
    const shown = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="charge-result"]');
    expect(shown?.textContent).toContain('75,00');
    expect(shown?.textContent).toContain('3 Einheiten');
  });

  it('reports a charge that was refused', () => {
    const { component } = loaded();

    component.quantity.set('3');
    component.chargedOn.set('2026-04-30');
    component.charge(new Event('submit'));
    http.expectOne('/api/klr/service-charges').error(new ProgressEvent('error'));

    expect(component.charging()).toBeFalse();
    expect(component.actionError()).toBe('Konnte die Leistung nicht verrechnen.');
    expect(component.result()).toBeNull();
  });
});
