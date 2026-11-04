import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { KlrCostTypesComponent } from './cost-types.component';
import { CostType, CostTypeAccount } from '../core/api/generated';

describe('KlrCostTypesComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [KlrCostTypesComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const type = (ID: number, over: Partial<CostType> = {}): CostType => ({
    ID,
    Code: '4711',
    Name: 'Personalkosten',
    CreatedBy: 'alice',
    ...over,
  });

  const account = (ID: number, AccountCode: string, CostTypeID = 1): CostTypeAccount => ({
    ID,
    CostTypeID,
    AccountCode,
  });

  const accountsURL = (id: number | string) => '/api/klr/cost-types/' + id + '/accounts';

  /** The screen with the catalogue and the first cost type's accounts loaded. */
  function loaded(types: CostType[] = [type(1)], accounts: CostTypeAccount[] = [account(1, '6810')]) {
    const fixture = TestBed.createComponent(KlrCostTypesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-types').flush(types);
    if (types.length > 0) {
      http.expectOne(accountsURL(types[0].ID)).flush(accounts);
    }
    fixture.detectChanges();
    return { fixture, component };
  }

  it('starts unloaded and blank', () => {
    const fixture = TestBed.createComponent(KlrCostTypesComponent);
    const component = fixture.componentInstance;

    expect(component.types()).toEqual([]);
    expect(component.accounts()).toEqual([]);
    expect(component.chosen()).toBe('');
    expect(component.loading()).toBeTrue();
    expect(component.saving()).toBeFalse();
    expect(component.mapping()).toBeFalse();
    expect(component.loadError()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.notice()).toBeNull();
    expect(component.lookupResult()).toBeNull();
    expect(component.lookupMiss()).toBeFalse();
    // Only the create form carries a seed value; every other field starts empty.
    expect(component.code()).toBe('');
    expect(component.name()).toBe('');
    expect(component.accountCode()).toBe('');
    expect(component.lookupCode()).toBe('');
  });

  it('loads the catalogue, chooses the first cost type and shows its accounts', () => {
    const { fixture, component } = loaded([type(1), type(2, { Code: '5000', Name: 'Sachkosten' })], [
      account(1, '6810'),
      account(2, '6820'),
    ]);
    const html = fixture.nativeElement as HTMLElement;

    expect(component.loading()).toBeFalse();
    expect(component.types().length).toBe(2);
    // The first entry is the one the operator most likely means to start with.
    expect(component.chosen()).toBe('1');
    expect(html.querySelector('[data-testid="cost-type-code-1"]')?.textContent).toContain('4711');
    expect(html.querySelector('[data-testid="cost-type-name-2"]')?.textContent).toContain('Sachkosten');
    expect(html.querySelector('[data-testid="account-6810"]')?.textContent).toContain('6810');
    expect(html.querySelector('[data-testid="account-6820"]')).not.toBeNull();
  });

  it('marks the chosen cost type in the catalogue', () => {
    const { fixture } = loaded([type(1), type(2)]);
    const rows = (fixture.nativeElement as HTMLElement).querySelectorAll('tbody tr');

    expect(rows[0].classList.contains('chosen')).toBeTrue();
    expect(rows[1].classList.contains('chosen')).toBeFalse();
  });

  it('treats an empty catalogue as no cost types, with nothing chosen', () => {
    const fixture = TestBed.createComponent(KlrCostTypesComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-types').flush([]);
    fixture.detectChanges();

    expect(component.types()).toEqual([]);
    expect(component.chosen()).toBe('');
    expect(component.loading()).toBeFalse();
    expect(component.loadError()).toBeNull();
    const html = fixture.nativeElement as HTMLElement;
    expect(html.textContent).toContain('keine Kostenarten angelegt');
    // An empty catalogue must still offer the form: otherwise the first cost type could not be
    // created from the screen at all.
    expect(html.querySelector('[data-testid="create-cost-type"]')).not.toBeNull();
  });

  it('treats an empty response body as no cost types', () => {
    const fixture = TestBed.createComponent(KlrCostTypesComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-types').flush(null);
    fixture.detectChanges();

    expect(fixture.componentInstance.types()).toEqual([]);
    expect(fixture.componentInstance.loading()).toBeFalse();
  });

  it('treats an empty accounts body as no accounts', () => {
    const { component } = loaded([type(1)], null as unknown as CostTypeAccount[]);

    expect(component.accounts()).toEqual([]);
    expect(component.loadError()).toBeNull();
  });

  it('says so when a chosen cost type has no accounts yet', () => {
    const { fixture } = loaded([type(1)], []);
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-testid="no-accounts"]'),
    ).not.toBeNull();
  });

  it('shows the loading state again on a reload', () => {
    const { component } = loaded([type(1)]);
    expect(component.loading()).toBeFalse();

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/klr/cost-types').flush([type(1)]);
    http.expectOne(accountsURL(1)).flush([]);
    expect(component.loading()).toBeFalse();
  });

  it('keeps the chosen cost type across a reload when it is still there', () => {
    const { component } = loaded([type(1), type(2)]);

    component.choose(type(2));
    http.expectOne(accountsURL(2)).flush([]);
    expect(component.chosen()).toBe('2');

    component.load();
    http.expectOne('/api/klr/cost-types').flush([type(1), type(2)]);
    http.expectOne(accountsURL(2)).flush([]);

    expect(component.chosen()).toBe('2');
  });

  it('drops the selection when the chosen cost type is gone', () => {
    const { component } = loaded([type(1), type(2)]);

    component.choose(type(2));
    http.expectOne(accountsURL(2)).flush([]);

    component.load();
    http.expectOne('/api/klr/cost-types').flush([type(1)]);
    http.expectOne(accountsURL(1)).flush([]);

    // The list moved on to the first entry rather than keeping a row that no longer exists.
    expect(component.chosen()).toBe('1');
  });

  it('reports a catalogue that could not be loaded', () => {
    const fixture = TestBed.createComponent(KlrCostTypesComponent);
    fixture.detectChanges();
    http.expectOne('/api/klr/cost-types').error(new ProgressEvent('error'));
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Kostenarten nicht laden.');
    expect(component.loading()).toBeFalse();
  });

  it('reports accounts that could not be loaded', () => {
    const { fixture, component } = loaded([type(1)]);

    component.loadAccounts();
    http.expectOne(accountsURL(1)).error(new ProgressEvent('error'));
    fixture.detectChanges();

    expect(component.loadError()).toBe('Konnte die Konten nicht laden.');
  });

  it('clears the accounts when there is no chosen cost type', () => {
    const fixture = TestBed.createComponent(KlrCostTypesComponent);
    const component = fixture.componentInstance;
    component.accounts.set([account(1, '6810')]);
    component.chosen.set('');

    component.loadAccounts();

    expect(component.accounts()).toEqual([]);
  });

  it('switches to the cost type the operator picks', () => {
    const { fixture, component } = loaded([type(1), type(2)], []);

    component.notice.set('Konto zugeordnet.');
    component.actionError.set('etwas');
    component.choose(type(2));

    expect(component.chosen()).toBe('2');
    expect(component.notice()).toBeNull();
    expect(component.actionError()).toBeNull();
    http.expectOne(accountsURL(2)).flush([account(5, '8200', 2)]);
    fixture.detectChanges();

    expect(component.accounts().map((a) => a.AccountCode)).toEqual(['8200']);
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-testid="choose-2"]'),
    ).not.toBeNull();
  });

  it('refuses to create a cost type without a code or a name', () => {
    const { component } = loaded();

    component.code.set('   ');
    component.name.set('Personalkosten');
    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Schlüssel und Bezeichnung sind erforderlich.');
    expect(component.saving()).toBeFalse();

    component.code.set('4711');
    component.name.set('  ');
    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Schlüssel und Bezeichnung sind erforderlich.');
  });

  it('creates a cost type, clears the form and moves to it', () => {
    const { component } = loaded([]);

    component.code.set('4711');
    component.name.set('Personalkosten');
    component.create(new Event('submit'));
    expect(component.saving()).toBeTrue();

    const request = http.expectOne('/api/klr/cost-types');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({ code: '4711', name: 'Personalkosten' });
    request.flush(type(7));

    expect(component.saving()).toBeFalse();
    expect(component.notice()).toBe('Kostenart angelegt.');
    expect(component.code()).toBe('');
    expect(component.name()).toBe('');

    // The catalogue is reloaded and the screen moves to the new cost type. The list comes back
    // with another entry first, so "7" is only chosen because the create response said so — a
    // reload that simply picks the first entry would land on 1.
    http.expectOne('/api/klr/cost-types').flush([type(1), type(7)]);
    http.expectOne(accountsURL(7)).flush([]);
    expect(component.chosen()).toBe('7');
  });

  it('survives a created cost type coming back without a body', () => {
    const { component } = loaded([type(1)]);

    component.code.set('4711');
    component.name.set('Personalkosten');
    component.create(new Event('submit'));
    http.expectOne('/api/klr/cost-types').flush(null);

    expect(component.notice()).toBe('Kostenart angelegt.');
    http.expectOne('/api/klr/cost-types').flush([type(1)]);
    http.expectOne(accountsURL(1)).flush([]);
  });

  it('reports a cost type that could not be created', () => {
    const { component } = loaded([]);

    component.code.set('4711');
    component.name.set('Personalkosten');
    component.create(new Event('submit'));
    http.expectOne('/api/klr/cost-types').error(new ProgressEvent('error'));

    expect(component.saving()).toBeFalse();
    expect(component.actionError()).toBe('Konnte die Kostenart nicht anlegen.');
    expect(component.notice()).toBeNull();
  });

  it('refuses to map an account without a number', () => {
    const { component } = loaded();

    component.accountCode.set('   ');
    component.map(new Event('submit'));

    expect(component.actionError()).toBe('Eine Kontonummer ist erforderlich.');
    expect(component.mapping()).toBeFalse();
  });

  it('maps an account to the chosen cost type and reloads its accounts', () => {
    const { component } = loaded([type(1)], []);

    component.accountCode.set('  6810  ');
    component.map(new Event('submit'));
    expect(component.mapping()).toBeTrue();

    const request = http.expectOne(accountsURL(1));
    expect(request.request.method).toBe('POST');
    // The code is sent as typed but trimmed.
    expect(request.request.body).toEqual({ account_code: '6810' });
    request.flush(account(1, '6810'));

    expect(component.mapping()).toBeFalse();
    expect(component.notice()).toBe('Konto zugeordnet.');
    expect(component.accountCode()).toBe('');

    http.expectOne(accountsURL(1)).flush([account(1, '6810')]);
    expect(component.accounts().map((a) => a.AccountCode)).toEqual(['6810']);
  });

  it('reports an account that could not be mapped', () => {
    const { component } = loaded();

    component.accountCode.set('6810');
    component.map(new Event('submit'));
    http.expectOne(accountsURL(1)).error(new ProgressEvent('error'));

    expect(component.mapping()).toBeFalse();
    expect(component.actionError()).toBe('Konnte das Konto nicht zuordnen.');
    expect(component.notice()).toBeNull();
  });

  it('refuses to look up a blank account', () => {
    const { component } = loaded();

    component.lookupCode.set('  ');
    component.lookup(new Event('submit'));

    expect(component.actionError()).toBe('Eine Kontonummer ist erforderlich.');
  });

  it('looks an account up and names the cost type it belongs to', () => {
    const { fixture, component } = loaded();

    component.lookupResult.set(type(9));
    component.lookupMiss.set(true);
    component.lookupCode.set('  6810 ');
    component.lookup(new Event('submit'));

    // The previous answer is cleared before the new one is asked for.
    expect(component.lookupResult()).toBeNull();
    expect(component.lookupMiss()).toBeFalse();

    const request = http.expectOne('/api/klr/accounts/6810/cost-type');
    expect(request.request.method).toBe('GET');
    request.flush(type(3, { Code: '5000', Name: 'Sachkosten' }));
    fixture.detectChanges();

    expect(component.lookupResult()?.Code).toBe('5000');
    const shown = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="lookup-result"]');
    expect(shown?.textContent).toContain('5000');
    expect(shown?.textContent).toContain('Sachkosten');
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-testid="lookup-miss"]'),
    ).toBeNull();
  });

  it('says so when an account has no cost type — a 404 is an answer, not a failure', () => {
    const { fixture, component } = loaded();

    component.lookupCode.set('9999');
    component.lookup(new Event('submit'));
    http.expectOne('/api/klr/accounts/9999/cost-type').flush(
      { title: 'klr: no cost type is mapped to that account' },
      { status: 404, statusText: 'Not Found' },
    );
    fixture.detectChanges();

    expect(component.lookupMiss()).toBeTrue();
    expect(component.lookupResult()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-testid="lookup-miss"]'),
    ).not.toBeNull();
  });

  it('leaves the lookup form empty until it is used', () => {
    const { fixture } = loaded();
    const html = fixture.nativeElement as HTMLElement;

    expect(html.querySelector('[data-testid="lookup-result"]')).toBeNull();
    expect(html.querySelector('[data-testid="lookup-miss"]')).toBeNull();
    expect(html.querySelector('[data-testid="lookup-account"]')).not.toBeNull();
  });
});
