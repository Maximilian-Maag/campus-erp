import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { CashAccountsComponent } from './cash-accounts.component';
import { CashAccount } from '../core/api/generated';

describe('CashAccountsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [CashAccountsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const account = (ID: number, Name: string, over: Partial<CashAccount> = {}): CashAccount => ({
    ID, Name, IBAN: 'DE02120300000000202051', BIC: 'BYLADEM1001', Currency: 'EUR', ...over,
  });
  const uni = account(1, 'Universität');

  /** The screen with its accounts loaded. */
  function loaded(accounts: CashAccount[] = [uni]) {
    const fixture = TestBed.createComponent(CashAccountsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush(accounts);
    fixture.detectChanges();
    return fixture;
  }

  it('starts unloaded and empty', () => {
    const fixture = TestBed.createComponent(CashAccountsComponent);
    const component = fixture.componentInstance;

    expect(component.accounts()).toEqual([]);
    expect(component.loading()).toBeTrue();
    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.loadError()).toBeNull();
    // The form starts blank, with the currency defaulting.
    expect(component.name()).toBe('');
    expect(component.iban()).toBe('');
    expect(component.bic()).toBe('');
    expect(component.currency()).toBe('EUR');
  });

  it('loads the accounts', () => {
    const fixture = loaded([uni, account(2, 'Zweitkonto')]);
    const component = fixture.componentInstance;

    expect(component.loading()).toBeFalse();
    expect(component.accounts().map((a) => a.Name)).toEqual(['Universität', 'Zweitkonto']);
  });

  it('shows the loading state again on a reload', () => {
    const fixture = loaded([uni]);
    const component = fixture.componentInstance;

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/cash-accounts').flush([account(2, 'Zweitkonto')]);
    fixture.detectChanges();

    expect(component.loading()).toBeFalse();
    expect(component.accounts().map((a) => a.Name)).toEqual(['Zweitkonto']);
  });

  it('treats an empty response as no accounts and says so', () => {
    const fixture = TestBed.createComponent(CashAccountsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.accounts()).toEqual([]);
    expect(component.loading()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('kein eigenes Konto angelegt');
  });

  it('reports a failed load instead of an empty list', () => {
    const fixture = TestBed.createComponent(CashAccountsComponent);
    fixture.detectChanges();
    http.expectOne('/api/cash-accounts').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Konten nicht laden.');
    expect(component.loading()).toBeFalse();
  });

  it('creates an account and joins it to the list', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;
    component.name.set('  Universität  ');
    component.iban.set(' DE02120300000000202051 ');
    component.bic.set(' BYLADEM1001 ');
    component.currency.set(' eur ');

    component.create(new Event('submit'));

    expect(component.submitting()).toBeTrue();
    const post = http.expectOne('/api/cash-accounts');
    expect(post.request.method).toBe('POST');
    // Every field is trimmed, so a padded paste still lands as the API expects it.
    expect(post.request.body).toEqual({
      name: 'Universität',
      iban: 'DE02120300000000202051',
      bic: 'BYLADEM1001',
      currency: 'eur',
    });
    post.flush(account(7, 'Universität'));
    fixture.detectChanges();

    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBe('Konto angelegt.');
    expect(component.accounts().map((a) => a.ID)).toEqual([7]);
    // The form is cleared for the next account, with the currency back to its default.
    expect(component.name()).toBe('');
    expect(component.iban()).toBe('');
    expect(component.bic()).toBe('');
    expect(component.currency()).toBe('EUR');
    const row = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="account-7"]');
    expect(row?.textContent).toContain('Universität');
  });

  it('refuses a blank name, IBAN or currency without asking the API', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;

    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Ein Name ist erforderlich.');

    component.name.set('Universität');
    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Eine IBAN ist erforderlich.');

    // A whitespace-only IBAN is a missing IBAN.
    component.iban.set('   ');
    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Eine IBAN ist erforderlich.');

    component.iban.set('DE02120300000000202051');
    component.currency.set('  ');
    component.create(new Event('submit'));
    expect(component.actionError()).toBe('Eine Währung ist erforderlich.');

    expect(component.submitting()).toBeFalse();
    http.expectNone('/api/cash-accounts');
  });

  it('names the failure when the account is refused, and keeps what was typed', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;
    component.name.set('Universität');
    component.iban.set('DE00');

    component.create(new Event('submit'));
    http.expectOne('/api/cash-accounts').flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte das Konto nicht anlegen.');
    expect(component.submitting()).toBeFalse();
    expect(component.accounts()).toEqual([]);
    expect(component.iban()).toBe('DE00');
  });
});
