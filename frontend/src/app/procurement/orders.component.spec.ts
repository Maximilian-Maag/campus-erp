import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { PurchaseOrdersComponent } from './orders.component';
import { OrderSummary } from '../core/api/generated';

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

  const order = (ID: number, Number: string, Status: string, GrossCents: number): OrderSummary => ({
    ID, Number, Supplier: 'Lieferant GmbH', BudgetPositionID: 1, Currency: 'EUR',
    NetCents: GrossCents, VatCents: 0, GrossCents, Status,
  });
  const draft = order(1, 'PO-1', 'draft', 11900);
  const approved = order(2, 'PO-2', 'approved', 23800);

  it('loads the orders as soon as it is shown', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    // Before the API answers: loading, nothing listed, no error.
    expect(fixture.componentInstance.loading()).toBeTrue();
    expect(fixture.componentInstance.orders()).toEqual([]);
    expect(fixture.componentInstance.loadError()).toBeNull();
    fixture.detectChanges();

    const req = http.expectOne('/api/purchase-orders');
    expect(req.request.method).toBe('GET');
    req.flush([draft, approved]);
    fixture.detectChanges();

    expect(fixture.componentInstance.orders().length).toBe(2);
    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.loadError()).toBeNull();
  });

  it('renders each order with its amount and the German status word', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders').flush([draft, approved]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    const cells = html.querySelectorAll('[data-testid="order-PO-1"] td');
    expect(cells[0].textContent).toContain('PO-1');
    expect(cells[1].textContent).toContain('Lieferant GmbH');
    expect(cells[2].textContent).toContain('119,00');
    expect(html.querySelector('[data-testid="status-PO-1"]')?.textContent).toContain('Entwurf');
    expect(html.querySelector('[data-testid="status-PO-2"]')?.textContent).toContain('Genehmigt');
  });

  it('filters to one status without asking the API again', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders').flush([draft, approved]);
    fixture.detectChanges();
    const component = fixture.componentInstance;

    component.status.set('approved');
    fixture.detectChanges();

    expect(component.visible().map((o) => o.Number)).toEqual(['PO-2']);
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="order-PO-2"]')).toBeTruthy();
    expect(html.querySelector('[data-testid="order-PO-1"]')).toBeNull();
    // Still one request in total: the filter is a view over what was loaded.
    http.expectNone('/api/purchase-orders');

    // And back to everything.
    component.status.set('');
    fixture.detectChanges();
    expect(component.visible().length).toBe(2);
  });

  it('says so when there are no orders, and when the chosen status has none', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders').flush([]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('table')).toBeNull();
    expect(html.querySelector('[role="status"]')?.textContent).toContain('keine Bestellungen angelegt');

    // With a status chosen and nothing loaded, the message names the status.
    fixture.componentInstance.status.set('cancelled');
    fixture.detectChanges();
    expect(html.querySelector('[role="status"]')?.textContent).toContain('Keine Bestellung in diesem Status');
  });

  it('reports a failed load instead of showing an empty list', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/purchase-orders').flush('nope', { status: 500, statusText: 'Server Error' });
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
    http.expectOne('/api/purchase-orders').flush([draft]);
    fixture.detectChanges();

    fixture.componentInstance.approve(draft);
    expect(fixture.componentInstance.approving()).toBe(1);

    const post = http.expectOne('/api/purchase-orders/1/approve');
    expect(post.request.method).toBe('POST');
    post.flush({ status: 'approved' });

    // The screen reloads so the row shows its new state.
    expect(fixture.componentInstance.loading()).toBeTrue();
    http.expectOne('/api/purchase-orders').flush([approved]);
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
    http.expectOne('/api/purchase-orders').flush([draft]);
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
    http.expectOne('/api/purchase-orders').flush([draft, approved]);
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

    http.expectOne('/api/purchase-orders').flush(null);
    fixture.detectChanges();

    expect(component.orders()).toEqual([]);
    expect(component.visible()).toEqual([]);
    expect(component.loadError()).toBeNull();
    expect(component.loading()).toBeFalse();
  });

  it('names the lifecycle states and formats amounts', () => {
    const fixture = TestBed.createComponent(PurchaseOrdersComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    http.expectOne('/api/purchase-orders').flush([]);

    expect(component.statusLabel('draft')).toBe('Entwurf');
    expect(component.statusLabel('approved')).toBe('Genehmigt');
    expect(component.statusLabel('received')).toBe('Erhalten');
    expect(component.statusLabel('cancelled')).toBe('Storniert');
    expect(component.statusLabel('something-else')).toBe('something-else');
    expect(component.euro(11900)).toBe('119,00');
    expect(component.euro(0)).toBe('0,00');
  });
});
