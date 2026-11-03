import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { PaymentOrdersComponent } from './payment-orders.component';
import { KasseOrderSummary } from '../core/api/generated';

describe('PaymentOrdersComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PaymentOrdersComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const order = (ID: number, Reference: string, Status: string): KasseOrderSummary => ({
    ID, Reference, AmountCents: 125000, Currency: 'EUR', BudgetPositionID: 1,
    Purpose: 'Stromabschlag', DueDate: '2026-04-01T00:00:00Z', Status,
  });
  const draft = order(1, 'AO-1', 'draft');
  const submitted = order(2, 'AO-2', 'submitted');
  const approved = order(3, 'AO-3', 'approved');

  /** The screen with its list already loaded. */
  function loaded(orders: KasseOrderSummary[] = [draft, submitted]) {
    const fixture = TestBed.createComponent(PaymentOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/payment-orders').flush(orders);
    fixture.detectChanges();
    return fixture;
  }

  it('starts with an unloaded, empty screen', () => {
    const fixture = TestBed.createComponent(PaymentOrdersComponent);
    const component = fixture.componentInstance;

    expect(component.orders()).toEqual([]);
    expect(component.status()).toBe('');
    expect(component.loading()).toBeTrue();
    expect(component.loadError()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.busy()).toBeNull();
    expect(component.rejecting()).toBeNull();
    expect(component.reason()).toBe('');
  });

  it('loads the orders as soon as it is shown', () => {
    const fixture = loaded([draft, approved]);
    expect(fixture.componentInstance.orders().length).toBe(2);
    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.loadError()).toBeNull();
  });

  it('renders each order with purpose, amount, due date and the German state word', () => {
    const fixture = loaded([draft, submitted, approved]);
    const html = fixture.nativeElement as HTMLElement;

    const cells = html.querySelectorAll('[data-testid="order-AO-1"] td');
    expect(cells[0].textContent).toContain('AO-1');
    expect(cells[1].textContent).toContain('Stromabschlag');
    expect(cells[2].textContent).toContain('1.250,00');
    expect(cells[3].textContent).toContain('01.04.2026');
    expect(html.querySelector('[data-testid="status-AO-1"]')?.textContent).toContain('Entwurf');
    expect(html.querySelector('[data-testid="status-AO-2"]')?.textContent).toContain('Vorgelegt');
    expect(html.querySelector('[data-testid="status-AO-3"]')?.textContent).toContain('Genehmigt');
  });

  it('offers only the actions a state allows', () => {
    const fixture = loaded([draft, submitted, approved]);
    const html = fixture.nativeElement as HTMLElement;

    // A draft may be submitted or cancelled, but not approved or rejected.
    const draftButtons = Array.from(html.querySelectorAll('[data-testid="order-AO-1"] button')).map((b) => b.textContent?.trim());
    expect(draftButtons).toEqual(['Vorlegen', 'Stornieren']);

    // A submitted order may be approved, rejected or cancelled — not submitted again.
    const submittedButtons = Array.from(html.querySelectorAll('[data-testid="order-AO-2"] button')).map((b) => b.textContent?.trim());
    expect(submittedButtons).toEqual(['Genehmigen', 'Ablehnen', 'Stornieren']);

    // A decided order offers nothing.
    expect(html.querySelectorAll('[data-testid="order-AO-3"] button').length).toBe(0);
  });

  it('asks the API for the chosen status instead of sifting what it has', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    component.filterTo('submitted');
    const filtered = http.expectOne('/api/payment-orders?status=submitted');
    expect(filtered.request.method).toBe('GET');
    filtered.flush([submitted]);
    fixture.detectChanges();

    expect(component.status()).toBe('submitted');
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="order-AO-2"]')).toBeTruthy();
    expect(html.querySelector('[data-testid="order-AO-1"]')).toBeNull();

    // Back to everything: no filter parameter at all.
    component.filterTo('');
    http.expectOne('/api/payment-orders').flush([draft, submitted]);
    fixture.detectChanges();
    expect(component.orders().length).toBe(2);
  });

  it('says so when there are no orders, and when the chosen status has none', () => {
    const fixture = loaded([]);
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('table')).toBeNull();
    expect(html.querySelector('[role="status"]')?.textContent).toContain('keine Zahlungsanordnungen angelegt');

    fixture.componentInstance.filterTo('cancelled');
    http.expectOne('/api/payment-orders?status=cancelled').flush([]);
    fixture.detectChanges();
    expect(html.querySelector('[role="status"]')?.textContent).toContain('Keine Anordnung in diesem Status');
  });

  it('reports a failed load instead of showing an empty list', () => {
    const fixture = TestBed.createComponent(PaymentOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/payment-orders').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Zahlungsanordnungen nicht laden.');
    expect(component.loading()).toBeFalse();
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[role="alert"]')).toBeTruthy();
    expect(html.querySelector('[role="status"]')).toBeNull();
  });

  it('treats an empty response body as a list without orders', () => {
    const fixture = TestBed.createComponent(PaymentOrdersComponent);
    fixture.detectChanges();
    http.expectOne('/api/payment-orders').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.orders()).toEqual([]);
    expect(component.loadError()).toBeNull();
    expect(component.loading()).toBeFalse();
  });

  it('submits a draft and reloads', () => {
    const fixture = loaded([draft]);
    const component = fixture.componentInstance;

    component.submit(draft);
    expect(component.busy()).toBe(1);

    const post = http.expectOne('/api/payment-orders/1/submit');
    expect(post.request.method).toBe('POST');
    post.flush({ ID: 1, Status: 'submitted' });

    // The screen reloads so the row shows its new state.
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/payment-orders').flush([submitted]);
    fixture.detectChanges();

    expect(component.busy()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect((fixture.nativeElement as HTMLElement).querySelector('[data-testid="status-AO-2"]')?.textContent).toContain('Vorgelegt');
  });

  it('approves a submitted order and reloads', () => {
    const fixture = loaded([submitted]);
    const component = fixture.componentInstance;

    component.approve(submitted);
    http.expectOne('/api/payment-orders/2/approve').flush({ ID: 2, Status: 'approved' });
    http.expectOne('/api/payment-orders').flush([approved]);
    fixture.detectChanges();

    expect(component.busy()).toBeNull();
    expect((fixture.nativeElement as HTMLElement).querySelector('[data-testid="status-AO-3"]')?.textContent).toContain('Genehmigt');
  });

  it('cancels an order and reloads', () => {
    const fixture = loaded([draft]);
    const component = fixture.componentInstance;

    component.cancel(draft);
    http.expectOne('/api/payment-orders/1/cancel').flush({ ID: 1, Status: 'cancelled' });
    http.expectOne('/api/payment-orders').flush([]);
    fixture.detectChanges();

    expect(component.busy()).toBeNull();
    expect(component.actionError()).toBeNull();
  });

  it('asks for a reason before rejecting, and sends it', () => {
    const fixture = loaded([submitted]);
    const component = fixture.componentInstance;
    const html = fixture.nativeElement as HTMLElement;

    component.startReject(submitted);
    fixture.detectChanges();
    expect(component.rejecting()).toBe(2);
    expect(component.reason()).toBe('');
    expect(html.querySelector('[data-testid="order-AO-2"] [type="text"]')).toBeTruthy();

    // A blank reason does not reach the API.
    component.reject(submitted);
    expect(component.actionError()).toBe('Für eine Ablehnung ist ein Grund erforderlich.');
    http.expectNone('/api/payment-orders/2/reject');

    component.reason.set('  Sachlich falsch  ');
    component.reject(submitted);
    const post = http.expectOne('/api/payment-orders/2/reject');
    expect(post.request.body).toEqual({ reason: 'Sachlich falsch' });
    post.flush({ ID: 2, Status: 'rejected' });

    // The reason box is gone and the list reloaded.
    expect(component.rejecting()).toBeNull();
    expect(component.reason()).toBe('');
    http.expectOne('/api/payment-orders').flush([]);
    fixture.detectChanges();
    expect(component.busy()).toBeNull();
  });

  it('abandons a rejection without sending anything', () => {
    const fixture = loaded([submitted]);
    const component = fixture.componentInstance;

    component.startReject(submitted);
    component.reason.set('halb getippt');
    component.stopReject();
    fixture.detectChanges();

    expect(component.rejecting()).toBeNull();
    expect(component.reason()).toBe('');
    http.expectNone('/api/payment-orders/2/reject');
    expect((fixture.nativeElement as HTMLElement).querySelector('[type="text"]')).toBeNull();
  });

  it('names the order and keeps the list when an action is refused', () => {
    const fixture = loaded([submitted, approved]);
    const component = fixture.componentInstance;

    component.approve(submitted);
    http.expectOne('/api/payment-orders/2/approve').flush('no', { status: 403, statusText: 'Forbidden' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Anordnung AO-2 nicht genehmigen.');
    expect(component.busy()).toBeNull();
    // The list is still there, and the refused action did not become a load error.
    expect(component.loadError()).toBeNull();
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="order-AO-2"]')).toBeTruthy();
    expect(html.querySelector('table')).toBeTruthy();
  });

  it('keeps the reason box when a rejection is refused, so it can be retried', () => {
    const fixture = loaded([submitted]);
    const component = fixture.componentInstance;

    component.startReject(submitted);
    component.reason.set('zu teuer');
    component.reject(submitted);
    http.expectOne('/api/payment-orders/2/reject').flush('no', { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Anordnung AO-2 nicht ablehnen.');
    expect(component.rejecting()).toBe(2);
    expect(component.reason()).toBe('zu teuer');
    expect(component.busy()).toBeNull();
  });

  it('blanks the previous error when a new rejection starts', () => {
    const fixture = loaded([submitted]);
    const component = fixture.componentInstance;
    component.actionError.set('alt');

    component.startReject(submitted);
    expect(component.actionError()).toBeNull();
  });

  it('names the workflow states and formats figures', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;

    expect(component.statusLabel('draft')).toBe('Entwurf');
    expect(component.statusLabel('submitted')).toBe('Vorgelegt');
    expect(component.statusLabel('approved')).toBe('Genehmigt');
    expect(component.statusLabel('rejected')).toBe('Abgelehnt');
    expect(component.statusLabel('cancelled')).toBe('Storniert');
    expect(component.statusLabel('something-else')).toBe('something-else');
    expect(component.euro(125000)).toBe('1.250,00');
    expect(component.day('2026-04-01T00:00:00Z')).toBe('01.04.2026');
  });

  it('names the order and the step when submitting or cancelling is refused', () => {
    // The verb in the message is the one that was attempted, so the message says which step
    // failed rather than a generic "action".
    const fixture = loaded([draft]);
    const component = fixture.componentInstance;

    component.submit(draft);
    http.expectOne('/api/payment-orders/1/submit').flush('no', { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();
    expect(component.actionError()).toBe('Konnte die Anordnung AO-1 nicht vorlegen.');

    component.cancel(draft);
    http.expectOne('/api/payment-orders/1/cancel').flush('no', { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();
    expect(component.actionError()).toBe('Konnte die Anordnung AO-1 nicht stornieren.');
    expect(component.busy()).toBeNull();
  });

  it('leaves another order\'s reason box alone when a different action succeeds', () => {
    // The reason box belongs to the order being rejected: approving a different order must not
    // close it.
    const fixture = loaded([submitted, approved]);
    const component = fixture.componentInstance;

    component.startReject(submitted);
    component.reason.set('halb getippt');
    component.approve(approved);
    http.expectOne('/api/payment-orders/3/approve').flush({ ID: 3, Status: 'approved' });
    http.expectOne('/api/payment-orders').flush([submitted, approved]);
    fixture.detectChanges();

    expect(component.rejecting()).toBe(2);
    expect(component.reason()).toBe('halb getippt');
  });
});
