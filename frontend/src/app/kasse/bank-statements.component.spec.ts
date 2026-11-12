import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { BankStatementsComponent } from './bank-statements.component';
import { CashAccount, StatementLine, StatementSummary } from '../core/api/generated';

describe('BankStatementsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [BankStatementsComponent],
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

  const statement = (ID: number, Reference: string): StatementSummary => ({
    ID, AccountID: 1, Reference, Number: 1, Page: 1, Currency: 'EUR',
    OpeningBalanceCents: 1000000, OpeningDate: '2026-10-01T00:00:00Z',
    ClosingBalanceCents: 1000000, ClosingDate: '2026-10-31T00:00:00Z',
    LineCount: 0, ImportedBy: 'carol',
  });
  const first = statement(1, 'STMT-1');

  /** A page's worth of statements, so the screen believes there may be another. */
  const fullPage = (from: number): StatementSummary[] =>
    Array.from({ length: 50 }, (_, i) => statement(from + i, `STMT-${from + i}`));
  const second2 = statement(2, 'STMT-2');

  /** A bank statement line as the API returns it. */
  const line = (LineNo: number, Purpose: string): StatementLine => ({
    ID: LineNo, LineNo, ValueDate: '2026-10-05T00:00:00Z', BookingDate: '2026-10-05T00:00:00Z',
    Credit: false, Return: false, AmountCents: 125000, Currency: 'EUR', PostingKey: '051',
    ClientRef: '', BankRef: '', Purpose, EndToEndRef: '', CounterpartyName: '', CounterpartyIBAN: '',
    CounterpartyBIC: '', UnclearedCents: 0, Status: '', OrderID: null,
  });

  /** The screen with its accounts and one account's statements loaded. */
  function loaded(accounts: CashAccount[] = [uni], statements: StatementSummary[] = [first]) {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush(accounts);
    fixture.detectChanges();
    const req = http.match((r) => r.url.startsWith('/api/bank-statements?'));
    req.forEach((r) => r.flush(statements));
    fixture.detectChanges();
    return fixture;
  }

  it('starts with an unloaded, empty screen', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    const component = fixture.componentInstance;

    expect(component.accounts()).toEqual([]);
    expect(component.statements()).toEqual([]);
    expect(component.account()).toBe('');
    expect(component.content()).toBe('');
    expect(component.loading()).toBeTrue();
    expect(component.importing()).toBeFalse();
    expect(component.loadError()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.notice()).toBeNull();
    expect(component.open()).toBeNull();
    expect(component.lines()).toEqual([]);
  });

  it('loads the accounts, picks the first and lists its statements', () => {
    const fixture = loaded([uni, second], [first, second2]);
    const component = fixture.componentInstance;

    expect(component.loading()).toBeFalse();
    expect(component.accounts().length).toBe(2);
    expect(component.account()).toBe('1');
    expect(component.statements().length).toBe(2);
  });

  it('asks the API for the chosen account instead of sifting what it has', () => {
    const fixture = loaded([uni, second], [first]);
    const component = fixture.componentInstance;

    component.choose('2');
    expect(component.loading()).toBeTrue();
    const filtered = http.expectOne('/api/bank-statements?account_id=2&limit=50&offset=0');
    expect(filtered.request.method).toBe('GET');
    filtered.flush([second2]);
    fixture.detectChanges();

    expect(component.account()).toBe('2');
    expect(component.statements().map((s) => s.Reference)).toEqual(['STMT-2']);
  });

  it('renders each statement with its balances and closing date', () => {
    const fixture = loaded([uni], [first]);
    const html = fixture.nativeElement as HTMLElement;

    const cells = html.querySelectorAll('[data-testid="statement-STMT-1"] td');
    expect(cells[0].textContent).toContain('STMT-1');
    expect(cells[1].textContent).toContain('1');
    expect(cells[2].textContent).toContain('31.10.2026');
    expect(cells[3].textContent).toContain('10.000,00');
    expect(cells[4].textContent).toContain('10.000,00');
    expect(cells[5].textContent).toContain('0');
  });

  it('says so when no own account exists', () => {
    const fixture = loaded([], []);
    const html = fixture.nativeElement as HTMLElement;

    expect(html.querySelector('[role="status"]')?.textContent).toContain('kein eigenes Konto angelegt');
    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.account()).toBe('');
    // No account means no statements were asked for.
    http.expectNone((r) => r.url.startsWith('/api/bank-statements'));
  });

  it('says so when the chosen account has no statement', () => {
    const fixture = loaded([uni], []);
    const html = fixture.nativeElement as HTMLElement;

    expect(html.querySelector('[role="status"]')?.textContent).toContain('kein Auszug eingelesen');
    expect(html.querySelector('table')).toBeNull();
  });

  it('reports a failed account load instead of an empty list', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Konten nicht laden.');
    expect(component.loading()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).querySelector('[role="alert"]')).toBeTruthy();
  });

  it('reports a failed statement load', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush([uni]);
    fixture.detectChanges();
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Kontoauszüge nicht laden.');
    expect(component.loading()).toBeFalse();
  });

  it('treats an empty account response body as no accounts', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.accounts()).toEqual([]);
    expect(component.statements()).toEqual([]);
    expect(component.loading()).toBeFalse();
  });

  it('opens the lines of a statement and closes them again', () => {
    const fixture = loaded([uni], [first]);
    const component = fixture.componentInstance;

    component.toggleLines(first);
    expect(component.open()).toBe(1);
    const get = http.expectOne('/api/bank-statements/1');
    expect(get.request.method).toBe('GET');
    get.flush({
      Lines: [{ ...line(1, 'Zahlung'), Status: 'matched' }],
    });
    fixture.detectChanges();

    expect(component.lines().length).toBe(1);
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="lines-STMT-1"]')).toBeTruthy();
    expect(html.querySelector('[data-testid="line-1"]')?.textContent).toContain('Zahlung');
    expect(html.querySelector('[data-testid="line-1"]')?.textContent).toContain('1.250,00');
    expect(html.querySelector('[data-testid="line-1"]')?.textContent).toContain('matched');
    // The button reports the expanded state.
    expect(html.querySelector('[aria-expanded]')?.getAttribute('aria-expanded')).toBe('true');

    // Toggling the same statement shut is a local operation: nothing is fetched.
    component.toggleLines(first);
    expect(component.open()).toBeNull();
    expect(component.lines()).toEqual([]);
    http.expectNone('/api/bank-statements/1');
  });

  it('shows an unmatched line as open and says when a statement has no lines', () => {
    const fixture = loaded([uni], [first]);
    const component = fixture.componentInstance;

    component.toggleLines(first);
    http.expectOne('/api/bank-statements/1').flush({
      Lines: [{ ...line(4, ''), Credit: true, AmountCents: 100 }],
    });
    fixture.detectChanges();
    // A line the bank has not matched yet reads as open rather than as a blank cell.
    expect((fixture.nativeElement as HTMLElement).querySelector('[data-testid="line-4"]')?.textContent).toContain('offen');

    component.toggleLines(first);
    component.toggleLines(first);
    http.expectOne('/api/bank-statements/1').flush({ Lines: [] });
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Dieser Auszug hat keine Zeilen.');

    // A statement with no lines field at all is the same as one with none.
    component.toggleLines(first);
    component.toggleLines(first);
    http.expectOne('/api/bank-statements/1').flush({});
    fixture.detectChanges();
    expect(component.lines()).toEqual([]);
  });

  it('names the statement when its lines cannot be loaded, and does not leave it open', () => {
    const fixture = loaded([uni], [first]);
    const component = fixture.componentInstance;

    component.toggleLines(first);
    http.expectOne('/api/bank-statements/1').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Zeilen von Auszug STMT-1 nicht laden.');
    expect(component.open()).toBeNull();
  });

  it('imports content and reloads the statements of that account', () => {
    const fixture = loaded([uni], []);
    const component = fixture.componentInstance;
    component.content.set(':20:STMT-3\n:25:DE02120300000000202051\n:62F:C261031EUR10000,00\n-\n');

    component.import(new Event('submit'));

    expect(component.importing()).toBeTrue();
    const post = http.expectOne('/api/bank-statements');
    expect(post.request.method).toBe('POST');
    // The content is sent trimmed, so a pasted file's trailing newline does not become part of
    // the request.
    expect(post.request.body).toEqual({
      account_id: 1, content: ':20:STMT-3\n:25:DE02120300000000202051\n:62F:C261031EUR10000,00\n-',
    });
    post.flush([first]);
    fixture.detectChanges();

    expect(component.importing()).toBeFalse();
    expect(component.notice()).toBe('Ein Auszug eingelesen.');
    // The content box is cleared, and the list is reloaded.
    expect(component.content()).toBe('');
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush([first]);
    fixture.detectChanges();
    expect(component.statements().length).toBe(1);
  });

  it('counts several imported statements', () => {
    const fixture = loaded([uni], []);
    const component = fixture.componentInstance;
    component.content.set(':20:A\n');

    component.import(new Event('submit'));
    http.expectOne('/api/bank-statements').flush([first, second2]);
    fixture.detectChanges();

    expect(component.notice()).toBe('2 Auszüge eingelesen.');
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush([first, second2]);
  });

  it('treats an empty import response as nothing read in', () => {
    const fixture = loaded([uni], []);
    const component = fixture.componentInstance;
    component.content.set(':20:A\n');

    component.import(new Event('submit'));
    http.expectOne('/api/bank-statements').flush(null);
    fixture.detectChanges();

    expect(component.notice()).toBe('0 Auszüge eingelesen.');
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush([]);
  });

  it('refuses an empty import without asking the API', () => {
    const fixture = loaded([uni], []);
    const component = fixture.componentInstance;
    component.content.set('   ');

    component.import(new Event('submit'));

    expect(component.actionError()).toBe('Ein Auszug braucht Inhalt.');
    expect(component.importing()).toBeFalse();
    http.expectNone('/api/bank-statements');
  });

  it('keeps the content and names the failure when an import is refused', () => {
    const fixture = loaded([uni], []);
    const component = fixture.componentInstance;
    component.content.set(':20:BAD\n');

    component.import(new Event('submit'));
    http.expectOne('/api/bank-statements').flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte den Auszug nicht einlesen.');
    expect(component.importing()).toBeFalse();
    // The typed content is still there to correct and send again.
    expect(component.content()).toBe(':20:BAD\n');
    expect(component.loadError()).toBeNull();
    // A failed import must not re-read the list either. The predicate is on the collection, not on one
    // URL: an exact URL that no longer matches anything would make this pass for the wrong reason.
    http.expectNone((r) => r.url.startsWith('/api/bank-statements'));
  });

  it('clears a previous notice and error when another account is chosen', () => {
    const fixture = loaded([uni, second], [first]);
    const component = fixture.componentInstance;
    component.notice.set('alt');
    component.actionError.set('alt');

    component.choose('2');
    http.expectOne('/api/bank-statements?account_id=2&limit=50&offset=0').flush([]);
    fixture.detectChanges();

    expect(component.notice()).toBeNull();
    expect(component.actionError()).toBeNull();
  });

  it('closes an open statement when the account changes', () => {
    const fixture = loaded([uni, second], [first]);
    const component = fixture.componentInstance;
    component.open.set(1);
    component.lines.set([line(1, '')]);

    component.choose('2');
    expect(component.open()).toBeNull();
    expect(component.lines()).toEqual([]);
    http.expectOne('/api/bank-statements?account_id=2&limit=50&offset=0').flush([]);
  });

  it('formats amounts and dates with the shared helpers', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;

    expect(component.euro(125000)).toBe('1.250,00');
    expect(component.euro(0)).toBe('0,00');
    expect(component.day('2026-10-31T00:00:00Z')).toBe('31.10.2026');
  });

  it('shows the loading state again on a reload', () => {
    const fixture = loaded([uni], [first]);
    const component = fixture.componentInstance;
    expect(component.loading()).toBeFalse();

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/cash-accounts').flush([uni]);
    fixture.detectChanges();
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush([first]);
    fixture.detectChanges();
    expect(component.loading()).toBeFalse();
  });

  it('treats an empty statement response body as no statements', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush([uni]);
    fixture.detectChanges();
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.statements()).toEqual([]);
    expect(component.loadError()).toBeNull();
    expect(component.loading()).toBeFalse();
  });

  it('clears the previous lines as soon as another statement is opened', () => {
    // The old statement's lines must not stand under the new one's heading while the request is
    // in flight.
    const fixture = loaded([uni], [first]);
    const component = fixture.componentInstance;
    component.lines.set([line(9, 'alt')]);

    component.toggleLines(first);
    expect(component.lines()).toEqual([]);

    http.expectOne('/api/bank-statements/1').flush({ Lines: [] });
  });

  it('reads a statement with no body at all as one without lines', () => {
    const fixture = loaded([uni], [first]);
    const component = fixture.componentInstance;

    component.toggleLines(first);
    http.expectOne('/api/bank-statements/1').flush(null);
    fixture.detectChanges();

    expect(component.lines()).toEqual([]);
    expect(component.actionError()).toBeNull();
    expect(component.open()).toBe(1);
  });

  it('opens with one page and offers the next only while the pages come back full', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();

    // Nothing has been read yet, so there is no next page to offer and none is being read.
    expect(component.hasMore()).toBeFalse();
    expect(component.loadingMore()).toBeFalse();

    http.expectOne('/api/cash-accounts').flush([uni]);
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    expect(component.statements().length).toBe(50);
    expect(component.hasMore()).toBeTrue();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Weitere laden');
  });

  it('reads the next page of the chosen account and appends it', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush([uni]);
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    component.loadMore();

    expect(component.loadingMore()).toBeTrue();
    // The page is the chosen account's, from where the last one ended — the filter and the page are
    // asked for together, so switching account cannot mix two accounts' pages.
    const secondPage = http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=50');
    fixture.detectChanges();
    const button = (fixture.nativeElement as HTMLElement).querySelector(
      '[data-testid="load-more"]',
    ) as HTMLButtonElement;
    expect(button.disabled).toBeTrue();
    expect(button.textContent).toContain('Weitere werden geladen');

    secondPage.flush(fullPage(51));
    fixture.detectChanges();

    expect(component.statements().length).toBe(100);
    expect(component.statements()[0].Reference).toBe('STMT-1');
    // A second full page means there may be a third, so the offer stays.
    expect(component.hasMore()).toBeTrue();
    expect(component.loadingMore()).toBeFalse();
  });

  it('stops offering the next page when a page comes back short or empty', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush([uni]);
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    component.loadMore();
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=50').flush(null);
    fixture.detectChanges();

    // An empty page appends nothing: the rows read are the rows on screen, whatever the server says.
    expect(component.statements().length).toBe(50);
    expect(component.hasMore()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).textContent).not.toContain('Weitere laden');

    // With no next page to offer there is nothing to ask for.
    component.loadMore();
    expect(component.loadingMore()).toBeFalse();
  });

  it('keeps the statements it has when the next page cannot be read, and does not read it twice', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush([uni]);
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();

    component.loadMore();
    // A second click while the page is in flight must not send a second request.
    component.loadMore();
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=50').flush(
      { error: 'unreadable' },
      { status: 500, statusText: 'Internal Server Error' },
    );
    fixture.detectChanges();

    expect(component.statements().length).toBe(50);
    expect(component.loadingMore()).toBeFalse();
    expect(component.actionError()).toBe('Konnte die weiteren Kontoauszüge nicht laden.');
    expect(component.loadError()).toBeNull();
    // The offer stays: the page failed, so it is still there to be read.
    expect(component.hasMore()).toBeTrue();
  });

  it('clears the offer when another account is chosen', () => {
    const fixture = TestBed.createComponent(BankStatementsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush([uni, second]);
    http.expectOne('/api/bank-statements?account_id=1&limit=50&offset=0').flush(fullPage(1));
    fixture.detectChanges();
    expect(component.hasMore()).toBeTrue();

    component.choose('2');

    // The other account's list starts from its own first page: a stale offer would ask for a page of
    // the account that is no longer on screen.
    expect(component.hasMore()).toBeFalse();
    http.expectOne('/api/bank-statements?account_id=2&limit=50&offset=0').flush([second2]);
    fixture.detectChanges();

    expect(component.statements().length).toBe(1);
    expect(component.hasMore()).toBeFalse();
  });
});
