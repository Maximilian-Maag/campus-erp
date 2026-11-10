import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { GoodsReceiptsComponent } from './goods-receipts.component';
import { ProcurementOrderSummary, ReceiptResult, ReceiptState } from '../core/api/generated';

describe('GoodsReceiptsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [GoodsReceiptsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const order = (over: Partial<ProcurementOrderSummary> = {}): ProcurementOrderSummary => ({
    ID: 7,
    Number: 'PO-1',
    Supplier: 'Lieferant GmbH',
    BudgetPositionID: 1,
    Currency: 'EUR',
    NetCents: 11000,
    VatCents: 2090,
    GrossCents: 13090,
    Status: 'approved',
    ...over,
  });

  const state = (over: Partial<ReceiptState> = {}): ReceiptState => ({
    OrderID: 7,
    Complete: false,
    Progress: [
      { LineNo: 1, Description: 'Ware', OrderedMilli: 1000, ReceivedMilli: 0 },
      { LineNo: 2, Description: 'Versand', OrderedMilli: 2000, ReceivedMilli: 0 },
    ],
    Receipts: [],
    ...over,
  });

  let fixture: ComponentFixture<GoodsReceiptsComponent>;
  let component: GoodsReceiptsComponent;
  let el: HTMLElement;

  /** Creates the screen and answers the orders read with the given orders. */
  function screen(orders: ProcurementOrderSummary[] = [order()]): void {
    fixture = TestBed.createComponent(GoodsReceiptsComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    el = fixture.nativeElement as HTMLElement;
    http.expectOne('/api/purchase-orders?status=approved').flush(orders);
    fixture.detectChanges();
  }

  /** Chooses an order and answers the arrival read with the given state. */
  function chooseWith(id: number, state: ReceiptState): void {
    component.choose(String(id));
    http.expectOne(`/api/purchase-orders/${id}/receipts`).flush(state);
    fixture.detectChanges();
  }

  it('loads the approved orders and starts with nothing chosen', () => {
    screen();
    expect(component.orders().length).toBe(1);
    expect(component.loadingOrders()).toBeFalse();
    expect(component.order()).toBe('');
    expect(component.state()).toBeNull();
    expect(component.openLines()).toEqual([]);
    expect(el.textContent).toContain('PO-1');
    expect(el.querySelector('select')).not.toBeNull();
  });

  it('reports a failed orders read and offers no picker', () => {
    fixture = TestBed.createComponent(GoodsReceiptsComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    el = fixture.nativeElement as HTMLElement;
    http.expectOne('/api/purchase-orders?status=approved').flush(
      { error: 'boom' },
      { status: 500, statusText: 'Server Error' }
    );
    fixture.detectChanges();

    expect(component.loadError()).toBe('Die Bestellungen konnten nicht geladen werden.');
    expect(component.loadingOrders()).toBeFalse();
    expect(el.querySelector('select')).toBeNull();
  });

  it('states an empty approved list', () => {
    screen([]);
    expect(el.textContent).toContain('keine freigegebene Bestellung offen');
    expect(component.loadError()).toBeNull();
  });

  it('reads the arrival state of the chosen order and shows every quantity', () => {
    screen();
    chooseWith(7, state({ Progress: [{ LineNo: 1, Description: 'Ware', OrderedMilli: 1000, ReceivedMilli: 500 }] }));

    expect(component.order()).toBe('7');
    expect(component.selected()?.Number).toBe('PO-1');
    const row = el.querySelector('[data-testid="line-1"]')!;
    expect(row.textContent).toContain('Ware');
    expect(row.textContent).toContain('0,5');
    expect(el.textContent).toContain('Bestellung PO-1 von Lieferant GmbH');
    expect(el.textContent).toContain('130,90');
  });

  it('reports a failed arrival read', () => {
    screen();
    component.choose('7');
    http.expectOne('/api/purchase-orders/7/receipts').flush(
      { error: 'boom' },
      { status: 500, statusText: 'Server Error' }
    );
    expect(component.loadError()).toBe('Der Wareneingang konnte nicht geladen werden.');
  });

  it('clears the state and asks nothing when the choice is emptied', () => {
    screen();
    chooseWith(7, state());
    component.choose('');
    http.expectNone('/api/purchase-orders//receipts');
    expect(component.state()).toBeNull();
    expect(component.reference()).toBe('');
    expect(component.quantities()).toEqual({});
  });

  it('books the quantities that arrived, in milli-units', () => {
    screen();
    chooseWith(7, state());
    component.reference.set('LS-1');
    component.receivedOn.set('2026-11-20');
    component.setQuantity(1, '0,5');
    component.setQuantity(2, '2');
    component.record(new Event('submit'));

    expect(component.busy()).toBeTrue();
    const req = http.expectOne('/api/purchase-orders/7/receipts');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({
      reference: 'LS-1',
      received_on: '2026-11-20',
      lines: [
        { line_no: 1, quantity_milli: 500 },
        { line_no: 2, quantity_milli: 2000 },
      ],
    });
    const result: ReceiptResult = {
      Receipt: { ID: 3, OrderID: 7, Reference: 'LS-1', ReceivedOn: '2026-11-20T00:00:00Z', ReceivedBy: 'carol', Lines: [{ LineNo: 1, QuantityMilli: 500 }], ReceivedCents: 5950 },
      State: state({ Progress: [{ LineNo: 1, Description: 'Ware', OrderedMilli: 1000, ReceivedMilli: 500 }] }),
    };
    req.flush(result);
    // An order may have left the approved list, so the picker is read again.
    http.expectOne('/api/purchase-orders?status=approved').flush([order()]);
    fixture.detectChanges();

    expect(component.busy()).toBeFalse();
    expect(component.notice()).toBe('Wareneingang gebucht (59,50).');
    expect(component.state()?.Progress[0].ReceivedMilli).toBe(500);
    expect(component.reference()).toBe('');
    expect(component.quantities()).toEqual({});
  });

  it('says so when the delivery completes the order', () => {
    screen();
    chooseWith(7, state());
    component.reference.set('LS-1');
    component.receivedOn.set('2026-11-20');
    component.setQuantity(1, '1');
    component.record(new Event('submit'));
    http.expectOne('/api/purchase-orders/7/receipts').flush({
      Receipt: { ID: 3, OrderID: 7, Reference: 'LS-1', ReceivedOn: '2026-11-20T00:00:00Z', ReceivedBy: 'carol', Lines: [], ReceivedCents: 13090 },
      State: state({
        Complete: true,
        Progress: [
          { LineNo: 1, Description: 'Ware', OrderedMilli: 1000, ReceivedMilli: 1000 },
          { LineNo: 2, Description: 'Versand', OrderedMilli: 2000, ReceivedMilli: 2000 },
        ],
      }),
    } satisfies ReceiptResult);
    http.expectOne('/api/purchase-orders?status=approved').flush([]);
    fixture.detectChanges();

    expect(component.notice()).toBe('Wareneingang gebucht (130,90). Die Bestellung ist vollständig geliefert.');
    expect(component.openLines()).toEqual([]);
  });

  it('reports a refused booking and keeps what was typed', () => {
    screen();
    chooseWith(7, state());
    component.reference.set('LS-1');
    component.receivedOn.set('2026-11-20');
    component.setQuantity(1, '1');
    component.record(new Event('submit'));
    http.expectOne('/api/purchase-orders/7/receipts').flush(
      { error: 'the receipt exceeds the ordered quantity' },
      { status: 422, statusText: 'Unprocessable Entity' }
    );
    fixture.detectChanges();

    expect(component.busy()).toBeFalse();
    expect(component.actionError()).toBe('Der Wareneingang konnte nicht gebucht werden.');
    expect(component.notice()).toBeNull();
    expect(component.reference()).toBe('LS-1');
    expect(component.quantities()[1]).toBe('1');
    expect(el.textContent).toContain('Der Wareneingang konnte nicht gebucht werden.');
  });

  it('refuses an incomplete booking before asking the API', () => {
    screen();
    chooseWith(7, state());

    component.record(new Event('submit'));
    expect(component.actionError()).toBe('Ein Lieferschein ist erforderlich.');

    component.reference.set('LS-1');
    component.receivedOn.set('20.11.2026');
    component.record(new Event('submit'));
    expect(component.actionError()).toBe('Ein Datum ist erforderlich.');

    component.receivedOn.set('2026-11-20');
    component.record(new Event('submit'));
    expect(component.actionError()).toBe('Es ist keine Menge erfasst.');

    component.setQuantity(1, 'zwei');
    component.record(new Event('submit'));
    expect(component.actionError()).toBe('Die Menge der Position 1 ist keine gültige Menge.');

    component.setQuantity(1, '0');
    component.record(new Event('submit'));
    expect(component.actionError()).toBe('Die Menge der Position 1 ist keine gültige Menge.');

    http.expectNone('/api/purchase-orders/7/receipts');
    expect(component.busy()).toBeFalse();
  });

  it('refuses a booking with no order chosen', () => {
    screen();
    component.reference.set('LS-1');
    component.receivedOn.set('2026-11-20');
    component.setQuantity(1, '1');
    component.record(new Event('submit'));

    expect(component.actionError()).toBe('Es ist keine Bestellung gewählt.');
    http.expectNone('/api/purchase-orders/0/receipts');
  });

  it('ignores a blank quantity line and books only the filled ones', () => {
    screen();
    chooseWith(7, state());
    component.reference.set('LS-1');
    component.receivedOn.set('2026-11-20');
    component.setQuantity(1, '   ');
    component.setQuantity(2, '1,5');
    component.record(new Event('submit'));

    const req = http.expectOne('/api/purchase-orders/7/receipts');
    expect(req.request.body.lines).toEqual([{ line_no: 2, quantity_milli: 1500 }]);
    req.flush({
      Receipt: { ID: 3, OrderID: 7, Reference: 'LS-1', ReceivedOn: '2026-11-20T00:00:00Z', ReceivedBy: 'carol', Lines: [], ReceivedCents: 595 },
      State: state(),
    } satisfies ReceiptResult);
    http.expectOne('/api/purchase-orders?status=approved').flush([order()]);
  });

  it('offers no form once the order is complete and lists the receipts', () => {
    screen();
    chooseWith(7, state({
      Complete: true,
      Progress: [{ LineNo: 1, Description: 'Ware', OrderedMilli: 1000, ReceivedMilli: 1000 }],
      Receipts: [
        { ID: 3, OrderID: 7, Reference: 'LS-1', ReceivedOn: '2026-11-20T00:00:00Z', ReceivedBy: 'carol', Lines: [{ LineNo: 1, QuantityMilli: 1000 }], ReceivedCents: 11900 },
        { ID: 4, OrderID: 7, Reference: 'LS-2', ReceivedOn: '2026-11-25T00:00:00Z', ReceivedBy: 'carol', Lines: [{ LineNo: 1, QuantityMilli: 500 }, { LineNo: 2, QuantityMilli: 2000 }], ReceivedCents: 1190 },
      ],
    }));

    expect(el.textContent).toContain('Die Bestellung ist vollständig geliefert.');
    expect(el.querySelector('form')).toBeNull();
    expect(component.openLines()).toEqual([]);
    const receipt = el.querySelector('[data-testid="receipt-4"]')!;
    expect(receipt.textContent).toContain('LS-2');
    expect(receipt.textContent).toContain('25.11.2026');
    expect(receipt.textContent).toContain('1: 0,5; 2: 2');
  });

  it('offers an input only for the lines that are still open', () => {
    screen();
    chooseWith(7, state({
      Progress: [
        { LineNo: 1, Description: 'Ware', OrderedMilli: 1000, ReceivedMilli: 1000 },
        { LineNo: 2, Description: 'Versand', OrderedMilli: 2000, ReceivedMilli: 500 },
      ],
    }));

    expect(el.querySelector('[data-testid="quantity-1"]')).toBeNull();
    expect(el.querySelector('[data-testid="quantity-2"]')).not.toBeNull();
    expect(component.openLines().map((p) => p.LineNo)).toEqual([2]);
  });

  it('derives the open quantity and renders quantities, amounts and days', () => {
    screen();
    expect(component.open({ OrderedMilli: 1000, ReceivedMilli: 400 })).toBe(600);
    expect(component.open({ OrderedMilli: 1000, ReceivedMilli: 1000 })).toBe(0);
    expect(component.open({ OrderedMilli: 1000, ReceivedMilli: 1200 })).toBe(0);
    expect(component.quantity(1500)).toBe('1,5');
    expect(component.amount(13090)).toBe('130,90');
    expect(component.day('2026-11-20T00:00:00Z')).toBe('20.11.2026');
    expect(component.lineList({ Lines: [] })).toBe('');
  });
});
