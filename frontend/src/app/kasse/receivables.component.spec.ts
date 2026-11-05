import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ReceivablesComponent, stateLabel } from './receivables.component';
import { euroToCents } from '../core/format';
import { Receivable } from '../core/api/generated';

describe('euroToCents', () => {
  it('reads a plain, comma or point amount as cents', () => {
    expect(euroToCents('425')).toBe(42500);
    expect(euroToCents('425,00')).toBe(42500);
    expect(euroToCents('425.5')).toBe(42550);
    expect(euroToCents('0,01')).toBe(1);
    expect(euroToCents('  1.250,75 ')).toBe(125075);
  });

  it('refuses a non-positive or malformed amount', () => {
    for (const bad of ['', ' ', '0', '0,00', '-5', 'abc', '1,234', '1.2.3', ',50', '5€']) {
      expect(euroToCents(bad)).toBeNull();
    }
  });
});

describe('stateLabel', () => {
  it('names the states in German, and passes an unknown one through', () => {
    expect(stateLabel('open')).toBe('offen');
    expect(stateLabel('collected')).toBe('eingezogen');
    expect(stateLabel('cancelled')).toBe('storniert');
    expect(stateLabel('other')).toBe('other');
  });
});

describe('ReceivablesComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [ReceivablesComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const item = (ID: number, over: Partial<Receivable> = {}): Receivable => ({
    ID,
    MandateReference: 'MAN-1',
    AmountCents: 125000,
    Currency: 'EUR',
    DueDate: '2026-10-15T00:00:00Z',
    Purpose: 'Semesterbeitrag',
    State: 'open',
    CollectedRunID: null,
    Source: 'manual',
    CreatedBy: 'carol',
    ...over,
  });

  /** The screen with the open items loaded. */
  function loaded(items: Receivable[] = [item(1)]) {
    const fixture = TestBed.createComponent(ReceivablesComponent);
    fixture.detectChanges();
    http.expectOne('/api/receivables?state=open').flush(items);
    fixture.detectChanges();
    return fixture;
  }

  it('starts unloaded and empty', () => {
    const fixture = TestBed.createComponent(ReceivablesComponent);
    const component = fixture.componentInstance;

    expect(component.items()).toEqual([]);
    expect(component.stateFilter()).toBe('open');
    expect(component.loading()).toBeTrue();
    expect(component.saving()).toBeFalse();
    expect(component.notice()).toBeNull();
    expect(component.actionError()).toBeNull();
    // The form starts blank, with only the currency defaulting to euro.
    expect(component.mandate()).toBe('');
    expect(component.amount()).toBe('');
    expect(component.currency()).toBe('EUR');
    expect(component.dueDate()).toBe('');
    expect(component.purpose()).toBe('');
    expect(component.source()).toBe('');
  });

  it('shows the loading state again on a reload', () => {
    const fixture = loaded([item(1)]);
    const component = fixture.componentInstance;
    expect(component.loading()).toBeFalse();

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/receivables?state=open').flush([item(1)]);
    fixture.detectChanges();
    expect(component.loading()).toBeFalse();
  });

  it('treats an empty response body as no items', () => {
    const fixture = TestBed.createComponent(ReceivablesComponent);
    fixture.detectChanges();
    http.expectOne('/api/receivables?state=open').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.items()).toEqual([]);
    expect(component.loadError()).toBeNull();
    expect(component.loading()).toBeFalse();
  });

  it('loads the open items and renders them', () => {
    const fixture = loaded([item(1), item(2, { MandateReference: 'MAN-2', State: 'collected' })]);
    const html = fixture.nativeElement as HTMLElement;

    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.items().length).toBe(2);
    const row = html.querySelector('[data-testid="receivable-1"]');
    expect(row?.textContent).toContain('MAN-1');
    expect(row?.textContent).toContain('1.250,00');
    expect(row?.textContent).toContain('15.10.2026');
    expect(row?.textContent).toContain('Semesterbeitrag');
    expect(row?.textContent).toContain('offen');
    expect(html.querySelector('[data-testid="receivable-2"]')?.textContent).toContain('eingezogen');
  });

  it('asks the server for the chosen state instead of sifting what it has', () => {
    const fixture = loaded([item(1)]);
    const component = fixture.componentInstance;

    component.choose('collected');
    const filtered = http.expectOne('/api/receivables?state=collected');
    expect(filtered.request.method).toBe('GET');
    filtered.flush([item(2, { State: 'collected' })]);
    fixture.detectChanges();

    expect(component.stateFilter()).toBe('collected');
    expect(component.items().map((r) => r.ID)).toEqual([2]);
  });

  it('leaves the filter out for "alle"', () => {
    const fixture = loaded([item(1)]);
    const component = fixture.componentInstance;

    component.choose('');
    // An empty state means every state: the query carries no filter.
    http.expectOne('/api/receivables').flush([item(1)]);
    fixture.detectChanges();

    expect(component.stateFilter()).toBe('');
  });

  it('says so when the chosen state has no items', () => {
    const fixture = loaded([]);
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Keine Posten in diesem Status.');
    expect((fixture.nativeElement as HTMLElement).querySelector('table')).toBeNull();
  });

  it('reports a failed load instead of an empty list', () => {
    const fixture = TestBed.createComponent(ReceivablesComponent);
    fixture.detectChanges();
    http.expectOne('/api/receivables?state=open').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die offenen Posten nicht laden.');
    expect(component.loading()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).querySelector('[role="alert"]')).toBeTruthy();
  });

  it('records an item and reloads the open list', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;
    component.mandate.set('  MAN-9  ');
    component.amount.set(' 425,50 ');
    component.currency.set(' eur ');
    component.dueDate.set(' 2026-11-30 ');
    component.purpose.set(' Bibliotheksgebühr ');
    component.source.set(' hand ');

    component.record(new Event('submit'));

    expect(component.saving()).toBeTrue();
    const post = http.expectOne('/api/receivables');
    expect(post.request.method).toBe('POST');
    // The amount is euros converted on the string; every field is trimmed and the currency is
    // upper-cased, so what a form control holds with stray spaces still sends clean.
    expect(post.request.body).toEqual({
      mandate_reference: 'MAN-9',
      amount_cents: 42550,
      currency: 'EUR',
      due_date: '2026-11-30',
      purpose: 'Bibliotheksgebühr',
      source: 'hand',
    });
    post.flush(item(9));
    fixture.detectChanges();

    expect(component.saving()).toBeFalse();
    expect(component.notice()).toBe('Posten erfasst.');
    expect(component.amount()).toBe('');
    expect(component.purpose()).toBe('');
    http.expectOne('/api/receivables?state=open').flush([item(9)]);
    fixture.detectChanges();
    expect(component.items().length).toBe(1);
  });

  it('switches to the open list after recording while another state is shown', () => {
    const fixture = TestBed.createComponent(ReceivablesComponent);
    fixture.detectChanges();
    http.expectOne('/api/receivables?state=open').flush([]);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    component.choose('collected');
    http.expectOne('/api/receivables?state=collected').flush([]);
    fixture.detectChanges();

    component.mandate.set('MAN-9');
    component.amount.set('10');
    component.dueDate.set('2026-11-30');
    component.purpose.set('Beitrag');
    component.record(new Event('submit'));
    http.expectOne('/api/receivables').flush(item(9));
    fixture.detectChanges();

    // A new item is open, so the shown state follows it back to open.
    expect(component.stateFilter()).toBe('open');
    http.expectOne('/api/receivables?state=open').flush([item(9)]);
  });

  it('refuses a bad amount without asking the API', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;
    component.mandate.set('MAN-9');
    component.dueDate.set('2026-11-30');
    for (const bad of ['', '0', 'abc']) {
      component.amount.set(bad);
      component.record(new Event('submit'));
      expect(component.actionError()).toBe('Der Betrag muss eine positive Zahl in Euro sein.');
      expect(component.saving()).toBeFalse();
    }
    http.expectNone('/api/receivables');
  });

  it('refuses a missing due date without asking the API', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;
    component.mandate.set('MAN-9');
    component.amount.set('5');
    // Blank and whitespace-only are both a missing due date.
    for (const missing of ['', '   ']) {
      component.dueDate.set(missing);
      component.record(new Event('submit'));
      expect(component.actionError()).toBe('Eine Fälligkeit ist erforderlich.');
      expect(component.saving()).toBeFalse();
    }
    http.expectNone('/api/receivables');
  });

  it('keeps the input and names the failure when a record is refused', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;
    component.mandate.set('MAN-absent');
    component.amount.set('5');
    component.dueDate.set('2026-11-30');
    component.purpose.set('Beitrag');

    component.record(new Event('submit'));
    http.expectOne('/api/receivables').flush('no', { status: 404, statusText: 'Not Found' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte den Posten nicht erfassen.');
    expect(component.saving()).toBeFalse();
    // The typed values are still there to correct and send again.
    expect(component.mandate()).toBe('MAN-absent');
    expect(component.amount()).toBe('5');
    http.expectNone('/api/receivables?state=open');
  });

  it('renders amounts and dates with the shared helpers', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;

    expect(component.euro(125000)).toBe('1.250,00');
    expect(component.euro(0)).toBe('0,00');
    expect(component.day('2026-10-31T00:00:00Z')).toBe('31.10.2026');
    expect(component.label('open')).toBe('offen');
  });
});
