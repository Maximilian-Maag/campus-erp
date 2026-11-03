import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { KlrAllocationComponent, basisPointsToShare, shareToBasisPoints } from './allocation.component';
import { Allocation, CostCenter } from '../core/api/generated';

describe('shareToBasisPoints', () => {
  it('reads a whole percentage', () => {
    expect(shareToBasisPoints('25')).toBe(2500);
    expect(shareToBasisPoints('100')).toBe(10000);
    expect(shareToBasisPoints('0')).toBe(0);
  });

  it('reads one and two decimals, with a dot or a comma', () => {
    expect(shareToBasisPoints('12.5')).toBe(1250);
    expect(shareToBasisPoints('12,5')).toBe(1250);
    expect(shareToBasisPoints('0.01')).toBe(1);
    expect(shareToBasisPoints('33,33')).toBe(3333);
  });

  it('tolerates surrounding spaces', () => {
    expect(shareToBasisPoints('  7  ')).toBe(700);
  });

  it('refuses what is not a share', () => {
    expect(shareToBasisPoints('')).toBeNull();
    expect(shareToBasisPoints('abc')).toBeNull();
    expect(shareToBasisPoints('-5')).toBeNull();
    expect(shareToBasisPoints('100.5')).toBeNull();
    expect(shareToBasisPoints('12.345')).toBeNull();
    expect(shareToBasisPoints('1234')).toBeNull();
    expect(shareToBasisPoints('1.2.3')).toBeNull();
  });
});

describe('basisPointsToShare', () => {
  it('renders whole percentages without a fraction', () => {
    expect(basisPointsToShare(2500)).toBe('25 %');
    expect(basisPointsToShare(10000)).toBe('100 %');
    expect(basisPointsToShare(0)).toBe('0 %');
  });

  it('renders one decimal where it is needed', () => {
    expect(basisPointsToShare(1250)).toBe('12,5 %');
    expect(basisPointsToShare(2550)).toBe('25,5 %');
  });

  it('renders a single basis point as two decimals', () => {
    expect(basisPointsToShare(1)).toBe('0,01 %');
    expect(basisPointsToShare(3333)).toBe('33,33 %');
  });
});

