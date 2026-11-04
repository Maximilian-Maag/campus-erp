import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { CollectionRunsComponent } from './collection-runs.component';
import { CashAccount, CollectionRun } from '../core/api/generated';

describe('CollectionRunsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [CollectionRunsComponent],
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

  const run = (over: Partial<CollectionRun> = {}): CollectionRun => ({
    ID: 1,
    Reference: 'RUN-1',
    AccountID: 1,
    CreditorID: 'DE98ZZZ09999999999',
    CollectionDate: '2026-11-01T00:00:00Z',
    SequenceType: 'RCUR',
    LocalInstrument: 'CORE',
    MessageID: 'RUN-1',
    ItemCount: 2,
    SumCents: 5500,
    FileHash: 'h',
    CreatedBy: 'carol',
    ...over,
  });

  /** The screen with its accounts loaded. */
  function loaded(accounts: CashAccount[] = [uni]) {
    const fixture = TestBed.createComponent(CollectionRunsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush(accounts);
    fixture.detectChanges();
    return fixture;
  }

  it('starts unloaded and empty', () => {
    const fixture = TestBed.createComponent(CollectionRunsComponent);
    const component = fixture.componentInstance;

    expect(component.accounts()).toEqual([]);
    expect(component.account()).toBe('');
    expect(component.loading()).toBeTrue();
    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.created()).toBeNull();
    expect(component.file()).toBeNull();
    // The form starts blank, with the sequence type and instrument defaulting.
    expect(component.reference()).toBe('');
    expect(component.creditorID()).toBe('');
    expect(component.collectionDate()).toBe('');
    expect(component.dueBy()).toBe('');
    expect(component.sequenceType()).toBe('RCUR');
    expect(component.localInstrument()).toBe('CORE');
  });

  it('loads the accounts, picks the first and shows the loading state again on a reload', () => {
    const fixture = loaded([uni, second]);
    const component = fixture.componentInstance;

    expect(component.loading()).toBeFalse();
    expect(component.accounts().length).toBe(2);
    expect(component.account()).toBe('1');

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/cash-accounts').flush([uni]);
    fixture.detectChanges();
    expect(component.loading()).toBeFalse();
  });

  it('keeps a chosen account on a reload', () => {
    const fixture = loaded([uni, second]);
    const component = fixture.componentInstance;
    component.account.set('2');

    component.load();
    http.expectOne('/api/cash-accounts').flush([uni, second]);
    fixture.detectChanges();

    expect(component.account()).toBe('2');
  });

  it('treats an empty account response as no accounts', () => {
    const fixture = TestBed.createComponent(CollectionRunsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.accounts()).toEqual([]);
    expect(component.account()).toBe('');
    expect(component.loading()).toBeFalse();
  });

  it('says so when no own account exists', () => {
    const fixture = loaded([]);
    const html = fixture.nativeElement as HTMLElement;
    expect(html.textContent).toContain('kein eigenes Konto angelegt');
  });

  it('reports a failed account load instead of an empty list', () => {
    const fixture = TestBed.createComponent(CollectionRunsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Konten nicht laden.');
    expect(component.loading()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).querySelector('[role="alert"]')).toBeTruthy();
  });

  it('creates a run from the receivables and shows what it produced', () => {
    const fixture = loaded([uni, second]);
    const component = fixture.componentInstance;
    component.account.set('2');
    component.reference.set('  RUN-1 ');
    component.creditorID.set(' DE98ZZZ09999999999 ');
    component.collectionDate.set(' 2026-11-01 ');
    component.dueBy.set(' 2026-10-15 ');
    component.sequenceType.set('FRST');
    component.localInstrument.set('B2B');

    component.run(new Event('submit'));

    expect(component.submitting()).toBeTrue();
    const post = http.expectOne('/api/direct-debit/runs/from-receivables');
    expect(post.request.method).toBe('POST');
    // Every field is trimmed, and the account id travels as a number.
    expect(post.request.body).toEqual({
      reference: 'RUN-1',
      account_id: 2,
      creditor_id: 'DE98ZZZ09999999999',
      collection_date: '2026-11-01',
      sequence_type: 'FRST',
      local_instrument: 'B2B',
      due_by: '2026-10-15',
    });
    post.flush(run({ AccountID: 2 }));
    fixture.detectChanges();

    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBe('Einzug erstellt.');
    const row = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="run-RUN-1"]');
    expect(row?.textContent).toContain('2 Posten');
    expect(row?.textContent).toContain('55,00');
    expect(row?.textContent).toContain('01.11.2026');
  });

  it('refuses a blank reference without asking the API', () => {
    const fixture = loaded([uni]);
    const component = fixture.componentInstance;
    component.collectionDate.set('2026-11-01');
    component.dueBy.set('2026-10-15');

    component.run(new Event('submit'));

    expect(component.actionError()).toBe('Eine Referenz ist erforderlich.');
    expect(component.submitting()).toBeFalse();
    http.expectNone('/api/direct-debit/runs/from-receivables');
  });

  it('refuses a missing date without asking the API', () => {
    const fixture = loaded([uni]);
    const component = fixture.componentInstance;
    component.reference.set('RUN-1');

    // Blank and whitespace-only are both a missing date, on either field.
    for (const [collectionDate, dueBy] of [['', '2026-10-15'], ['   ', '2026-10-15'], ['2026-11-01', ''], ['2026-11-01', '   ']]) {
      component.collectionDate.set(collectionDate);
      component.dueBy.set(dueBy);
      component.run(new Event('submit'));
      expect(component.actionError()).toBe('Einzugsdatum und Fälligkeit sind erforderlich.');
      expect(component.submitting()).toBeFalse();
    }
    http.expectNone('/api/direct-debit/runs/from-receivables');
  });

  it('names the failure when the run is refused, and keeps what was typed', () => {
    const fixture = loaded([uni]);
    const component = fixture.componentInstance;
    component.reference.set('RUN-1');
    component.collectionDate.set('2026-11-01');
    component.dueBy.set('2026-11-30');

    component.run(new Event('submit'));
    http.expectOne('/api/direct-debit/runs/from-receivables').flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte den Einzug nicht erstellen.');
    expect(component.submitting()).toBeFalse();
    expect(component.created()).toBeNull();
    expect(component.reference()).toBe('RUN-1');
  });

  it('reads the produced file and shows it', () => {
    const fixture = loaded([uni]);
    const component = fixture.componentInstance;
    component.reference.set('RUN-1');
    component.collectionDate.set('2026-11-01');
    component.dueBy.set('2026-10-15');
    component.run(new Event('submit'));
    http.expectOne('/api/direct-debit/runs/from-receivables').flush(run());
    fixture.detectChanges();

    component.fetchFile();
    const get = http.expectOne('/api/direct-debit/runs/RUN-1/file');
    expect(get.request.method).toBe('GET');
    get.flush('<Document><CstmrDrctDbtInitn/></Document>');
    fixture.detectChanges();

    const file = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="run-file-RUN-1"]');
    expect(file?.textContent).toContain('<CstmrDrctDbtInitn/>');
  });

  it('names the failure when the file cannot be read', () => {
    const fixture = loaded([uni]);
    const component = fixture.componentInstance;
    component.reference.set('RUN-1');
    component.collectionDate.set('2026-11-01');
    component.dueBy.set('2026-10-15');
    component.run(new Event('submit'));
    http.expectOne('/api/direct-debit/runs/from-receivables').flush(run());
    fixture.detectChanges();

    component.fetchFile();
    http.expectOne('/api/direct-debit/runs/RUN-1/file').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Datei nicht laden.');
    expect(component.file()).toBeNull();
  });

  it('does nothing when the file is asked for before a run exists', () => {
    const fixture = loaded([uni]);
    fixture.componentInstance.fetchFile();
    http.expectNone('/api/direct-debit/runs/RUN-1/file');
  });

  it('renders amounts and dates with the shared helpers', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;

    expect(component.euro(5500)).toBe('55,00');
    expect(component.euro(0)).toBe('0,00');
    expect(component.day('2026-11-01T00:00:00Z')).toBe('01.11.2026');
  });
});
