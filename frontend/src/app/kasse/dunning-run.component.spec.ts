import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { DunningRunComponent, skipReasonLabel } from './dunning-run.component';
import { DunningRunOutcome, DunningRunResult } from '../core/api/generated';

describe('DunningRunComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [DunningRunComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const outcome = (over: Partial<DunningRunOutcome> = {}): DunningRunOutcome => ({
    ReceivableID: 7,
    ClaimReference: 'receivable/7',
    MandateReference: 'MAN-1',
    AmountCents: 10000,
    DueDate: '2026-10-15T00:00:00Z',
    LevelCode: '1',
    ChargeCents: 500,
    NoticeID: 9,
    DocumentKey: 'dunning/9.pdf',
    Issued: true,
    Reason: '',
    Detail: '',
    ...over,
  });

  const report = (over: Partial<DunningRunResult> = {}): DunningRunResult => ({
    Year: 2026,
    AsOf: '2026-11-06T00:00:00Z',
    Issued: 1,
    Skipped: 0,
    Failed: 0,
    Outcomes: [outcome()],
    ...over,
  });

  function screen(): DunningRunComponent {
    const fixture = TestBed.createComponent(DunningRunComponent);
    fixture.detectChanges();
    return fixture.componentInstance;
  }

  it('starts with the current year and day, and no result', () => {
    const component = screen();
    const now = new Date();
    const month = String(now.getMonth() + 1).padStart(2, '0');
    const day = String(now.getDate()).padStart(2, '0');
    expect(component.year()).toBe(String(now.getFullYear()));
    expect(component.asOf()).toBe(`${now.getFullYear()}-${month}-${day}`);
    expect(component.running()).toBeFalse();
    expect(component.result()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.notice()).toBeNull();
  });

  it('accepts the year boundaries and tolerates surrounding spaces', () => {
    const component = screen();

    for (const year of ['1900', '2999', ' 2026 ']) {
      component.year.set(year);
      component.asOf.set('2026-01-10');
      component.run(new Event('submit'));
      const req = http.expectOne('/api/dunning/runs');
      expect(req.request.body.year).toBe(Number(year.trim()));
      req.flush(report());
    }
  });

  it('refuses a date with trailing or leading junk', () => {
    const component = screen();
    component.year.set('2026');

    component.asOf.set('2026-01-10x');
    component.run(new Event('submit'));
    expect(component.actionError()).toBe('Ein Stichtag ist erforderlich.');

    component.asOf.set('x2026-01-10');
    component.run(new Event('submit'));
    expect(component.actionError()).toBe('Ein Stichtag ist erforderlich.');
    http.expectNone('/api/dunning/runs');
  });

  it('posts the year and the as-of date, and reports the run', () => {
    const fixture = TestBed.createComponent(DunningRunComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;

    component.year.set('2026');
    component.asOf.set('2026-01-10');
    component.run(new Event('submit'));

    expect(component.running()).toBeTrue();
    const req = http.expectOne('/api/dunning/runs');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ year: 2026, as_of: '2026-01-10' });
    req.flush(report());
    fixture.detectChanges();

    expect(component.running()).toBeFalse();
    expect(component.result()).toEqual(report());
    expect(component.notice()).toBe(
      'Mahnlauf abgeschlossen: 1 ausgestellt, 0 übersprungen, 0 fehlgeschlagen.'
    );
    expect(fixture.nativeElement.textContent).toContain('receivable/7');
  });

  it('reports a failed run and clears the previous result', () => {
    const fixture = TestBed.createComponent(DunningRunComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;

    component.year.set('2026');
    component.asOf.set('2026-01-10');
    component.result.set(report());
    component.run(new Event('submit'));
    expect(component.result()).toBeNull();
    http.expectOne('/api/dunning/runs').flush({ error: 'conflict' }, { status: 409, statusText: 'Conflict' });

    expect(component.running()).toBeFalse();
    expect(component.result()).toBeNull();
    expect(component.actionError()).toBe('Der Mahnlauf konnte nicht ausgeführt werden.');
  });

  it('refuses an impossible year before asking the API', () => {
    const component = screen();
    component.year.set('1800');
    component.run(new Event('submit'));
    expect(component.actionError()).toBe('Das Jahr muss zwischen 1900 und 2999 liegen.');

    component.year.set('3000');
    component.run(new Event('submit'));
    expect(component.actionError()).toBe('Das Jahr muss zwischen 1900 und 2999 liegen.');

    component.year.set('abc');
    component.run(new Event('submit'));
    expect(component.actionError()).toBe('Das Jahr muss zwischen 1900 und 2999 liegen.');
    http.expectNone('/api/dunning/runs');
  });

  it('refuses a missing or malformed as-of date before asking the API', () => {
    const component = screen();
    component.year.set('2026');

    component.asOf.set('');
    component.run(new Event('submit'));
    expect(component.actionError()).toBe('Ein Stichtag ist erforderlich.');

    component.asOf.set('06.11.2026');
    component.run(new Event('submit'));
    expect(component.actionError()).toBe('Ein Stichtag ist erforderlich.');
    http.expectNone('/api/dunning/runs');
  });

  it('shows an empty run and only links documents that exist', () => {
    const fixture = TestBed.createComponent(DunningRunComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;

    component.year.set('2026');
    component.asOf.set('2026-01-10');
    component.run(new Event('submit'));
    http.expectOne('/api/dunning/runs').flush(report({ Issued: 0, Skipped: 0, Outcomes: [] }));
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Kein offener Posten war am Stichtag fällig.');

    const withLink = TestBed.createComponent(DunningRunComponent);
    withLink.detectChanges();
    const second = withLink.componentInstance;
    second.year.set('2026');
    second.asOf.set('2026-01-10');
    second.run(new Event('submit'));
    http.expectOne('/api/dunning/runs').flush(report({
      Outcomes: [outcome({ DocumentKey: '' }), outcome({ ReceivableID: 8, NoticeID: 0, DocumentKey: '' })],
    }));
    withLink.detectChanges();
    expect(withLink.nativeElement.querySelector('a[href="/api/dunning/notices/9/document"]')).toBeNull();
    expect(withLink.nativeElement.textContent).toContain('ausgestellt');
  });

  it('renders a skipped claim with its reason and no charge', () => {
    const fixture = TestBed.createComponent(DunningRunComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;

    component.year.set('2026');
    component.asOf.set('2026-01-10');
    component.run(new Event('submit'));
    http.expectOne('/api/dunning/runs').flush(report({
      Issued: 0,
      Skipped: 1,
      Outcomes: [{
        ...outcome({ Issued: false, NoticeID: 0, ChargeCents: 0, DocumentKey: '', Reason: 'not_due' }),
      }],
    }));
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[data-testid="run-7"]').textContent).toContain(
      'Frist nicht verstrichen'
    );
    expect(fixture.nativeElement.querySelector('a[href^="/api/dunning/notices/"]')).toBeNull();
  });

  it('formats amounts and days for the operator', () => {
    const component = screen();
    expect(component.amount(123456)).toBe('1.234,56');
    expect(component.day('2026-10-15T00:00:00Z')).toBe('15.10.2026');
  });

  it('labels every skip reason and falls back to the raw code', () => {
    expect(skipReasonLabel('already_dunned')).toBe('am Stichtag bereits gemahnt');
    expect(skipReasonLabel('not_due')).toBe('Frist nicht verstrichen');
    expect(skipReasonLabel('unknown_level')).toBe('Stufe unbekannt');
    expect(skipReasonLabel('no_next_level')).toBe('keine weitere Stufe');
    expect(skipReasonLabel('next_level_missing')).toBe('nächste Stufe fehlt');
    expect(skipReasonLabel('issue_failed')).toBe('Ausstellung fehlgeschlagen');
    expect(skipReasonLabel('something_else')).toBe('something_else');

    const component = screen();
    expect(component.reason('not_due')).toBe('Frist nicht verstrichen');
    expect(component.outcomeLabel(outcome())).toBe('ausgestellt');
    expect(component.outcomeLabel(outcome({ Issued: false, Reason: 'no_next_level' }))).toBe(
      'keine weitere Stufe'
    );
  });
});
