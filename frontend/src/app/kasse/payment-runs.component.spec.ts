import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { PaymentRunsComponent } from './payment-runs.component';
import { CashAccount, KasseOrderSummary, PaymentRun } from '../core/api/generated';

describe('PaymentRunsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PaymentRunsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const account = (ID: number, Name: string): CashAccount => ({
    ID, Name, IBAN: 'DE02120300000000202051', BIC: 'BYLADEM1001', Currency: 'EUR',
  });
  const uni = account(1, 'Universität');
  const second = account(2, 'Zweitkonto');

  const order = (ID: number, over: Partial<KasseOrderSummary> = {}): KasseOrderSummary => ({
    ID,
    Reference: 'AO-' + ID,
    AmountCents: 1000,
    Currency: 'EUR',
    BudgetPositionID: 1,
    Purpose: 'Beispiel',
    DueDate: '2026-11-01T00:00:00Z',
    Status: 'approved',
    ...over,
  });
  const first = order(1, { AmountCents: 2500, Purpose: 'Miete' });
  const secondOrder = order(2, { AmountCents: 3000, Purpose: 'Strom' });

  const run = (over: Partial<PaymentRun> = {}): PaymentRun => ({
    ID: 1,
    Reference: 'RUN-1',
    AccountID: 1,
    ExecutionDate: '2026-11-01T00:00:00Z',
    MessageID: 'RUN-1',
    ItemCount: 2,
    SumCents: 5500,
    FileHash: 'h',
    CreatedBy: 'carol',
    ...over,
  });

  /** The screen with its accounts and approved orders loaded. */
  function loaded(accounts: CashAccount[] = [uni], orders: KasseOrderSummary[] = [first, secondOrder]) {
    const fixture = TestBed.createComponent(PaymentRunsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush(accounts);
    if (accounts.length > 0) {
      http.expectOne('/api/payment-orders?status=approved').flush(orders);
    }
    fixture.detectChanges();
    return fixture;
  }

  it('starts unloaded and empty', () => {
    const fixture = TestBed.createComponent(PaymentRunsComponent);
    const component = fixture.componentInstance;

    expect(component.accounts()).toEqual([]);
    expect(component.account()).toBe('');
    expect(component.orders()).toEqual([]);
    expect(component.loading()).toBeTrue();
    expect(component.submitting()).toBeFalse();
    expect(component.exporting()).toBeFalse();
    expect(component.notice()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.created()).toBeNull();
    expect(component.file()).toBeNull();
    // The form starts blank, with nothing selected.
    expect(component.reference()).toBe('');
    expect(component.executionDate()).toBe('');
    expect(component.selected()).toEqual([]);
  });

  it('loads the accounts, then the approved orders, and picks the first account', () => {
    const fixture = loaded([uni, second]);
    const component = fixture.componentInstance;

    expect(component.loading()).toBeFalse();
    expect(component.accounts().length).toBe(2);
    expect(component.account()).toBe('1');
    // Only the approved orders are asked for — the server's filter, not a screen-side sift.
    expect(component.orders().map((o) => o.Reference)).toEqual(['AO-1', 'AO-2']);
  });

  it('keeps a chosen account on a reload', () => {
    const fixture = loaded([uni, second]);
    const component = fixture.componentInstance;
    component.account.set('2');

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/cash-accounts').flush([uni, second]);
    http.expectOne('/api/payment-orders?status=approved').flush([first]);
    fixture.detectChanges();

    expect(component.account()).toBe('2');
    expect(component.loading()).toBeFalse();
  });

  it('treats an empty account response as no accounts', () => {
    const fixture = TestBed.createComponent(PaymentRunsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.accounts()).toEqual([]);
    expect(component.account()).toBe('');
    expect(component.loading()).toBeFalse();
  });

  it('treats an empty order response as no orders', () => {
    const fixture = TestBed.createComponent(PaymentRunsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush([uni]);
    http.expectOne('/api/payment-orders?status=approved').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.orders()).toEqual([]);
    expect(component.loading()).toBeFalse();
  });

  it('says so when no own account exists, and does not ask for orders', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;

    expect(component.loading()).toBeFalse();
    expect(component.account()).toBe('');
    http.expectNone('/api/payment-orders?status=approved');
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('kein eigenes Konto angelegt');
  });

  it('says so when no approved order exists', () => {
    const fixture = loaded([uni], []);
    const component = fixture.componentInstance;

    expect(component.loading()).toBeFalse();
    expect(component.orders()).toEqual([]);
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('keine genehmigte Zahlungsanordnung');
  });

  it('reports a failed account load instead of an empty list', () => {
    const fixture = TestBed.createComponent(PaymentRunsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Konten nicht laden.');
    expect(component.loading()).toBeFalse();
  });

  it('reports a failed order load', () => {
    const fixture = TestBed.createComponent(PaymentRunsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush([uni]);
    http.expectOne('/api/payment-orders?status=approved').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Zahlungsanordnungen nicht laden.');
    expect(component.loading()).toBeFalse();
  });

  it('selects orders and sums what is chosen', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    expect(component.isSelected(1)).toBeFalse();
    expect(component.selectedSum()).toBe(0);

    component.toggle(1);
    component.toggle(2);
    expect(component.selected()).toEqual([1, 2]);
    expect(component.isSelected(1)).toBeTrue();
    expect(component.selectedSum()).toBe(5500);

    // Toggling one off again leaves the other.
    component.toggle(1);
    expect(component.selected()).toEqual([2]);
    expect(component.selectedSum()).toBe(3000);
  });

  it('drops a selection the reload no longer offers', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.toggle(1);
    component.toggle(2);

    component.load();
    http.expectOne('/api/cash-accounts').flush([uni]);
    // Only the second order is still approved; a third is offered alongside it.
    http
      .expectOne('/api/payment-orders?status=approved')
      .flush([secondOrder, order(3, { Reference: 'AO-3' })]);
    fixture.detectChanges();

    expect(component.selected()).toEqual([2]);
    expect(component.selectedSum()).toBe(3000);
  });

  it('creates a run from the chosen orders and shows it', () => {
    const fixture = loaded([uni, second]);
    const component = fixture.componentInstance;
    component.account.set('2');
    component.reference.set('  RUN-1 ');
    component.executionDate.set(' 2026-11-01 ');
    component.toggle(1);
    component.toggle(2);

    component.create(new Event('submit'));

    expect(component.submitting()).toBeTrue();
    const post = http.expectOne('/api/payment-runs');
    expect(post.request.method).toBe('POST');
    // Every field is trimmed, the account id travels as a number, and the ids go as selected.
    expect(post.request.body).toEqual({
      reference: 'RUN-1',
      account_id: 2,
      execution_date: '2026-11-01',
      order_ids: [1, 2],
    });
    post.flush(run({ AccountID: 2 }));
    fixture.detectChanges();

    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBe('Zahlungslauf erstellt.');
    const row = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="run-RUN-1"]');
    expect(row?.textContent).toContain('2 Posten');
    expect(row?.textContent).toContain('55,00');
    expect(row?.textContent).toContain('01.11.2026');
  });

  it('refuses an incomplete run without asking the API', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Eine Referenz ist erforderlich.');

    component.reference.set('RUN-1');
    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Ein Ausführungsdatum ist erforderlich.');

    // A whitespace-only date is a missing date.
    component.executionDate.set('   ');
    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Ein Ausführungsdatum ist erforderlich.');

    component.executionDate.set('2026-11-01');
    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Mindestens eine Anordnung ist erforderlich.');

    expect(component.submitting()).toBeFalse();
    http.expectNone('/api/payment-runs');
  });

  it('names the failure when the run is refused, and keeps the selection', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.reference.set('RUN-1');
    component.executionDate.set('2026-11-01');
    component.toggle(1);

    component.create(new Event('submit'));
    http.expectOne('/api/payment-runs').flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte den Zahlungslauf nicht erstellen.');
    expect(component.submitting()).toBeFalse();
    expect(component.created()).toBeNull();
    expect(component.selected()).toEqual([1]);
  });

  it('reads the produced file and shows it', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.reference.set('RUN-1');
    component.executionDate.set('2026-11-01');
    component.toggle(1);
    component.create(new Event('submit'));
    http.expectOne('/api/payment-runs').flush(run());
    fixture.detectChanges();

    component.fetchFile();
    const get = http.expectOne('/api/payment-runs/1/file');
    expect(get.request.method).toBe('GET');
    get.flush('<Document><CstmrCdtTrfInitn/></Document>');
    fixture.detectChanges();

    const file = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="run-file-RUN-1"]');
    expect(file?.textContent).toContain('<CstmrCdtTrfInitn/>');
  });

  it('names the failure when the file cannot be read', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.reference.set('RUN-1');
    component.executionDate.set('2026-11-01');
    component.toggle(1);
    component.create(new Event('submit'));
    http.expectOne('/api/payment-runs').flush(run());
    fixture.detectChanges();

    component.fetchFile();
    http.expectOne('/api/payment-runs/1/file').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Datei nicht laden.');
    expect(component.file()).toBeNull();
  });

  it('queues the file for the bank channel and reports the job', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.reference.set('RUN-1');
    component.executionDate.set('2026-11-01');
    component.toggle(1);
    component.create(new Event('submit'));
    http.expectOne('/api/payment-runs').flush(run());
    fixture.detectChanges();

    component.queueExport();
    expect(component.exporting()).toBeTrue();
    const post = http.expectOne('/api/payment-runs/1/exports');
    expect(post.request.method).toBe('POST');
    post.flush({ job_id: 77 });
    fixture.detectChanges();

    expect(component.exporting()).toBeFalse();
    expect(component.notice()).toBe('Übergabe an den Bankkanal beauftragt (Job 77).');
  });

  it('names the failure when the hand-over cannot be queued', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.reference.set('RUN-1');
    component.executionDate.set('2026-11-01');
    component.toggle(1);
    component.create(new Event('submit'));
    http.expectOne('/api/payment-runs').flush(run());
    fixture.detectChanges();

    component.queueExport();
    http.expectOne('/api/payment-runs/1/exports').flush('no', { status: 503, statusText: 'Service Unavailable' });
    fixture.detectChanges();

    expect(component.exporting()).toBeFalse();
    expect(component.actionError()).toBe('Konnte die Übergabe nicht beauftragen.');
  });

  it('does nothing when the file or the hand-over is asked for before a run exists', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    component.fetchFile();
    component.queueExport();

    http.expectNone('/api/payment-runs/1/file');
    http.expectNone('/api/payment-runs/1/exports');
    expect(component.exporting()).toBeFalse();
  });

  it('renders amounts and dates with the shared helpers', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    expect(component.euro(5500)).toBe('55,00');
    expect(component.euro(0)).toBe('0,00');
    expect(component.day('2026-11-01T00:00:00Z')).toBe('01.11.2026');
  });
});
