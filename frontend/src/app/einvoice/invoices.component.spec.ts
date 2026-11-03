import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { EInvoicesComponent } from './invoices.component';
import { InvoiceSummary } from '../core/api/generated';

describe('EInvoicesComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [EInvoicesComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const invoice = (ID: number, Number: string, matched: number): InvoiceSummary => ({
    ID, Number, IssueDate: '2026-04-01T00:00:00Z', TypeCode: '380', Currency: 'EUR',
    BuyerReference: 'PO-2026-1', GrossCents: 35700, MatchedOrderID: matched,
  });
  const unmatched = invoice(1, 'RE-1', 0);
  const matched = invoice(2, 'RE-2', 7);

  it('loads the invoices as soon as it is shown', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    expect(fixture.componentInstance.loading()).toBeTrue();
    expect(fixture.componentInstance.invoices()).toEqual([]);
    expect(fixture.componentInstance.xml()).toBe('');
    fixture.detectChanges();

    const req = http.expectOne('/api/e-invoices');
    expect(req.request.method).toBe('GET');
    req.flush([unmatched, matched]);
    fixture.detectChanges();

    expect(fixture.componentInstance.invoices().length).toBe(2);
    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.loadError()).toBeNull();
  });

  it('renders each invoice with its date, amount and match state', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices').flush([unmatched, matched]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    const cells = html.querySelectorAll('[data-testid="invoice-RE-1"] td');
    expect(cells[0].textContent).toContain('RE-1');
    expect(cells[1].textContent).toContain('01.04.2026');
    expect(cells[2].textContent).toContain('PO-2026-1');
    expect(cells[3].textContent).toContain('357,00');
    expect(html.querySelector('[data-testid="match-RE-1"]')?.textContent).toContain('offen');
    expect(html.querySelector('[data-testid="match-RE-2"]')?.textContent).toContain('Bestellung 7');
  });

  it('offers a match only for an unmatched invoice', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices').flush([unmatched, matched]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    const button = html.querySelector('[data-testid="invoice-RE-1"] button');
    expect(button?.getAttribute('aria-label')).toBe('Rechnung RE-1 der Bestellung zuordnen');
    expect(html.querySelector('[data-testid="invoice-RE-2"] button')).toBeNull();
  });

  it('matches an invoice and reloads the list', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices').flush([unmatched]);
    fixture.detectChanges();

    fixture.componentInstance.match(unmatched);
    expect(fixture.componentInstance.matching()).toBe(1);

    const post = http.expectOne('/api/e-invoices/1/match');
    expect(post.request.method).toBe('POST');
    post.flush({ Number: 'RE-1' });

    expect(fixture.componentInstance.loading()).toBeTrue();
    http.expectOne('/api/e-invoices').flush([matched]);
    fixture.detectChanges();

    expect(fixture.componentInstance.matching()).toBeNull();
    expect(fixture.componentInstance.actionError()).toBeNull();
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="match-RE-2"]')?.textContent).toContain('Bestellung 7');
  });

  it('keeps the list and names the invoice when a match fails', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices').flush([unmatched]);
    fixture.detectChanges();

    fixture.componentInstance.match(unmatched);
    http.expectOne('/api/e-invoices/1/match').flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    expect(fixture.componentInstance.actionError()).toBe('Konnte die Rechnung RE-1 nicht zuordnen.');
    expect(fixture.componentInstance.matching()).toBeNull();
    expect(fixture.componentInstance.loadError()).toBeNull();
    expect(html.querySelector('[data-testid="invoice-RE-1"]')).toBeTruthy();
  });

  it('reads a document in and reloads the list', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices').flush([]);
    fixture.detectChanges();

    fixture.componentInstance.xml.set('<Invoice/>');
    fixture.componentInstance.importInvoice();
    expect(fixture.componentInstance.importing()).toBeTrue();

    const post = http.expectOne('/api/e-invoices/received');
    expect(post.request.method).toBe('POST');
    expect(post.request.body).toBe('<Invoice/>');
    expect(post.request.headers.get('Content-Type')).toBe('application/xml');
    post.flush({ Number: 'RE-1' });

    expect(fixture.componentInstance.importing()).toBeFalse();
    // The document is cleared and the list reloaded.
    expect(fixture.componentInstance.xml()).toBe('');
    http.expectOne('/api/e-invoices').flush([unmatched]);
    fixture.detectChanges();

    expect(fixture.componentInstance.invoices().length).toBe(1);
    expect(fixture.componentInstance.actionError()).toBeNull();
  });

  it('refuses to read in nothing, and reports a refused document', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices').flush([]);
    const component = fixture.componentInstance;

    // Nothing typed in: no request is made at all.
    component.xml.set('   ');
    component.importInvoice();
    expect(component.actionError()).toBe('Es ist kein XML eingegeben.');
    expect(component.importing()).toBeFalse();
    http.expectNone('/api/e-invoices/received');

    // A document the API refuses.
    component.xml.set('<not-xml');
    component.importInvoice();
    http.expectOne('/api/e-invoices/received').flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Rechnung nicht einlesen.');
    expect(component.importing()).toBeFalse();
    // The text stays put so it can be corrected.
    expect(component.xml()).toBe('<not-xml');
  });

  it('says so when no invoice is recorded, and reports a failed load', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices').flush([]);
    fixture.detectChanges();
    let html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('table')).toBeNull();
    expect(html.querySelector('[role="status"]')?.textContent).toContain('keine Rechnungen erfasst');

    const failing = TestBed.createComponent(EInvoicesComponent);
    failing.detectChanges();
    http.expectOne('/api/e-invoices').flush('nope', { status: 500, statusText: 'Server Error' });
    failing.detectChanges();
    html = failing.nativeElement as HTMLElement;
    expect(failing.componentInstance.loadError()).toBe('Konnte die Rechnungen nicht laden.');
    expect(html.querySelector('[role="alert"]')).toBeTruthy();
    expect(html.querySelector('[role="status"]')).toBeNull();
  });

  it('treats an empty response body as a list without invoices', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();

    http.expectOne('/api/e-invoices').flush(null);
    fixture.detectChanges();

    expect(component.invoices()).toEqual([]);
    expect(component.loading()).toBeFalse();
    expect(component.loadError()).toBeNull();
  });

  it('formats dates and amounts the way the screen shows them', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/e-invoices').flush([]);

    expect(component.day('2026-04-01T00:00:00Z')).toBe('01.04.2026');
    // An unparseable date is shown as it came, rather than as "Invalid Date".
    expect(component.day('nonsense')).toBe('nonsense');
    expect(component.euro(35700)).toBe('357,00');
    expect(component.euro(0)).toBe('0,00');
  });
});
