import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ReturnPostingsComponent, isDay } from './return-postings.component';
import { StatementLine } from '../core/api/generated';

describe('ReturnPostingsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [ReturnPostingsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const line = (over: Partial<StatementLine> = {}): StatementLine => ({
    ID: 7,
    LineNo: 1,
    ValueDate: '2026-11-20T00:00:00Z',
    BookingDate: '2026-11-20T00:00:00Z',
    Credit: false,
    Return: true,
    AmountCents: 12500,
    Currency: 'EUR',
    PostingKey: '009',
    ClientRef: 'ANORD-2026-9',
    BankRef: 'BANKREF',
    Purpose: 'Rückläufer Lastschrift',
    EndToEndRef: '',
    CounterpartyName: '',
    CounterpartyIBAN: '',
    CounterpartyBIC: '',
    UnclearedCents: 0,
    Status: 'returned',
    OrderID: null,
    ...over,
  });

  let fixture: ComponentFixture<ReturnPostingsComponent>;
  let component: ReturnPostingsComponent;
  let el: HTMLElement;

  function screen(): void {
    fixture = TestBed.createComponent(ReturnPostingsComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    el = fixture.nativeElement as HTMLElement;
  }

  /**
   * The generated client builds the query into the URL itself, so the period is matched on
   * `urlWithParams` — the same string the browser would send.
   */
  function expectLoad(from: string, to: string) {
    return http.expectOne(
      (r) =>
        r.method === 'GET' && r.urlWithParams === `/api/bank-statement-returns?from=${from}&to=${to}`
    );
  }

  /** Loads a period and answers it with the given lines, re-rendering the screen. */
  function loadPeriod(from: string, to: string, lines: StatementLine[]): void {
    component.from.set(from);
    component.to.set(to);
    component.load(new Event('submit'));
    expectLoad(from, to).flush(lines);
    fixture.detectChanges();
  }

  it('starts with the first of the current month and today, and nothing loaded', () => {
    screen();
    const now = new Date();
    const first = new Date(now.getFullYear(), now.getMonth(), 1);
    const pad = (n: number) => String(n).padStart(2, '0');
    expect(component.from()).toBe(`${first.getFullYear()}-${pad(first.getMonth() + 1)}-01`);
    expect(component.to()).toBe(
      `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
    );
    expect(component.loaded()).toBeFalse();
    expect(component.loading()).toBeFalse();
    expect(component.lines()).toEqual([]);
    expect(component.loadError()).toBeNull();
    expect(component.notice()).toBeNull();
    expect(el.querySelector('table')).toBeNull();
  });

  it('asks the server for the stated period and lists the lines', () => {
    screen();
    component.from.set('2026-11-01');
    component.to.set('2026-11-30');
    component.load(new Event('submit'));

    expect(component.loading()).toBeTrue();
    const req = expectLoad('2026-11-01', '2026-11-30');
    expect(req.request.method).toBe('GET');
    req.flush([line()]);
    fixture.detectChanges();

    expect(component.loading()).toBeFalse();
    expect(component.loaded()).toBeTrue();
    expect(component.lines().length).toBe(1);
    expect(el.textContent).toContain('Rückläufer Lastschrift');
    expect(el.textContent).toContain('009');
    expect(el.textContent).toContain('125,00');
    expect(el.querySelector('[data-testid="return-7"]')!.textContent).toContain('Ausgang');
    expect(el.textContent).toContain('20.11.2026');
  });

  it('shows an incoming return as an Eingang and an outgoing one as an Ausgang', () => {
    screen();
    loadPeriod('2026-11-01', '2026-11-30', [
      line({ ID: 8, Credit: true, PostingKey: '059', Purpose: 'Rückläufer Zahlung' }),
    ]);
    const row = el.querySelector('[data-testid="return-8"]')!;
    expect(row.textContent).toContain('Eingang');
    expect(row.textContent).toContain('059');
  });

  it('shows an empty period as an empty list, not as an error', () => {
    screen();
    loadPeriod('2026-11-01', '2026-11-30', []);

    expect(component.loaded()).toBeTrue();
    expect(component.loadError()).toBeNull();
    expect(el.textContent).toContain('keine Rückläufer eingelesen');
    expect(el.querySelector('table')).toBeNull();
  });

  it('reports a refused period and shows no table', () => {
    screen();
    component.from.set('2026-11-30');
    component.to.set('2026-11-01');
    component.load(new Event('submit'));
    expectLoad('2026-11-30', '2026-11-01').flush(
      { error: 'the return-postings period end precedes its start' },
      { status: 422, statusText: 'Unprocessable Entity' }
    );
    fixture.detectChanges();

    expect(component.loading()).toBeFalse();
    expect(component.loaded()).toBeFalse();
    expect(component.loadError()).toBe('Die Rückläufer konnten nicht geladen werden.');
    expect(el.querySelector('table')).toBeNull();
    expect(el.textContent).toContain('Die Rückläufer konnten nicht geladen werden.');
  });

  it('refuses a malformed day before asking the API', () => {
    screen();
    for (const bad of ['', '01.11.2026', '2026-1-1', 'x2026-11-01']) {
      component.from.set(bad);
      component.to.set('2026-11-30');
      component.load(new Event('submit'));
      expect(component.loadError()).toBe('Ein Zeitraum aus zwei Tagen ist erforderlich.');
      expect(component.loaded()).toBeFalse();

      component.from.set('2026-11-01');
      component.to.set(bad);
      component.load(new Event('submit'));
      expect(component.loadError()).toBe('Ein Zeitraum aus zwei Tagen ist erforderlich.');
    }
    http.expectNone('/api/bank-statement-returns');
  });

  it('clears a previous load error when the period is stated again', () => {
    screen();
    component.from.set('');
    component.load(new Event('submit'));
    expect(component.loadError()).not.toBeNull();

    loadPeriod('2026-11-01', '2026-11-30', [line()]);
    expect(component.loadError()).toBeNull();
    expect(component.loaded()).toBeTrue();
  });

  it('queues the return-postings file for the loaded period', () => {
    screen();
    loadPeriod('2026-11-01', '2026-11-30', [line()]);

    component.queue(new Event('submit'));
    expect(component.queueing()).toBeTrue();
    const req = http.expectOne('/api/ledger/return-postings');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ from: '2026-11-01', to: '2026-11-30' });
    req.flush({ job_id: 42 });
    fixture.detectChanges();

    expect(component.queueing()).toBeFalse();
    expect(component.notice()).toBe('Übergabe an die FiBu eingeplant (Auftrag 42).');
    expect(el.textContent).toContain('Auftrag 42');
  });

  it('queues the period as typed even after the list was loaded for another one', () => {
    screen();
    loadPeriod('2026-11-01', '2026-11-30', [line()]);

    // The operator states a different period: the export must follow the fields, not the rows.
    component.from.set('2026-12-01');
    component.to.set('2026-12-31');
    component.queue(new Event('submit'));
    const req = http.expectOne('/api/ledger/return-postings');
    expect(req.request.body).toEqual({ from: '2026-12-01', to: '2026-12-31' });
    req.flush({ job_id: 7 });
  });

  it('reports a failed queue and keeps the lines', () => {
    screen();
    loadPeriod('2026-11-01', '2026-11-30', [line()]);

    component.queue(new Event('submit'));
    http.expectOne('/api/ledger/return-postings').flush(
      { error: 'boom' },
      { status: 500, statusText: 'Server Error' }
    );
    fixture.detectChanges();

    expect(component.queueing()).toBeFalse();
    expect(component.actionError()).toBe('Die Übergabe konnte nicht eingeplant werden.');
    expect(component.notice()).toBeNull();
    expect(component.lines().length).toBe(1);
    expect(el.textContent).toContain('Die Übergabe konnte nicht eingeplant werden.');
  });

  it('formats amounts, days and the direction of a return', () => {
    screen();
    expect(component.amount(12500)).toBe('125,00');
    expect(component.day('2026-11-20T00:00:00Z')).toBe('20.11.2026');
    expect(component.direction(line({ Credit: true }))).toBe('Eingang');
    expect(component.direction(line({ Credit: false }))).toBe('Ausgang');
  });

  it('recognises the API day form and nothing else', () => {
    expect(isDay('2026-11-20')).toBeTrue();
    expect(isDay('2026-11-20x')).toBeFalse();
    expect(isDay('20.11.2026')).toBeFalse();
    expect(isDay('')).toBeFalse();
  });
});
