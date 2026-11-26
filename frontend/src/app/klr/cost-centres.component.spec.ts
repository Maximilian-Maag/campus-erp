import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { CostCentresComponent } from './cost-centres.component';
import { CostCentre, KlrReport } from '../core/api/generated';
import { provideI18n } from '../i18n/provide-i18n';

describe('CostCentresComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [CostCentresComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideI18n()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const centre = (ID: number, Number: string, over: Partial<CostCentre> = {}): CostCentre => ({
    ID,
    Number,
    Name: 'Allgemeine Verwaltung',
    CreatedBy: 'alice',
    ...over,
  });

  const report = (CostCentreID: number, CostsCents: number, RevenuesCents: number): KlrReport => ({
    CostCentreID,
    CostsCents,
    RevenuesCents,
  });

  /** The screen with its catalogue loaded. */
  function loaded(centres: CostCentre[] = [centre(1, 'KST-1')]) {
    const fixture = TestBed.createComponent(CostCentresComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-centres').flush(centres);
    fixture.detectChanges();
    return fixture;
  }

  function text(fixture: ReturnType<typeof TestBed.createComponent<CostCentresComponent>>): string {
    return (fixture.nativeElement as HTMLElement).textContent ?? '';
  }

  function button(
    fixture: ReturnType<typeof TestBed.createComponent<CostCentresComponent>>,
    name: string,
  ): HTMLButtonElement {
    const found = Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('button')).find((b) =>
      (b.textContent ?? '').includes(name),
    );
    if (!found) {
      throw new Error('no button ' + name);
    }
    return found as HTMLButtonElement;
  }

  it('opens loading, with an empty form and nothing claimed', () => {
    const fixture = TestBed.createComponent(CostCentresComponent);
    // The first state is "loading", not "loaded with nothing": nothing has answered yet.
    expect(fixture.componentInstance.loading()).toBeTrue();
    fixture.detectChanges();
    const c = fixture.componentInstance;
    expect(c.loading()).toBeTrue();
    expect(c.centres()).toEqual([]);
    expect(c.number()).toBe('');
    expect(c.name()).toBe('');
    expect(c.report()).toBeNull();
    expect(c.reporting()).toBeNull();
    expect(c.notice()).toBeNull();
    expect(c.actionError()).toBeNull();
    expect(c.listError()).toBeNull();
    expect(c.reportError()).toBeNull();
    // Nothing blocks a submission before anything is in flight.
    expect(button(fixture, 'Kostenstelle aufnehmen').disabled).toBeFalse();
    http.expectOne('/api/klr/cost-centres').flush([]);
  });

  it('lists the cost centres and says so when there are none', () => {
    const fixture = loaded([centre(1, 'KST-1'), centre(2, 'KST-2', { Name: 'Bibliothek' })]);
    expect(text(fixture)).toContain('KST-1');
    expect(text(fixture)).toContain('Bibliothek');
    expect(text(fixture)).not.toContain('Es ist keine Kostenstelle angelegt.');

    const empty = loaded([]);
    expect(text(empty)).toContain('Es ist keine Kostenstelle angelegt.');
  });

  it('shows an answer that arrives as null as an empty catalogue rather than crashing', () => {
    const fixture = TestBed.createComponent(CostCentresComponent);
    fixture.detectChanges();
    // A nil Go slice serialises as null; the screen must show the empty state, not throw.
    http.expectOne('/api/klr/cost-centres').flush(null);
    fixture.detectChanges();
    expect(fixture.componentInstance.centres()).toEqual([]);
    expect(text(fixture)).toContain('Es ist keine Kostenstelle angelegt.');
  });

  it('records a cost centre, trims it and keeps the list in number order', () => {
    const fixture = loaded([centre(1, 'KST-2')]);
    const c = fixture.componentInstance;
    c.number.set('  KST-1 ');
    c.name.set('  Allgemeine Verwaltung ');
    c.create(new Event('submit'));

    const post = http.expectOne('/api/klr/cost-centres');
    expect(post.request.method).toBe('POST');
    expect(post.request.body).toEqual({ number: 'KST-1', name: 'Allgemeine Verwaltung' });
    // The submit is held while the write is in flight, so a centre cannot be filed twice.
    fixture.detectChanges();
    expect(button(fixture, 'Kostenstelle aufnehmen').disabled).toBeTrue();

    post.flush(centre(2, 'KST-1'));
    fixture.detectChanges();
    expect(c.notice()).toBe('Kostenstelle KST-1 aufgenommen.');
    expect(c.number()).toBe('');
    expect(c.name()).toBe('');
    expect(button(fixture, 'Kostenstelle aufnehmen').disabled).toBeFalse();
    const rows = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('[data-testid^="centre-"]'),
    ).map((row) => row.getAttribute('data-testid'));
    expect(rows).toEqual(['centre-2', 'centre-1']);
  });

  it('refuses a cost centre without a number or a name, including blanks', () => {
    const fixture = loaded([]);
    const c = fixture.componentInstance;
    c.name.set('Allgemeine Verwaltung');
    c.create(new Event('submit'));
    expect(c.actionError()).toContain('erforderlich');
    c.number.set('   ');
    c.create(new Event('submit'));
    expect(c.actionError()).toContain('erforderlich');
    c.number.set('KST-1');
    c.name.set('  ');
    c.create(new Event('submit'));
    expect(c.actionError()).toContain('erforderlich');
    http.expectNone('/api/klr/cost-centres');
  });

  it('reads one centre report on demand and names whose it is', () => {
    const fixture = loaded([centre(1, 'KST-1'), centre(2, 'KST-2')]);
    const c = fixture.componentInstance;
    c.showReport(centre(2, 'KST-2', { Name: 'Bibliothek' }));
    expect(c.reporting()?.ID).toBe(2);
    http.expectOne('/api/klr/cost-centres/2/report').flush(report(2, 100000, 120000));
    fixture.detectChanges();

    const panel = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="report-2"]');
    expect(panel?.textContent).toContain('KST-2 Bibliothek');
    expect(panel?.textContent).toContain('1.000,00');
    expect(panel?.textContent).toContain('1.200,00');
    // Erlöse above Kosten is a surplus, and the word says so.
    expect(panel?.textContent).toContain('Überschuss 200,00');
  });

  it('words a shortfall and a balanced centre rather than showing a bare minus', () => {
    const fixture = loaded([centre(1, 'KST-1')]);
    const c = fixture.componentInstance;

    c.showReport(centre(1, 'KST-1'));
    http.expectOne('/api/klr/cost-centres/1/report').flush(report(1, 150000, 100000));
    fixture.detectChanges();
    expect(c.balanceLabel(report(1, 150000, 100000))).toBe('Unterdeckung 500,00');

    c.showReport(centre(1, 'KST-1'));
    http.expectOne('/api/klr/cost-centres/1/report').flush(report(1, 100000, 100000));
    fixture.detectChanges();
    expect(c.balanceLabel(report(1, 100000, 100000))).toBe('ausgeglichen');
  });

  it('drops a stale report when the next one cannot be read', () => {
    const fixture = loaded([centre(1, 'KST-1'), centre(2, 'KST-2')]);
    const c = fixture.componentInstance;
    c.showReport(centre(1, 'KST-1'));
    http.expectOne('/api/klr/cost-centres/1/report').flush(report(1, 100000, 120000));
    fixture.detectChanges();
    expect(c.report()).not.toBeNull();

    c.showReport(centre(2, 'KST-2'));
    http
      .expectOne('/api/klr/cost-centres/2/report')
      .flush({}, { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();
    // The first centre's figures must not stay on screen under the second centre's heading.
    expect(c.report()).toBeNull();
    expect(c.reportError()).toContain('Bericht');
    expect(text(fixture)).not.toContain('Überschuss');
  });

  it('clears the previous report as soon as another centre is chosen', () => {
    const fixture = loaded([centre(1, 'KST-1'), centre(2, 'KST-2')]);
    const c = fixture.componentInstance;
    c.showReport(centre(1, 'KST-1'));
    http.expectOne('/api/klr/cost-centres/1/report').flush(report(1, 100000, 120000));
    fixture.detectChanges();
    expect(c.report()).not.toBeNull();

    c.showReport(centre(2, 'KST-2'));
    fixture.detectChanges();
    // The request is still in flight: the first centre's figures must not sit under the new heading.
    expect(c.report()).toBeNull();
    expect(text(fixture)).not.toContain('Überschuss');
    http.expectOne('/api/klr/cost-centres/2/report').flush(report(2, 100000, 90000));
  });

  it('ignores a report that arrives after another centre was chosen', () => {
    const fixture = loaded([centre(1, 'KST-1'), centre(2, 'KST-2')]);
    const c = fixture.componentInstance;

    c.showReport(centre(1, 'KST-1'));
    const first = http.expectOne('/api/klr/cost-centres/1/report');
    c.showReport(centre(2, 'KST-2'));
    http.expectOne('/api/klr/cost-centres/2/report').flush(report(2, 100000, 90000));

    // The first response lands late: it belongs to a centre the screen no longer shows.
    first.flush(report(1, 100000, 120000));
    fixture.detectChanges();

    expect(c.reporting()?.ID).toBe(2);
    expect(c.report()?.CostCentreID).toBe(2);
  });

  it('reports a catalogue that cannot be read instead of showing an empty one', () => {
    const fixture = TestBed.createComponent(CostCentresComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-centres').flush({}, { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();
    const c = fixture.componentInstance;
    expect(c.listError()).toContain('Kostenstellen');
    expect(c.loading()).toBeFalse();
    expect(text(fixture)).not.toContain('Es ist keine Kostenstelle angelegt.');
  });

  it('reports a write the server refuses and frees the submit', () => {
    const fixture = loaded([]);
    const c = fixture.componentInstance;
    c.number.set('KST-1');
    c.name.set('Allgemeine Verwaltung');
    c.create(new Event('submit'));
    http
      .expectOne('/api/klr/cost-centres')
      .flush({}, { status: 422, statusText: 'Unprocessable Entity' });
    fixture.detectChanges();
    expect(c.actionError()).toContain('Kostenstelle');
    expect(c.busy()).toBeFalse();
    // What was typed stays, so it can be corrected rather than retyped.
    expect(c.number()).toBe('KST-1');
  });
});
