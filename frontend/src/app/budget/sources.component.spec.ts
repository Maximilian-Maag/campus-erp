import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { BudgetSourcesComponent } from './sources.component';
import { BudgetPositionResponse, Center, Position, Source } from '../core/api/generated';

describe('BudgetSourcesComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [BudgetSourcesComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const source = (over: Partial<Source> = {}): Source => ({
    ID: 1,
    Code: 'LHH',
    Name: 'Landesmittel',
    Chapter: '05',
    Active: true,
    CreatedBy: 'alice',
    ...over,
  });

  const center = (over: Partial<Center> = {}): Center => ({
    ID: 2,
    Code: 'BW-1',
    Name: 'Fakultät 1',
    Active: true,
    CreatedBy: 'alice',
    ...over,
  });

  const position = (over: Partial<Position> = {}): Position => ({
    ID: 7,
    Year: 2026,
    Chapter: '05',
    Title: '1234',
    Description: 'Beschaffung',
    AllocatedCents: 100000,
    CreatedBy: 'alice',
    SourceID: null,
    CenterID: null,
    ...over,
  });

  const row = (over: Partial<Position> = {}): BudgetPositionResponse => ({
    position: position(over),
    balance: { PositionID: 7, AllocatedCents: 100000, CommittedCents: 0, ActualCents: 0 },
    available_cents: 100000,
  });

  let fixture: ComponentFixture<BudgetSourcesComponent>;
  let component: BudgetSourcesComponent;
  let el: HTMLElement;

  /** Creates the screen and answers the three reads it starts with. */
  function screen(
    sources: Source[] = [source()],
    centers: Center[] = [center()],
    positions: BudgetPositionResponse[] = [row()]
  ): void {
    fixture = TestBed.createComponent(BudgetSourcesComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    el = fixture.nativeElement as HTMLElement;
    http.expectOne('/api/budget/sources').flush(sources);
    http.expectOne('/api/budget/centers').flush(centers);
    http.expectOne('/api/budget/positions').flush(positions);
    fixture.detectChanges();
  }

  it('lists the sources, the centres and the positions they name', () => {
    screen([source()], [center()], [row({ SourceID: 1, CenterID: 2 })]);
    expect(component.loading()).toBeFalse();
    expect(component.loadError()).toBeNull();
    expect(el.querySelector('[data-testid="source-1"]')!.textContent).toContain('Landesmittel');
    expect(el.querySelector('[data-testid="center-2"]')!.textContent).toContain('Fakultät 1');
    const cells = el.querySelector('[data-testid="position-7"]')!.querySelectorAll('td');
    expect(cells[0].textContent).toContain('05/1234');
    expect(cells[2].textContent).toContain('1.000,00');
    expect(cells[3].textContent).toContain('LHH — Landesmittel');
    expect(cells[4].textContent).toContain('BW-1 — Fakultät 1');
  });

  it('shows an unnamed position as having none, and states empty lists', () => {
    screen([], [], [row()]);
    expect(el.textContent).toContain('keine Mittelquelle angelegt');
    expect(el.textContent).toContain('keine Bewirtschaftungsstelle angelegt');
    const cells = el.querySelector('[data-testid="position-7"]')!.querySelectorAll('td');
    expect(cells[3].textContent).toContain('keine');
    expect(cells[4].textContent).toContain('keine');
  });

  it('states that there is no position at all', () => {
    screen([source()], [center()], []);
    expect(el.textContent).toContain('keine Haushaltsstelle angelegt');
    expect(el.querySelector('#assign-position')).toBeNull();
  });

  it('reports a failed read', () => {
    fixture = TestBed.createComponent(BudgetSourcesComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    el = fixture.nativeElement as HTMLElement;
    http.expectOne('/api/budget/sources').flush(
      { error: 'boom' },
      { status: 500, statusText: 'Server Error' }
    );
    http.expectOne('/api/budget/centers').flush([center()]);
    http.expectOne('/api/budget/positions').flush([row()]);
    fixture.detectChanges();

    expect(component.loadError()).toBe('Die Mittelherkunft konnte nicht geladen werden.');
    expect(el.textContent).toContain('Die Mittelherkunft konnte nicht geladen werden.');
  });

  it('creates a source and keeps the list in code order', () => {
    screen([source()], [], []);
    component.sourceCode.set('S-1');
    component.sourceName.set('Stipendien');
    component.sourceChapter.set('07');
    component.createSource(new Event('submit'));

    expect(component.busy()).toBeTrue();
    const req = http.expectOne('/api/budget/sources');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ code: 'S-1', name: 'Stipendien', chapter: '07' });
    req.flush(source({ ID: 3, Code: 'A-1', Name: 'Drittmittel', Chapter: '' }));
    fixture.detectChanges();

    expect(component.busy()).toBeFalse();
    expect(component.notice()).toBe('Mittelquelle A-1 angelegt.');
    expect(component.sources().map((s) => s.Code)).toEqual(['A-1', 'LHH']);
    // The form is cleared for the next one.
    expect(component.sourceCode()).toBe('');
    expect(component.sourceName()).toBe('');
    expect(component.sourceChapter()).toBe('');
  });

  it('creates a centre', () => {
    screen([], [center()], []);
    component.centerCode.set('BW-2');
    component.centerName.set('Fakultät 2');
    component.createCenter(new Event('submit'));

    const req = http.expectOne('/api/budget/centers');
    expect(req.request.body).toEqual({ code: 'BW-2', name: 'Fakultät 2' });
    req.flush(center({ ID: 4, Code: 'BW-2', Name: 'Fakultät 2' }));
    fixture.detectChanges();

    expect(component.notice()).toBe('Bewirtschaftungsstelle BW-2 angelegt.');
    expect(component.centers().map((c) => c.Code)).toEqual(['BW-1', 'BW-2']);
    expect(component.centerCode()).toBe('');
  });

  it('refuses an incomplete source or centre before asking the API', () => {
    screen();
    component.createSource(new Event('submit'));
    expect(component.actionError()).toBe('Schlüssel und Bezeichnung der Mittelquelle sind erforderlich.');
    component.sourceCode.set('X');
    component.createSource(new Event('submit'));
    expect(component.actionError()).toBe('Schlüssel und Bezeichnung der Mittelquelle sind erforderlich.');

    component.createCenter(new Event('submit'));
    expect(component.actionError()).toBe(
      'Schlüssel und Bezeichnung der Bewirtschaftungsstelle sind erforderlich.'
    );
    http.expectNone((r) => r.method === 'POST');
    expect(component.busy()).toBeFalse();
  });

  it('reports a refused create', () => {
    screen();
    component.sourceCode.set('LHH');
    component.sourceName.set('Nochmal');
    component.createSource(new Event('submit'));
    http.expectOne('/api/budget/sources').flush({ error: 'duplicate' }, { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();

    expect(component.busy()).toBeFalse();
    expect(component.actionError()).toBe('Die Mittelquelle konnte nicht angelegt werden.');
    expect(component.notice()).toBeNull();

    component.centerCode.set('BW-9');
    component.centerName.set('Neun');
    component.createCenter(new Event('submit'));
    http.expectOne('/api/budget/centers').flush({ error: 'boom' }, { status: 500, statusText: 'Server Error' });
    expect(component.actionError()).toBe('Die Bewirtschaftungsstelle konnte nicht angelegt werden.');
  });

  it('assigns the chosen position its source and centre', () => {
    screen();
    component.position.set('7');
    component.source.set('1');
    component.center.set('2');
    component.assign(new Event('submit'));

    const req = http.expectOne('/api/budget/positions/7/assignment');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ source_id: 1, center_id: 2 });
    req.flush(position({ SourceID: 1, CenterID: 2 }));
    fixture.detectChanges();

    expect(component.notice()).toBe('Die Zuordnung ist gespeichert.');
    const cells = el.querySelector('[data-testid="position-7"]')!.querySelectorAll('td');
    expect(cells[3].textContent).toContain('LHH — Landesmittel');
    expect(cells[4].textContent).toContain('BW-1 — Fakultät 1');
  });

  it('clears a half when the choice is emptied', () => {
    screen([source()], [center()], [row({ SourceID: 1, CenterID: 2 })]);
    component.position.set('7');
    component.source.set('');
    component.center.set('2');
    component.assign(new Event('submit'));

    const req = http.expectOne('/api/budget/positions/7/assignment');
    expect(req.request.body).toEqual({ source_id: null, center_id: 2 });
    req.flush(position({ SourceID: null, CenterID: 2 }));
  });

  it('refuses an assignment with no position chosen, and reports a refused one', () => {
    screen();
    component.assign(new Event('submit'));
    expect(component.actionError()).toBe('Es ist keine Haushaltsstelle gewählt.');
    http.expectNone('/api/budget/positions/0/assignment');

    component.position.set('7');
    component.source.set('99');
    component.assign(new Event('submit'));
    http.expectOne('/api/budget/positions/7/assignment').flush(
      { error: 'unknown source' },
      { status: 422, statusText: 'Unprocessable Entity' }
    );
    fixture.detectChanges();
    expect(component.actionError()).toBe('Die Zuordnung konnte nicht gespeichert werden.');
    expect(component.busy()).toBeFalse();
  });

  it('falls back to the id when a named source or centre is not in the lists', () => {
    screen([source()], [center()], [row({ SourceID: 99, CenterID: 98 })]);
    expect(component.sourceNameOf(row({ SourceID: 99 }))).toBe('99');
    expect(component.centerNameOf(row({ CenterID: 98 }))).toBe('98');
    expect(component.sourceNameOf(row({ SourceID: null }))).toBe('keine');
    expect(component.amount(100000)).toBe('1.000,00');
  });
});
