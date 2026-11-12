import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { PurchaseOrdersComponent } from './orders.component';
import { ProcurementOrderSummary } from '../core/api/generated';

describe('PurchaseOrdersComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PurchaseOrdersComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const order = (ID: number, Number: string, Status: string, GrossCents: number): ProcurementOrderSummary => ({
    ID, Number, Supplier: 'Lieferant GmbH', BudgetPositionID: 1, Currency: 'EUR',
    NetCents: GrossCents, VatCents: 0, GrossCents, InvoicedCents: 0, Status,
  });
  const draft = order(1, 'PO-1', 'draft', 11900);
  const approved = order(2, 'PO-2', 'approved', 23800);

  /** A page's worth of orders, so the screen believes there may be another. */
  const fullPage = (from: number, status = 'draft'): ProcurementOrderSummary[] =>
    Array.from({ length: 50 }, (_, i) => order(from + i, `PO-${from + i}`, status, 11900));

  it('loads the orders as soon as it is shown', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    // Before the API answers: loading, nothing listed, no error.
    expect(fixture.componentInstance.loading()).toBeTrue();
    expect(fixture.componentInstance.orders()).toEqual([]);
    expect(fixture.componentInstance.loadError()).toBeNull();
    fixture.detectChanges();

    const req = http.expectOne('/api/purchase-orders?limit=50&offset=0');
    expect(req.request.method).toBe('GET');
    req.flush([draft, approved]);
    fixture.detectChanges();

    expect(fixture.componentInstance.orders().length).toBe(2);
    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.loadError()).toBeNull();
  });

  it('says how much of an order has been billed, so a part-billed order is not mistaken for an open one', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    const billed = (ID: number, Number: string, GrossCents: number, InvoicedCents: number) => ({
      ...order(ID, Number, 'approved', GrossCents),
      InvoicedCents,
    });
    http
      .expectOne('/api/purchase-orders?limit=50&offset=0')
      .flush([billed(1, 'PO-1', 11900, 0), billed(2, 'PO-2', 11900, 5000), billed(3, 'PO-3', 11900, 11900)]);
    fixture.detectChanges();

    const cell = (number: string): string =>
      ((fixture.nativeElement as HTMLElement).querySelector(`[data-testid="billed-${number}"]`)?.textContent ?? '').trim();
    // Nothing billed yet, a part of it, and all of it — three different facts, three different words.
    expect(cell('PO-1')).toBe('—');
    expect(cell('PO-2')).toBe('50,00 von 119,00');
    expect(cell('PO-3')).toBe('119,00 (vollständig)');
  });

  it('renders each order with its amount and the German status word', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush([draft, approved]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    const cells = html.querySelectorAll('[data-testid="order-PO-1"] td');
    expect(cells[0].textContent).toContain('PO-1');
    expect(cells[1].textContent).toContain('Lieferant GmbH');
    expect(cells[2].textContent).toContain('119,00');
    expect(html.querySelector('[data-testid="status-PO-1"]')?.textContent).toContain('Entwurf');
    expect(html.querySelector('[data-testid="status-PO-2"]')?.textContent).toContain('Genehmigt');
  });

  it('asks the API for the chosen status instead of sifting what it has', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush([draft, approved]);
    fixture.detectChanges();
    const component = fixture.componentInstance;

    component.filterTo('approved');

    // The filter travels to the server, so the screen works on a list it never holds in full.
    const filtered = http.expectOne('/api/purchase-orders?status=approved&limit=50&offset=0');
    expect(filtered.request.method).toBe('GET');
    filtered.flush([approved]);
    fixture.detectChanges();

    expect(component.status()).toBe('approved');
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="order-PO-2"]')).toBeTruthy();
    expect(html.querySelector('[data-testid="order-PO-1"]')).toBeNull();

    // Back to everything: no filter parameter at all.
    component.filterTo('');
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush([draft, approved]);
    fixture.detectChanges();
    expect(component.orders().length).toBe(2);
  });

  it('says so when there are no orders, and when the chosen status has none', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush([]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('table')).toBeNull();
    expect(html.querySelector('[role="status"]')?.textContent).toContain('keine Bestellungen angelegt');

    // With a status chosen and nothing loaded, the message names the status.
    fixture.componentInstance.filterTo('cancelled');
    http.expectOne('/api/purchase-orders?status=cancelled&limit=50&offset=0').flush([]);
    fixture.detectChanges();
    expect(html.querySelector('[role="status"]')?.textContent).toContain('Keine Bestellung in diesem Status');
  });

  it('reports a failed load instead of showing an empty list', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Bestellungen nicht laden.');
    expect(component.loading()).toBeFalse();
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[role="alert"]')).toBeTruthy();
    expect(html.querySelector('[role="status"]')).toBeNull();
  });

  it('approves a draft and reloads the list', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush([draft]);
    fixture.detectChanges();

    fixture.componentInstance.approve(draft);
    expect(fixture.componentInstance.approving()).toBe(1);

    const post = http.expectOne('/api/purchase-orders/1/approve');
    expect(post.request.method).toBe('POST');
    post.flush({ status: 'approved' });

    // The screen reloads so the row shows its new state.
    expect(fixture.componentInstance.loading()).toBeTrue();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush([approved]);
    fixture.detectChanges();

    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.approving()).toBeNull();
    expect(fixture.componentInstance.actionError()).toBeNull();
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="status-PO-2"]')?.textContent).toContain('Genehmigt');
  });

  it('keeps the list and names the order when an approval fails', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush([draft]);
    fixture.detectChanges();

    fixture.componentInstance.approve(draft);
    http.expectOne('/api/purchase-orders/1/approve').flush('no', { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    expect(fixture.componentInstance.actionError()).toBe('Konnte die Bestellung PO-1 nicht genehmigen.');
    expect(fixture.componentInstance.approving()).toBeNull();
    // The list is still there, and the failed action did not become a load error.
    expect(fixture.componentInstance.loadError()).toBeNull();
    expect(html.querySelector('[data-testid="order-PO-1"]')).toBeTruthy();
    expect(html.querySelector('table')).toBeTruthy();
  });

  it('offers approval only for a draft', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush([draft, approved]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    const draftButton = html.querySelector('[data-testid="order-PO-1"] button');
    expect(draftButton?.getAttribute('aria-label')).toBe('Bestellung PO-1 genehmigen');
    expect(html.querySelector('[data-testid="order-PO-2"] button')).toBeNull();
  });

  it('treats an empty response body as a list without orders', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();

    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush(null);
    fixture.detectChanges();

    expect(component.orders()).toEqual([]);
    expect(component.loadError()).toBeNull();
    expect(component.loading()).toBeFalse();
  });

  it('names the lifecycle states and formats amounts', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush([]);

    expect(component.statusLabel('draft')).toBe('Entwurf');
    expect(component.statusLabel('approved')).toBe('Genehmigt');
    expect(component.statusLabel('received')).toBe('Erhalten');
    expect(component.statusLabel('cancelled')).toBe('Storniert');
    expect(component.statusLabel('something-else')).toBe('something-else');
    expect(component.euro(11900)).toBe('119,00');
    expect(component.euro(0)).toBe('0,00');
  });

  it('opens with one page and offers the next only while the pages come back full', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    const component = fixture.componentInstance;

    // Before anything is read at all: no next page to offer, none being read. This is asserted before
    // the first `detectChanges()`, because the first load resets the offer — asserting afterwards
    // would test the reset rather than the screen's opening state.
    expect(component.hasMore()).toBeFalse();
    expect(component.loadingMore()).toBeFalse();

    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    expect(component.orders().length).toBe(50);
    expect(component.hasMore()).toBeTrue();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Weitere laden');
  });

  it('reads the next page of the chosen status and appends it', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush(fullPage(1, 'approved'));
    fixture.detectChanges();

    component.loadMore();

    expect(component.loadingMore()).toBeTrue();
    // The page carries the filter too, so a page of one status cannot be read for another.
    const second = http.expectOne('/api/purchase-orders?limit=50&offset=50');
    fixture.detectChanges();
    const button = (fixture.nativeElement as HTMLElement).querySelector(
      '[data-testid="load-more"]',
    ) as HTMLButtonElement;
    expect(button.disabled).toBeTrue();
    expect(button.textContent).toContain('Weitere werden geladen');

    second.flush(fullPage(51, 'approved'));
    fixture.detectChanges();

    expect(component.orders().length).toBe(100);
    expect(component.orders()[0].Number).toBe('PO-1');
    // A second full page means there may be a third, so the offer stays.
    expect(component.hasMore()).toBeTrue();
    expect(component.loadingMore()).toBeFalse();
  });

  it('stops offering the next page when a page comes back empty', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    component.loadMore();
    http.expectOne('/api/purchase-orders?limit=50&offset=50').flush(null);
    fixture.detectChanges();

    // An empty page appends nothing: the rows read are the rows on screen, whatever the server says.
    expect(component.orders().length).toBe(50);
    expect(component.hasMore()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).textContent).not.toContain('Weitere laden');

    component.loadMore();
    expect(component.loadingMore()).toBeFalse();
  });

  it('carries the chosen status into the next page', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();
    component.filterTo('approved');
    http.expectOne('/api/purchase-orders?status=approved&limit=50&offset=0').flush(fullPage(1, 'approved'));
    fixture.detectChanges();

    component.loadMore();

    // The filter travels with the page: a page of one status must never be read without it, or the
    // offset would count rows of a different list.
    http.expectOne('/api/purchase-orders?status=approved&limit=50&offset=50').flush([]);
    fixture.detectChanges();

    expect(component.orders().length).toBe(50);
    expect(component.hasMore()).toBeFalse();
  });

  it('keeps the orders it has when the next page cannot be read, and does not read it twice', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    component.loadMore();
    component.loadMore();
    http
      .expectOne('/api/purchase-orders?limit=50&offset=50')
      .flush({ error: 'unreadable' }, { status: 500, statusText: 'Internal Server Error' });
    fixture.detectChanges();

    expect(component.orders().length).toBe(50);
    expect(component.loadingMore()).toBeFalse();
    expect(component.actionError()).toBe('Konnte die weiteren Bestellungen nicht laden.');
    expect(component.loadError()).toBeNull();
    // The offer stays: the page failed, so it is still there to be read.
    expect(component.hasMore()).toBeTrue();
  });

  it('clears the offer when another status is chosen', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders?limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();
    expect(component.hasMore()).toBeTrue();

    component.filterTo('approved');

    // The other status starts from its own first page: a stale offer would ask for a page of the
    // list that is no longer on screen.
    expect(component.hasMore()).toBeFalse();
    http.expectOne('/api/purchase-orders?status=approved&limit=50&offset=0').flush([approved]);
    fixture.detectChanges();

    expect(component.orders().length).toBe(1);
    expect(component.hasMore()).toBeFalse();
  });
});
