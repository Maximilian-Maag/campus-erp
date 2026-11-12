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

  const invoice = (ID: number, Number: string, direction: string, matched: number): InvoiceSummary => ({
    ID, Number, IssueDate: '2026-04-01T00:00:00Z', TypeCode: '380', Currency: 'EUR',
    BuyerReference: 'PO-2026-1', GrossCents: 35700, Direction: direction, MatchedOrderID: matched,
  });
  const unmatched = invoice(1, 'RE-1', 'inbound', 0);
  const matched = invoice(2, 'RE-2', 'inbound', 7);
  const outbound = invoice(3, 'RE-3', 'outbound', 0);

  /** A page's worth of invoices, so the screen believes there may be another. */
  const fullPage = (from: number): InvoiceSummary[] =>
    Array.from({ length: 50 }, (_, i) => invoice(from + i, `RE-${from + i}`, 'inbound', 0));

  it('loads the invoices as soon as it is shown', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    expect(fixture.componentInstance.loading()).toBeTrue();
    expect(fixture.componentInstance.invoices()).toEqual([]);
    expect(fixture.componentInstance.xml()).toBe('');
    fixture.detectChanges();

    const req = http.expectOne('/api/e-invoices?limit=50&offset=0');
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
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([unmatched, matched]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    const cells = html.querySelectorAll('[data-testid="invoice-RE-1"] td');
    expect(cells[0].textContent).toContain('RE-1');
    expect(cells[2].textContent).toContain('01.04.2026');
    expect(cells[3].textContent).toContain('PO-2026-1');
    expect(cells[4].textContent).toContain('357,00');
    expect(html.querySelector('[data-testid="direction-RE-1"]')?.textContent).toContain('Eingang');
    expect(html.querySelector('[data-testid="match-RE-1"]')?.textContent).toContain('offen');
    expect(html.querySelector('[data-testid="match-RE-2"]')?.textContent).toContain('Bestellung 7');
  });

  it('offers both syntaxes of the document: XRechnung (UBL) and ZUGFeRD (CII)', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([unmatched]);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.documentUrl(1, 'xrechnung')).toBe('/api/e-invoices/1/xrechnung');
    expect(component.documentUrl(7, 'cii')).toBe('/api/e-invoices/7/cii');

    const cell = fixture.nativeElement.querySelector('[data-testid="documents-RE-1"]') as HTMLElement;
    const links = Array.from(cell.querySelectorAll('a')).map((a) => [
      a.textContent!.trim(),
      a.getAttribute('href'),
    ]);
    expect(links).toEqual([
      ['XRechnung (UBL)', '/api/e-invoices/1/xrechnung'],
      ['ZUGFeRD (CII)', '/api/e-invoices/1/cii'],
    ]);
  });

  it('offers a match only for a received invoice that is still open', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([unmatched, matched, outbound]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    const button = html.querySelector('[data-testid="invoice-RE-1"] button');
    expect(button?.getAttribute('aria-label')).toBe('Rechnung RE-1 der Bestellung zuordnen');
    // Already matched: nothing left to do.
    expect(html.querySelector('[data-testid="invoice-RE-2"] button')).toBeNull();
    // Issued by us: a purchase order's receipt does not answer it, so it is not offered.
    expect(html.querySelector('[data-testid="invoice-RE-3"] button')).toBeNull();
    expect(html.querySelector('[data-testid="direction-RE-3"]')?.textContent).toContain('Ausgang');
    expect(html.querySelector('[data-testid="match-RE-3"]')?.textContent).toContain('—');
  });

  it('matches an invoice and reloads the list', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([unmatched]);
    fixture.detectChanges();

    fixture.componentInstance.match(unmatched);
    expect(fixture.componentInstance.matching()).toBe(1);

    const post = http.expectOne('/api/e-invoices/1/match');
    expect(post.request.method).toBe('POST');
    post.flush({ Number: 'RE-1' });

    expect(fixture.componentInstance.loading()).toBeTrue();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([matched]);
    fixture.detectChanges();

    expect(fixture.componentInstance.matching()).toBeNull();
    expect(fixture.componentInstance.actionError()).toBeNull();
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="match-RE-2"]')?.textContent).toContain('Bestellung 7');
  });

  it('keeps the list and names the invoice when a match fails', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([unmatched]);
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
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([]);
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
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([unmatched]);
    fixture.detectChanges();

    expect(fixture.componentInstance.invoices().length).toBe(1);
    expect(fixture.componentInstance.actionError()).toBeNull();
  });

  it('refuses to read in nothing, and reports a refused document', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([]);
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
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([]);
    fixture.detectChanges();
    let html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('table')).toBeNull();
    expect(html.querySelector('[role="status"]')?.textContent).toContain('keine Rechnungen erfasst');

    const failing = TestBed.createComponent(EInvoicesComponent);
    failing.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush('nope', { status: 500, statusText: 'Server Error' });
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

    http.expectOne('/api/e-invoices?limit=50&offset=0').flush(null);
    fixture.detectChanges();

    expect(component.invoices()).toEqual([]);
    expect(component.loading()).toBeFalse();
    expect(component.loadError()).toBeNull();
  });

  it('formats dates and amounts the way the screen shows them', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([]);

    expect(component.direction('inbound')).toBe('Eingang');
    expect(component.direction('outbound')).toBe('Ausgang');
    expect(component.direction('')).toBe('');
    expect(component.day('2026-04-01T00:00:00Z')).toBe('01.04.2026');
    // An unparseable date is shown as it came, rather than as "Invalid Date".
    expect(component.day('nonsense')).toBe('nonsense');
    expect(component.euro(35700)).toBe('357,00');
    expect(component.euro(0)).toBe('0,00');
  });

  it('opens with one page and offers the next only while the pages come back full', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();

    // Nothing has been read yet, so there is no next page to offer and none is being read. A screen
    // that claimed otherwise would show a button that asks for a page it cannot know exists.
    expect(component.hasMore()).toBeFalse();
    expect(component.loadingMore()).toBeFalse();

    // The first page is asked for explicitly: the screen's page size, from the beginning.
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    expect(component.invoices().length).toBe(50);
    expect(component.hasMore()).toBeTrue();
    const host = fixture.nativeElement as HTMLElement;
    expect(host.textContent).toContain('Weitere laden');
  });

  it('reads the next page from where the last one ended and appends it', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    component.loadMore();

    expect(component.loadingMore()).toBeTrue();
    // The offset is the number of rows already read, so the pages continue rather than repeat.
    const second = http.expectOne('/api/e-invoices?limit=50&offset=50');
    fixture.detectChanges();
    const button = (fixture.nativeElement as HTMLElement).querySelector(
      '[data-testid="load-more"]',
    ) as HTMLButtonElement;
    expect(button.disabled).toBeTrue();
    expect(button.textContent).toContain('Weitere werden geladen');

    second.flush(fullPage(51).slice(0, 2));
    fixture.detectChanges();

    expect(component.loadingMore()).toBeFalse();
    expect(component.invoices().length).toBe(52);
    expect(component.invoices()[0].Number).toBe('RE-1');
    expect(component.invoices()[51].Number).toBe('RE-52');
    // A short page is the end of the list, so the button goes away.
    expect(component.hasMore()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).textContent).not.toContain('Weitere laden');
  });

  it('keeps offering the next page while the pages keep coming back full', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    component.loadMore();
    // A second *full* page means there may be a third, so the offer stays.
    http.expectOne('/api/e-invoices?limit=50&offset=50').flush(fullPage(51));
    fixture.detectChanges();

    expect(component.invoices().length).toBe(100);
    expect(component.hasMore()).toBeTrue();

    component.loadMore();
    expect(component.loadingMore()).toBeTrue();
    http.expectOne('/api/e-invoices?limit=50&offset=100').flush(null);
    fixture.detectChanges();

    // An empty page appends nothing: the rows read are the rows on screen, whatever the server says.
    expect(component.invoices().length).toBe(100);
    expect(component.hasMore()).toBeFalse();
    expect(component.loadingMore()).toBeFalse();
  });

  it('does nothing when there is no next page, or one is already being read', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush([unmatched]);
    fixture.detectChanges();

    // One short page: there is nothing to load, so no request is made.
    expect(component.hasMore()).toBeFalse();
    component.loadMore();
    expect(component.loadingMore()).toBeFalse();
  });

  it('keeps the invoices it has when the next page cannot be read', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    component.loadMore();
    http
      .expectOne('/api/e-invoices?limit=50&offset=50')
      .flush({ error: 'unreadable' }, { status: 500, statusText: 'Internal Server Error' });
    fixture.detectChanges();

    // What was read is still on screen and still valid; only the page is reported as missing.
    expect(component.loadingMore()).toBeFalse();
    expect(component.invoices().length).toBe(50);
    expect(component.actionError()).toBe('Konnte die weiteren Rechnungen nicht laden.');
    expect(component.loadError()).toBeNull();
  });

  it('reports an unreadable first page and offers no next one', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http
      .expectOne('/api/e-invoices?limit=50&offset=0')
      .flush({ error: 'unreadable' }, { status: 500, statusText: 'Internal Server Error' });
    fixture.detectChanges();

    expect(component.loadError()).toBe('Konnte die Rechnungen nicht laden.');
    expect(component.hasMore()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).textContent).not.toContain('Weitere laden');
  });

  it('treats an empty first page as no invoices and offers no next one', () => {
    const fixture = TestBed.createComponent(EInvoicesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/e-invoices?limit=50&offset=0').flush(null);
    fixture.detectChanges();

    expect(component.invoices()).toEqual([]);
    expect(component.hasMore()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Es sind keine Rechnungen erfasst.');
  });
});