describe('KlrAllocationComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [KlrAllocationComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const center = (ID: number, Number: string, Name: string): CostCenter => ({
    ID, Number, Name, CreatedBy: 'alice',
  });
  const source = center(1, 'K-10', 'Bibliothek');
  const receiver = center(2, 'K-20', 'Werkstatt');

  const key = (ID: number, TargetCenterID: number, ShareBP: number): Allocation => ({
    ID, SourceCenterID: 1, TargetCenterID, ShareBP,
    ValidFrom: '2026-04-01', ValidTo: null, CreatedBy: 'alice',
  });

  /** Brings the screen to the state after the centre list and the keys have loaded. */
  function loaded(centers: CostCenter[] = [source, receiver], keys: Allocation[] = []) {
    const fixture = TestBed.createComponent(KlrAllocationComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-centers').flush(centers);
    fixture.detectChanges();
    const keyReq = http.match((r) => r.url === '/api/klr/cost-centers/1/allocations');
    keyReq.forEach((r) => r.flush(keys));
    fixture.detectChanges();
    return fixture;
  }

  it('loads the centres, picks the first and shows its keys', () => {
    const fixture = loaded([source, receiver], [key(7, 2, 2500)]);
    const component = fixture.componentInstance;

    expect(component.loading()).toBeFalse();
    expect(component.centers().length).toBe(2);
    expect(component.source()).toBe('1');
    expect(component.keys().length).toBe(1);

    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="key-7"]')).toBeTruthy();
    expect(html.querySelector('[data-testid="key-target-7"]')?.textContent).toContain('K-20');
    expect(html.querySelector('[data-testid="key-share-7"]')?.textContent).toContain('25 %');
  });

  it('offers only the other centres as receivers', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    expect(component.receivers().map((c) => c.ID)).toEqual([2]);
    expect(component.target()).toBe('2');
    // Choosing the receiver as the source leaves the original source as the only receiver.
    component.chooseSource('2');
    http.expectOne('/api/klr/cost-centers/2/allocations').flush([]);
    expect(component.receivers().map((c) => c.ID)).toEqual([1]);
    expect(component.target()).toBe('1');
  });

  it('reloads the keys when another centre is chosen and clears the last result', () => {
    const fixture = loaded([source, receiver], [key(7, 2, 2500)]);
    const component = fixture.componentInstance;
    component.result.set({ AllocatedCents: 1000, Receivers: 1 });

    component.chooseSource('2');
    const req = http.expectOne('/api/klr/cost-centers/2/allocations');
    req.flush([key(8, 1, 10000)]);
    fixture.detectChanges();

    expect(component.source()).toBe('2');
    expect(component.keys().length).toBe(1);
    expect(component.result()).toBeNull();
  });

  it('says so when no centre exists', () => {
    const fixture = loaded([]);
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[role="status"]')?.textContent).toContain('keine Kostenstellen angelegt');
    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.keys()).toEqual([]);
  });

  it('says so when the chosen centre has no keys', () => {
    const fixture = loaded([source, receiver], []);
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[role="status"]')?.textContent).toContain('kein Schlüssel hinterlegt');
    expect(html.querySelector('table')).toBeNull();
  });

  it('reports a failed load instead of an empty list', () => {
    const fixture = TestBed.createComponent(KlrAllocationComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-centers').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Kostenstellen nicht laden.');
    expect(component.loading()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).querySelector('[role="alert"]')).toBeTruthy();
  });

  it('reports a failed key load', () => {
    const fixture = TestBed.createComponent(KlrAllocationComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-centers').flush([source, receiver]);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-centers/1/allocations').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    expect(fixture.componentInstance.loadError()).toBe('Konnte die Schlüssel nicht laden.');
  });

  it('defines a key with the share converted to basis points and reloads the keys', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.validFrom.set('2026-04-01');
    component.validUntil.set('2026-06-30');
    component.sharePercent.set('12,5');

    component.define(new Event('submit'));

    expect(component.saving()).toBeTrue();
    const post = http.expectOne('/api/klr/cost-centers/1/allocations');
    expect(post.request.method).toBe('POST');
    expect(post.request.body).toEqual({
      target_center_id: 2, share_bp: 1250, valid_from: '2026-04-01', valid_to: '2026-06-30',
    });
    post.flush(key(9, 2, 1250));
    fixture.detectChanges();

    expect(component.saving()).toBeFalse();
    expect(component.notice()).toBe('Schlüssel angelegt.');
    http.expectOne('/api/klr/cost-centers/1/allocations').flush([key(9, 2, 1250)]);
    fixture.detectChanges();
    expect(component.keys().length).toBe(1);
  });

  it('refuses a share that is not a percentage, without asking the API', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.validFrom.set('2026-04-01');
    component.sharePercent.set('120');

    component.define(new Event('submit'));

    expect(component.actionError()).toBe('Der Anteil muss eine Prozentzahl zwischen 0 und 100 sein.');
    expect(component.saving()).toBeFalse();
    http.expectNone('/api/klr/cost-centers/1/allocations');
  });

  it('requires a valid-from date', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.validFrom.set('');

    component.define(new Event('submit'));

    expect(component.actionError()).toBe('Ein Gültig-ab-Datum ist erforderlich.');
    http.expectNone('/api/klr/cost-centers/1/allocations');
  });

  it('keeps the screen usable when defining a key fails', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.validFrom.set('2026-04-01');

    component.define(new Event('submit'));
    http.expectOne('/api/klr/cost-centers/1/allocations').flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte den Schlüssel nicht anlegen.');
    expect(component.saving()).toBeFalse();
    expect(component.loadError()).toBeNull();
    expect((fixture.nativeElement as HTMLElement).querySelector('[role="alert"]')).toBeTruthy();
  });

  it('runs a period and reports what it distributed', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.runFrom.set('2026-04-01');
    component.runTo.set('2026-04-30');

    component.run(new Event('submit'));

    expect(component.running()).toBeTrue();
    const post = http.expectOne('/api/klr/cost-centers/1/allocations/run');
    expect(post.request.method).toBe('POST');
    expect(post.request.body).toEqual({ from: '2026-04-01', to: '2026-04-30' });
    post.flush({ ID: 3, SourceCenterID: 1, PeriodFrom: '2026-04-01', PeriodTo: '2026-04-30', AllocatedCents: 80000, Receivers: 1, RunBy: 'carol' });
    fixture.detectChanges();

    expect(component.running()).toBeFalse();
    expect(component.result()?.AllocatedCents).toBe(80000);
    expect((fixture.nativeElement as HTMLElement).querySelector('[data-testid="run-result"]')?.textContent).toContain('800,00');
  });

  it('requires both dates for a run', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.runFrom.set('2026-04-01');
    component.runTo.set('');

    component.run(new Event('submit'));

    expect(component.actionError()).toBe('Für die Umlage sind Von und Bis erforderlich.');
    http.expectNone('/api/klr/cost-centers/1/allocations/run');
  });

  it('names the failure when a run is refused', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.runFrom.set('2026-04-01');
    component.runTo.set('2026-04-30');

    component.run(new Event('submit'));
    http.expectOne('/api/klr/cost-centers/1/allocations/run').flush('no', { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Umlage nicht ausführen.');
    expect(component.running()).toBeFalse();
    expect(component.result()).toBeNull();
  });

  it('shows an unloaded centre by its id and renders dates and an open end', () => {
    const fixture = loaded([source, receiver], [
      { ...key(7, 99, 2500), ValidTo: '2026-06-30' },
    ]);
    const component = fixture.componentInstance;
    const html = fixture.nativeElement as HTMLElement;

    expect(component.centerLabel(99)).toBe('99');
    expect(component.centerLabel(2)).toBe('K-20 Werkstatt');
    expect(html.querySelector('[data-testid="key-target-7"]')?.textContent).toContain('99');

    const cells = html.querySelectorAll('[data-testid="key-7"] td');
    expect(cells[2].textContent).toContain('2026-04-01');
    expect(cells[3].textContent).toContain('2026-06-30');

    // An open-ended key says so rather than showing an empty cell.
    component.keys.set([key(8, 2, 10000)]);
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelectorAll('[data-testid="key-8"] td')[3].textContent).toContain('unbegrenzt');
  });

  it('formats amounts with the shared formatter', () => {
    const fixture = loaded();
    expect(fixture.componentInstance.euro(11900)).toBe('119,00');
    expect(fixture.componentInstance.day('2026-04-30T00:00:00Z')).toBe('2026-04-30');
  });

  it('starts with an unloaded, empty screen', () => {
    // The initial state is observable before any request, which is what the mutants of the
    // signal declarations are checked against.
    const fixture = TestBed.createComponent(KlrAllocationComponent);
    const component = fixture.componentInstance;

    expect(component.centers()).toEqual([]);
    expect(component.keys()).toEqual([]);
    expect(component.source()).toBe('');
    expect(component.target()).toBe('');
    expect(component.sharePercent()).toBe('25');
    expect(component.validFrom()).toBe('');
    expect(component.validUntil()).toBe('');
    expect(component.runFrom()).toBe('');
    expect(component.runTo()).toBe('');
    expect(component.loading()).toBeTrue();
    expect(component.saving()).toBeFalse();
    expect(component.running()).toBeFalse();
    expect(component.loadError()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.notice()).toBeNull();
    expect(component.result()).toBeNull();
  });

  it('sets loading again on a reload', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    expect(component.loading()).toBeFalse();

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/klr/cost-centers').flush([source, receiver]);
    fixture.detectChanges();
    // A reload keeps the chosen centre, so it does not ask for its keys again.
    http.expectNone('/api/klr/cost-centers/1/allocations');
    expect(component.loading()).toBeFalse();
    expect(component.source()).toBe('1');
  });

  it('treats an empty centre response body as no centres', () => {
    const fixture = TestBed.createComponent(KlrAllocationComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-centers').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.centers()).toEqual([]);
    expect(component.source()).toBe('');
    expect(component.loading()).toBeFalse();
    // No centre was chosen, so nothing asked for keys.
    http.expectNone('/api/klr/cost-centers/1/allocations');
  });

  it('treats an empty key response body as no keys', () => {
    const fixture = TestBed.createComponent(KlrAllocationComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-centers').flush([source, receiver]);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-centers/1/allocations').flush(null);
    fixture.detectChanges();

    expect(fixture.componentInstance.keys()).toEqual([]);
    expect(fixture.componentInstance.loadError()).toBeNull();
  });

  it('asks for no keys when no centre is chosen', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.keys.set([key(7, 2, 2500)]);

    component.source.set('');
    component.loadKeys();

    expect(component.keys()).toEqual([]);
    http.expectNone((r) => r.url.includes('/allocations'));
  });

  it('leaves the receiver unset when there is no other centre', () => {
    // One centre means no receiver to offer, and the screen says so by leaving it empty rather
    // than naming the centre itself.
    const fixture = loaded([source]);
    const component = fixture.componentInstance;

    expect(component.source()).toBe('1');
    expect(component.receivers()).toEqual([]);
    expect(component.target()).toBe('');

    component.chooseSource('1');
    http.expectOne('/api/klr/cost-centers/1/allocations').flush([]);
    expect(component.target()).toBe('');
  });
});
