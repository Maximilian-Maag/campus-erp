import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { DunningComponent } from './dunning.component';
import { DunningLevel, DunningNotice } from '../core/api/generated';

describe('DunningComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [DunningComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const level = (over: Partial<DunningLevel> = {}): DunningLevel => ({
    ID: 1,
    Year: 2026,
    Code: '1',
    Description: '1. Mahnung',
    DaysAllowed: 14,
    Active: true,
    NextCode: '2',
    Bands: [
      { UpToCents: 5000, FeeCents: 500, RateBasisPoints: 0 },
      { UpToCents: 0, FeeCents: 2500, RateBasisPoints: 200 },
    ],
    ...over,
  });

  const issuedNotice = (over: Partial<DunningNotice> = {}): DunningNotice => ({
    ID: 9,
    ClaimReference: 'F-2026-001',
    LevelCode: '1',
    OutstandingCents: 12000,
    ChargeCents: 2740,
    IssuedOn: '2026-11-05T00:00:00Z',
    IssuedBy: 'alice',
    DocumentKey: 'dunning/9.pdf',
    ...over,
  });

  /** The screen with its levels loaded. */
  function loaded(levels: DunningLevel[] = [level()]) {
    const fixture = TestBed.createComponent(DunningComponent);
    fixture.detectChanges();
    http.expectOne('/api/dunning/levels').flush(levels);
    fixture.detectChanges();
    return fixture;
  }

  it('starts unloaded and empty', () => {
    const fixture = TestBed.createComponent(DunningComponent);
    const component = fixture.componentInstance;

    expect(component.levels()).toEqual([]);
    expect(component.year()).toBe('');
    expect(component.loading()).toBeTrue();
    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.loadError()).toBeNull();
    expect(component.issued()).toBeNull();
    expect(component.noticeLevelKey()).toBe('');
    // The level form starts with one empty band and a 14-day period.
    expect(component.levelDays()).toBe('14');
    expect(component.levelActive()).toBeTrue();
    expect(component.bands()).toEqual([{ upTo: '', fee: '', rate: '' }]);
    expect(component.levelCode()).toBe('');
    expect(component.levelDescription()).toBe('');
    expect(component.levelNext()).toBe('');
    expect(component.levelYear()).toBe(String(new Date().getFullYear()));
    // The notice form starts blank.
    expect(component.claimReference()).toBe('');
    expect(component.outstanding()).toBe('');
  });

  it('loads the levels and points the notice picker at the first one', () => {
    const fixture = loaded([level({ ID: 7, Code: 'A' }), level({ ID: 8, Code: 'B' })]);
    const component = fixture.componentInstance;

    expect(component.loading()).toBeFalse();
    expect(component.levels().length).toBe(2);
    expect(component.noticeLevelKey()).toBe('2026|A');
  });

  it('asks the server for the chosen year, and for every year when cleared', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    // A padded year is the same year.
    component.filterTo(' 2026 ');
    expect(component.year()).toBe(' 2026 ');
    http.expectOne('/api/dunning/levels?year=2026').flush([level()]);
    fixture.detectChanges();

    component.filterTo('');
    expect(component.year()).toBe('');
    http.expectOne('/api/dunning/levels').flush([]);
    fixture.detectChanges();

    // Nothing is on offer, so the picker holds nothing.
    expect(component.noticeLevelKey()).toBe('');
  });

  it('treats an empty response as no levels', () => {
    const fixture = TestBed.createComponent(DunningComponent);
    fixture.detectChanges();
    http.expectOne('/api/dunning/levels').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.levels()).toEqual([]);
    expect(component.loading()).toBeFalse();
  });

  it('keeps the picker on a level the reload still offers', () => {
    const fixture = loaded([level({ Code: 'A' }), level({ ID: 2, Code: 'B' })]);
    const component = fixture.componentInstance;
    // The second level, not the first: a reload must not walk the choice back to the top.
    component.noticeLevelKey.set('2026|B');

    component.load();
    expect(component.loading()).toBeTrue();
    http.expectOne('/api/dunning/levels').flush([level({ Code: 'A' }), level({ ID: 2, Code: 'B' })]);
    fixture.detectChanges();

    expect(component.loading()).toBeFalse();
    expect(component.noticeLevelKey()).toBe('2026|B');
  });

  it('reports a failed load instead of an empty list', () => {
    const fixture = TestBed.createComponent(DunningComponent);
    fixture.detectChanges();
    http.expectOne('/api/dunning/levels').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Mahnstufen nicht laden.');
    expect(component.loading()).toBeFalse();
  });

  it('adds, edits and removes band rows', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    component.addBand();
    expect(component.bands().length).toBe(2);
    // The new row is blank, not a copy of the one before it.
    expect(component.bands()[1]).toEqual({ upTo: '', fee: '', rate: '' });

    component.setBand(1, 'fee', '10,00');
    expect(component.bands()[1].fee).toBe('10,00');
    // The other row is untouched.
    expect(component.bands()[0].fee).toBe('');

    component.removeBand(0);
    expect(component.bands().length).toBe(1);
    expect(component.bands()[0].fee).toBe('10,00');
  });

  it('defines a level, parsing euros and percent into what the API stores', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.levelYear.set(' 2027 ');
    component.levelCode.set(' 2 ');
    component.levelDescription.set(' 2. Mahnung ');
    component.levelDays.set(' 7 ');
    component.levelNext.set(' 3 ');
    component.levelActive.set(false);
    component.bands.set([
      { upTo: '50,00', fee: '5,00', rate: '0' },
      { upTo: '', fee: '', rate: '' }, // an open-ended band with no fee and no interest
      { upTo: '0,00', fee: '0,0', rate: '1,5' }, // the zero forms an open band is written in
    ]);

    component.defineLevel(new Event('submit'));

    expect(component.submitting()).toBeTrue();
    const post = http.expectOne('/api/dunning/levels');
    expect(post.request.method).toBe('POST');
    expect(post.request.body).toEqual({
      year: 2027,
      code: '2',
      description: '2. Mahnung',
      days_allowed: 7,
      active: false,
      next_code: '3',
      bands: [
        { up_to_cents: 5000, fee_cents: 500, rate_basis_points: 0 },
        { up_to_cents: 0, fee_cents: 0, rate_basis_points: 0 },
        { up_to_cents: 0, fee_cents: 0, rate_basis_points: 150 },
      ],
    });
    post.flush(level());
    // The list is the server's: the screen reloads it and shows what was stored.
    http.expectOne('/api/dunning/levels').flush([level({ Code: '2' })]);
    fixture.detectChanges();

    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBe('Mahnstufe angelegt.');
    expect(component.levelCode()).toBe('');
    expect(component.levelDescription()).toBe('');
    expect(component.bands()).toEqual([{ upTo: '', fee: '', rate: '' }]);
  });

  it('refuses an incomplete level without asking the API', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    component.defineLevel(new Event('submit'));
    expect(component.actionError()).toBe('Ein Kürzel ist erforderlich.');

    component.levelCode.set('1');
    component.levelYear.set('0');
    component.defineLevel(new Event('submit'));
    expect(component.actionError()).toBe('Ein Jahr ist erforderlich.');

    component.levelYear.set('2026');
    component.levelDays.set('1,5');
    component.bands.set([{ upTo: '50,00', fee: '5,00', rate: '1' }]);
    component.defineLevel(new Event('submit'));
    expect(component.actionError()).toBe('Die Frist muss eine ganze Zahl von Tagen sein.');

    // A negative period is not a period either.
    component.levelDays.set('-5');
    component.defineLevel(new Event('submit'));
    expect(component.actionError()).toBe('Die Frist muss eine ganze Zahl von Tagen sein.');

    component.levelDays.set('14');
    component.bands.set([{ upTo: '50,00', fee: '5,00', rate: '100,5' }]);
    component.defineLevel(new Event('submit'));
    expect(component.actionError()).toBe('Jede Staffel braucht Beträge und einen Zinssatz zwischen 0 und 100 %.');
    expect(component.submitting()).toBeFalse();

    component.bands.set([{ upTo: 'abc', fee: '5,00', rate: '1' }]);
    component.defineLevel(new Event('submit'));
    expect(component.actionError()).toBe('Jede Staffel braucht Beträge und einen Zinssatz zwischen 0 und 100 %.');

    // The fee is parsed too, not only the bound.
    component.bands.set([{ upTo: '50,00', fee: 'xyz', rate: '1' }]);
    component.defineLevel(new Event('submit'));
    expect(component.actionError()).toBe('Jede Staffel braucht Beträge und einen Zinssatz zwischen 0 und 100 %.');
    http.expectNone('/api/dunning/levels');
  });

  it('takes a band bound or fee of zero in every form it is written in', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.levelCode.set('1');
    // A blank period is no period, not a refusal.
    component.levelDays.set('   ');
    component.bands.set([
      { upTo: '0', fee: '0', rate: '   ' },
      { upTo: '00', fee: '0,0', rate: '' },
      { upTo: ' 0 ', fee: '0,00', rate: '0' },
      { upTo: '0,00', fee: ' 0 ', rate: '' },
      { upTo: '0,50', fee: '0,10', rate: '1' }, // a real sub-euro amount is not zero
    ]);

    component.defineLevel(new Event('submit'));

    const post = http.expectOne('/api/dunning/levels');
    const body = post.request.body as { days_allowed: number; bands: unknown[] };
    expect(body.days_allowed).toBe(0);
    expect(body.bands).toEqual([
      { up_to_cents: 0, fee_cents: 0, rate_basis_points: 0 },
      { up_to_cents: 0, fee_cents: 0, rate_basis_points: 0 },
      { up_to_cents: 0, fee_cents: 0, rate_basis_points: 0 },
      { up_to_cents: 0, fee_cents: 0, rate_basis_points: 0 },
      { up_to_cents: 50, fee_cents: 10, rate_basis_points: 100 },
    ]);
    post.flush(level());
    http.expectOne('/api/dunning/levels').flush([]);
  });

  it('names the failure when the level is refused', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.levelCode.set('1');

    component.defineLevel(new Event('submit'));
    http.expectOne('/api/dunning/levels').flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Mahnstufe nicht anlegen.');
    expect(component.submitting()).toBeFalse();
    expect(component.levelCode()).toBe('1');
  });

  it('issues a notice for the chosen level and shows the charge that came back', () => {
    const fixture = loaded([level({ Year: 2027, Code: 'B' })]);
    const component = fixture.componentInstance;
    component.claimReference.set(' F-2026-001 ');
    component.outstanding.set('120,00');

    component.issue(new Event('submit'));

    expect(component.submitting()).toBeTrue();
    const post = http.expectOne('/api/dunning/notices');
    expect(post.request.method).toBe('POST');
    // The level's year and code travel from the picker, not from the level form.
    expect(post.request.body).toEqual({
      year: 2027,
      level_code: 'B',
      claim_reference: 'F-2026-001',
      outstanding_cents: 12000,
    });
    post.flush(issuedNotice({ LevelCode: 'B' }));
    fixture.detectChanges();

    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBe('Mahnung ausgestellt.');
    expect(component.issued()?.ChargeCents).toBe(2740);
    expect(component.claimReference()).toBe('');
    expect(component.outstanding()).toBe('');

    const row = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="notice-9"]');
    expect(row?.textContent).toContain('27,40');
    expect(row?.textContent).toContain('05.11.2026');
  });

  it('links the PDF only when the notice carries one, and keeps the fields on a refusal', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.claimReference.set('F-1');
    component.outstanding.set('10,00');

    component.issue(new Event('submit'));
    http.expectOne('/api/dunning/notices').flush(issuedNotice({ DocumentKey: '' }));
    fixture.detectChanges();

    let html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('a[href*="/document"]')).toBeNull();
    expect(html.textContent).toContain('kein Dokument erzeugt');

    // A second notice, this time with a document, and with the API refusing it.
    component.claimReference.set('F-2');
    component.outstanding.set('10,00');
    component.issue(new Event('submit'));
    http.expectOne('/api/dunning/notices').flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Mahnung nicht ausstellen.');
    expect(component.claimReference()).toBe('F-2');
    expect(component.submitting()).toBeFalse();

    component.claimReference.set('F-3');
    component.issue(new Event('submit'));
    http.expectOne('/api/dunning/notices').flush(issuedNotice({ ID: 11 }));
    fixture.detectChanges();

    html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('a[href="/api/dunning/notices/11/document"]')).toBeTruthy();
  });

  it('refuses an incomplete notice without asking the API', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    component.issue(new Event('submit'));
    expect(component.actionError()).toBe('Eine Vorgangsnummer ist erforderlich.');

    component.claimReference.set('F-1');
    component.issue(new Event('submit'));
    expect(component.actionError()).toBe('Ein positiver offener Betrag ist erforderlich.');

    component.outstanding.set('0,00');
    component.issue(new Event('submit'));
    expect(component.actionError()).toBe('Ein positiver offener Betrag ist erforderlich.');

    // No level on offer: there is nothing to issue against.
    component.noticeLevelKey.set('');
    component.outstanding.set('10,00');
    component.issue(new Event('submit'));
    expect(component.actionError()).toBe('Eine Mahnstufe ist erforderlich.');

    // A year without a code, and a level code without a year, are both incomplete.
    component.noticeLevelKey.set('2026|');
    component.issue(new Event('submit'));
    expect(component.actionError()).toBe('Eine Mahnstufe ist erforderlich.');

    // A key that never had a code at all is incomplete too.
    component.noticeLevelKey.set('2026');
    component.issue(new Event('submit'));
    expect(component.actionError()).toBe('Eine Mahnstufe ist erforderlich.');

    component.noticeLevelKey.set('0|1');
    component.issue(new Event('submit'));
    expect(component.actionError()).toBe('Eine Mahnstufe ist erforderlich.');

    expect(component.submitting()).toBeFalse();
    http.expectNone('/api/dunning/notices');
  });

  it('clears the previous result when a new submission starts', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;
    component.claimReference.set('F-1');
    component.outstanding.set('10,00');
    component.issue(new Event('submit'));
    http.expectOne('/api/dunning/notices').flush(issuedNotice());
    fixture.detectChanges();

    expect(component.issued()).not.toBeNull();
    expect(component.notice()).toBe('Mahnung ausgestellt.');

    // Starting the other form clears what the last one showed, so a stale notice cannot be
    // mistaken for this attempt's result.
    component.levelCode.set('1');
    component.defineLevel(new Event('submit'));
    expect(component.issued()).toBeNull();
    expect(component.notice()).toBeNull();
    http.expectOne('/api/dunning/levels').flush(level());
    http.expectOne('/api/dunning/levels').flush([]);
  });

  it('renders a level and its bands as the operator reads them', () => {
    const fixture = loaded();
    const component = fixture.componentInstance;

    expect(component.levelKey(level())).toBe('2026|1');
    expect(component.levelLabel(level())).toBe('2026 / 1 — 1. Mahnung');
    expect(component.levelLabel(level({ Description: '' }))).toBe('2026 / 1');
    expect(component.bandLabel({ UpToCents: 5000, FeeCents: 500, RateBasisPoints: 0 })).toBe('bis 50,00: 5,00 + 0 %');
    expect(component.bandLabel({ UpToCents: 0, FeeCents: 2500, RateBasisPoints: 200 })).toBe('offen: 25,00 + 2 %');
    expect(component.documentUrl(9)).toBe('/api/dunning/notices/9/document');
  });
});
