import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { KlrCostUnitsComponent } from './cost-units.component';
import { CostCenter, CostUnit, CostUnitKey } from '../core/api/generated';

describe('KlrCostUnitsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [KlrCostUnitsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const center = (ID: number, over: Partial<CostCenter> = {}): CostCenter => ({
    ID,
    Number: 'K-10',
    Name: 'Verwaltung',
    CreatedBy: 'alice',
    ...over,
  });

  const unit = (ID: number, over: Partial<CostUnit> = {}): CostUnit => ({
    ID,
    Number: 'T-1',
    Name: 'Studiengang A',
    CreatedBy: 'alice',
    ...over,
  });

  const key = (ID: number, TargetUnitID = 1, over: Partial<CostUnitKey> = {}): CostUnitKey => ({
    ID,
    SourceCenterID: 1,
    TargetUnitID,
    ShareBP: 2500,
    ValidFrom: '2026-01-01T00:00:00Z',
    ValidTo: null,
    CreatedBy: 'alice',
    ...over,
  });

  const keysURL = (id: number | string) => '/api/klr/cost-centers/' + id + '/cost-unit-keys';

  /** The screen with centres, units and the first centre's keys loaded. */
  function loaded(centers: CostCenter[] = [center(1)], units: CostUnit[] = [unit(1)], keys: CostUnitKey[] = [key(1)]) {
    const fixture = TestBed.createComponent(KlrCostUnitsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-units').flush(units);
    http.expectOne('/api/klr/cost-centers').flush(centers);
    if (centers.length > 0) {
      http.expectOne(keysURL(centers[0].ID)).flush(keys);
    }
    fixture.detectChanges();
    return { fixture, component };
  }

  it('starts unloaded and blank', () => {
    const fixture = TestBed.createComponent(KlrCostUnitsComponent);
    const component = fixture.componentInstance;

    expect(component.centers()).toEqual([]);
    expect(component.units()).toEqual([]);
    expect(component.keys()).toEqual([]);
    expect(component.source()).toBe('');
    expect(component.target()).toBe('');
    expect(component.loading()).toBeTrue();
    expect(component.saving()).toBeFalse();
    expect(component.running()).toBeFalse();
    expect(component.loadError()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.notice()).toBeNull();
    expect(component.result()).toBeNull();
    // Only the share carries a seed value; the dates start empty.
    expect(component.sharePercent()).toBe('25');
    expect(component.validFrom()).toBe('');
    expect(component.validUntil()).toBe('');
    expect(component.runFrom()).toBe('');
    expect(component.runTo()).toBe('');
  });

  it('loads the centres, the units and the first centre keys, and renders them', () => {
    const { fixture, component } = loaded(
      [center(1), center(2, { Number: 'K-99', Name: 'Bibliothek' })],
      [unit(1), unit(2, { Number: 'T-2', Name: 'Studiengang B' })],
      [key(1, 2, { ShareBP: 7500 })],
    );
    const html = fixture.nativeElement as HTMLElement;

    expect(component.loading()).toBeFalse();
    // The first centre is the one the operator most likely means to start with.
    expect(component.source()).toBe('1');
    expect(component.target()).toBe('1');
    expect(component.keys().length).toBe(1);
    // The unit is shown as the operator reads it, number and name, with the share in percent.
    expect(html.querySelector('[data-testid="unit-key-target-1"]')?.textContent?.trim()).toBe('T-2 Studiengang B');
    expect(html.querySelector('[data-testid="unit-key-share-1"]')?.textContent).toContain('75 %');
  });

  it('treats empty bodies as no centres, no units and no keys', () => {
    const fixture = TestBed.createComponent(KlrCostUnitsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-units').flush(null);
    http.expectOne('/api/klr/cost-centers').flush(null);
    fixture.detectChanges();

    expect(component.centers()).toEqual([]);
    expect(component.units()).toEqual([]);
    expect(component.keys()).toEqual([]);
    expect(component.source()).toBe('');
    // With no unit to pick, the picker is left empty rather than pointing at a row that is not
    // there.
    expect(component.target()).toBe('');
    expect(component.loading()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('keine Kostenstellen angelegt');
  });

  it('says so when a centre has no keys and there are no units at all', () => {
    const { fixture } = loaded([center(1)], [], []);
    const html = fixture.nativeElement as HTMLElement;

    expect(html.querySelector('[data-testid="no-unit-keys"]')).not.toBeNull();
    expect(html.textContent).toContain('keine Kostenträger angelegt');
    // With no unit to pick there is nothing to define a key against.
    expect(html.querySelector('[data-testid="define-unit-key"]')).toBeNull();
  });

  it('shows an unbounded key as unbegrenzt and a bounded one by its end date', () => {
    const { fixture } = loaded([center(1)], [unit(1)], [
      key(1, 1, { ValidTo: '2026-06-30T00:00:00Z' }),
      key(2, 1, { ValidFrom: '2026-07-01T00:00:00Z' }),
    ]);
    const rows = (fixture.nativeElement as HTMLElement).querySelectorAll('tbody tr');

    expect(rows[0].textContent).toContain('30.06.2026');
    expect(rows[1].textContent).toContain('unbegrenzt');
    // The start date is a German day too, not the ISO the API sent.
    expect(rows[0].textContent).toContain('01.01.2026');
  });

  it('falls back to the id for a unit it has not loaded', () => {
    const { fixture } = loaded([center(1)], [unit(1)], [key(1, 99)]);
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-testid="unit-key-target-1"]')?.textContent?.trim(),
    ).toBe('99');
  });

  it('treats an empty keys body as no keys', () => {
    const fixture = TestBed.createComponent(KlrCostUnitsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-units').flush([unit(1)]);
    http.expectOne('/api/klr/cost-centers').flush([center(1)]);
    http.expectOne(keysURL(1)).flush(null);
    fixture.detectChanges();

    expect(component.keys()).toEqual([]);
    expect(component.loadError()).toBeNull();
  });

  it('keeps a chosen centre across a reload even when it is not the first in the list', () => {
    const { component } = loaded([center(1), center(2, { Number: 'K-99' })], [unit(1), unit(2)], []);

    component.chooseSource('2');
    http.expectOne(keysURL(2)).flush([]);
    expect(component.source()).toBe('2');

    component.load();
    http.expectOne('/api/klr/cost-units').flush([unit(1), unit(2)]);
    http.expectOne('/api/klr/cost-centers').flush([center(1), center(2, { Number: 'K-99' })]);
    http.expectOne(keysURL(2)).flush([]);

    // Not the first entry: only the "is it still there" check keeps it.
    expect(component.source()).toBe('2');
  });

  it('clears the unit picker when there is no unit to pick', () => {
    const { component } = loaded([center(1), center(2)], [], []);

    component.target.set('9');
    component.chooseSource('2');

    expect(component.target()).toBe('');
    http.expectOne(keysURL(2)).flush([]);
  });

  it('shows the loading state again on a reload', () => {
    const { component } = loaded();
    expect(component.loading()).toBeFalse();

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/klr/cost-units').flush([unit(1)]);
    http.expectOne('/api/klr/cost-centers').flush([center(1)]);
    http.expectOne(keysURL(1)).flush([]);
    expect(component.loading()).toBeFalse();
  });

  it('reports centres that could not be loaded', () => {
    const fixture = TestBed.createComponent(KlrCostUnitsComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-units').flush([unit(1)]);
    http.expectOne('/api/klr/cost-centers').error(new ProgressEvent('error'));
    fixture.detectChanges();

    expect(fixture.componentInstance.loadError()).toBe('Konnte die Kostenstellen nicht laden.');
    expect(fixture.componentInstance.loading()).toBeFalse();
  });

  it('reports units that could not be loaded', () => {
    const fixture = TestBed.createComponent(KlrCostUnitsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-units').error(new ProgressEvent('error'));
    // The centres request is still open, so this is the units handler's own doing: it is the one
    // that ends the loading state when the units are the thing that failed.
    expect(component.loading()).toBeFalse();
    http.expectOne('/api/klr/cost-centers').flush([center(1)]);
    http.expectOne(keysURL(1)).flush([]);
    fixture.detectChanges();

    expect(component.loadError()).toBe('Konnte die Kostenträger nicht laden.');
  });

  it('reports keys that could not be loaded', () => {
    const { fixture, component } = loaded();

    component.loadKeys();
    http.expectOne(keysURL(1)).error(new ProgressEvent('error'));
    fixture.detectChanges();

    expect(component.loadError()).toBe('Konnte die Schlüssel nicht laden.');
  });

  it('clears the keys when there is no chosen centre', () => {
    const fixture = TestBed.createComponent(KlrCostUnitsComponent);
    const component = fixture.componentInstance;
    component.keys.set([key(1)]);
    component.source.set('');

    component.loadKeys();

    expect(component.keys()).toEqual([]);
  });

  it('switches to the centre the operator picks and reloads its keys', () => {
    const { fixture, component } = loaded([center(1), center(2, { Number: 'K-99' })], [unit(1), unit(2)], []);

    component.result.set({ AllocatedCents: 1, Receivers: 1 });
    component.actionError.set('etwas');
    component.chooseSource('2');

    expect(component.source()).toBe('2');
    expect(component.result()).toBeNull();
    expect(component.actionError()).toBeNull();
    // The unit picker goes back to the first unit, so it is never about a stale row.
    expect(component.target()).toBe('1');
    http.expectOne(keysURL(2)).flush([key(5, 2, { ShareBP: 10000 })]);
    fixture.detectChanges();

    expect(component.keys().map((k) => k.ID)).toEqual([5]);
  });

  it('refuses a share that is not a percentage, and one with no start date', () => {
    const { component } = loaded();

    component.sharePercent.set('keine Zahl');
    component.define(new Event('submit'));
    expect(component.actionError()).toBe('Der Anteil muss eine Prozentzahl zwischen 0 und 100 sein.');
    expect(component.saving()).toBeFalse();

    component.sharePercent.set('25');
    component.validFrom.set('');
    component.define(new Event('submit'));
    expect(component.actionError()).toBe('Ein Gültig-ab-Datum ist erforderlich.');
    expect(component.saving()).toBeFalse();
  });

  it('defines a key and reloads the keys', () => {
    const { component } = loaded([center(1)], [unit(1), unit(2)], []);

    component.target.set('2');
    component.sharePercent.set('25');
    component.validFrom.set('2026-01-01');
    component.validUntil.set('2026-12-31');
    component.define(new Event('submit'));
    expect(component.saving()).toBeTrue();

    const request = http.expectOne(keysURL(1));
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({
      target_unit_id: 2,
      share_bp: 2500,
      valid_from: '2026-01-01',
      valid_to: '2026-12-31',
    });
    request.flush(key(9, 2));

    expect(component.saving()).toBeFalse();
    expect(component.notice()).toBe('Schlüssel angelegt.');
    http.expectOne(keysURL(1)).flush([key(9, 2)]);
    expect(component.keys().map((k) => k.ID)).toEqual([9]);
  });

  it('reports a key that could not be defined', () => {
    const { component } = loaded();

    component.sharePercent.set('25');
    component.validFrom.set('2026-01-01');
    component.define(new Event('submit'));
    http.expectOne(keysURL(1)).error(new ProgressEvent('error'));

    expect(component.saving()).toBeFalse();
    expect(component.actionError()).toBe('Konnte den Schlüssel nicht anlegen.');
    expect(component.notice()).toBeNull();
  });

  it('refuses a run without both dates', () => {
    const { component } = loaded();

    component.runFrom.set('2026-04-01');
    component.runTo.set('');
    component.run(new Event('submit'));
    expect(component.actionError()).toBe('Für die Kostenträgerrechnung sind Von und Bis erforderlich.');
    expect(component.running()).toBeFalse();
  });

  it('runs a period and shows what was attributed', () => {
    const { fixture, component } = loaded();

    component.result.set({ AllocatedCents: 1, Receivers: 9 });
    component.runFrom.set('2026-04-01');
    component.runTo.set('2026-04-30');
    component.run(new Event('submit'));
    expect(component.running()).toBeTrue();
    // The previous answer is cleared before the new one is asked for.
    expect(component.result()).toBeNull();

    const request = http.expectOne(keysURL(1) + '/run');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({ from: '2026-04-01', to: '2026-04-30' });
    request.flush({ ID: 3, AllocatedCents: 100000, Receivers: 2 });
    fixture.detectChanges();

    expect(component.running()).toBeFalse();
    expect(component.result()?.Receivers).toBe(2);
    const shown = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="unit-run-result"]');
    expect(shown?.textContent).toContain('1.000,00');
    expect(shown?.textContent).toContain('2 Kostenträger');
  });

  it('reports a run that was refused', () => {
    const { component } = loaded();

    component.runFrom.set('2026-04-01');
    component.runTo.set('2026-04-30');
    component.run(new Event('submit'));
    http.expectOne(keysURL(1) + '/run').error(new ProgressEvent('error'));

    expect(component.running()).toBeFalse();
    expect(component.actionError()).toBe('Konnte die Kostenträgerrechnung nicht ausführen.');
    expect(component.result()).toBeNull();
  });
});
