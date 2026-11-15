import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { BudgetPositionsComponent } from './positions.component';
import { BudgetPositionResponse } from '../core/api/generated';

describe('BudgetPositionsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [BudgetPositionsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const row: BudgetPositionResponse = {
    position: {
      ID: 1, Year: 2026, Chapter: '0710', Title: '6810',
      Description: 'Beschaffung', AllocatedCents: 100000, CreatedBy: 'alice',
      SourceID: null, CenterID: null,
    },
    balance: { PositionID: 1, AllocatedCents: 100000, CommittedCents: 25000, ActualCents: 5000 },
    available_cents: 70000,
  };

  it('asks the API for every position as soon as it is shown', () => {
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    fixture.detectChanges();

    const req = http.expectOne('/api/budget/positions?limit=50&offset=0');
    expect(req.request.method).toBe('GET');
    req.flush([row]);
    fixture.detectChanges();

    expect(fixture.componentInstance.positions().length).toBe(1);
    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.error()).toBeNull();
  });

  it('renders each position with the available amount the API reported', () => {
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    fixture.detectChanges();
    http.expectOne('/api/budget/positions?limit=50&offset=0').flush([row]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    const cells = html.querySelectorAll('[data-testid="position-1"] td');
    expect(cells[0].textContent).toContain('2026');
    expect(cells[1].textContent).toContain('0710');
    expect(cells[3].textContent).toContain('Beschaffung');
    expect(cells[4].textContent).toContain('1.000,00');
    expect(cells[5].textContent).toContain('250,00');
    expect(cells[6].textContent).toContain('50,00');
    // The available figure is the API's, not a sum computed here.
    expect(html.querySelector('[data-testid="available-1"]')?.textContent).toContain('700,00');
    expect(html.querySelector('caption')?.textContent).toContain('Ansatz');
  });

  it('says so when the budget holds no positions', () => {
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    fixture.detectChanges();
    http.expectOne('/api/budget/positions?limit=50&offset=0').flush([]);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('table')).toBeNull();
    expect(html.querySelector('[role="status"]')?.textContent).toContain('keine Haushaltsstellen');
  });

  it('reports a failed load instead of showing an empty budget', () => {
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    fixture.detectChanges();
    http.expectOne('/api/budget/positions?limit=50&offset=0').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    expect(fixture.componentInstance.error()).toBe('Konnte die Mittelübersicht nicht laden.');
    expect(fixture.componentInstance.loading()).toBeFalse();
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[role="alert"]')).toBeTruthy();
    expect(html.querySelector('[role="status"]')).toBeNull();
  });

  it('starts in a loading state with nothing to show', () => {
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    const component = fixture.componentInstance;

    // Before the API answers: loading, no rows, no error.
    expect(component.loading()).toBeTrue();
    expect(component.positions()).toEqual([]);
    expect(component.error()).toBeNull();
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('[role="status"]')?.textContent).toContain(
      'werden geladen'
    );

    http.expectOne('/api/budget/positions?limit=50&offset=0').flush([row]);
    fixture.detectChanges();
    expect(component.loading()).toBeFalse();
  });

  it('shows the loading state again when asked to reload', () => {
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/budget/positions?limit=50&offset=0').flush([row]);

    component.load();
    expect(component.loading()).toBeTrue();
    expect(component.error()).toBeNull();

    http.expectOne('/api/budget/positions?limit=50&offset=0').flush([]);
    expect(component.loading()).toBeFalse();
    expect(component.positions()).toEqual([]);
  });

  it('treats an empty response body as a budget without positions', () => {
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();

    http.expectOne('/api/budget/positions?limit=50&offset=0').flush(null);
    fixture.detectChanges();

    expect(component.positions()).toEqual([]);
    expect(component.error()).toBeNull();
    expect(component.loading()).toBeFalse();
  });

  it('formats minor units as German amounts, including negatives', () => {
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/budget/positions?limit=50&offset=0').flush([]);

    expect(component.euro(70000)).toBe('700,00');
    expect(component.euro(5)).toBe('0,05');
    expect(component.euro(-250)).toBe('-2,50');
  });
  it('reads the positions one page at a time, offering more only while a page comes back full', () => {
    const fifty = Array.from({ length: 50 }, (_, i) => ({
      ...row,
      position: { ...row.position, ID: i + 1 },
      balance: { ...row.balance, PositionID: i + 1 },
    }));
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;

    http.expectOne('/api/budget/positions?limit=50&offset=0').flush(fifty);
    fixture.detectChanges();
    expect(component.hasMore()).toBeTrue();
    expect(el.querySelector('[data-testid="load-more"]')).not.toBeNull();

    component.loadMore();
    expect(component.loadingMore()).toBeTrue();
    http
      .expectOne('/api/budget/positions?limit=50&offset=50')
      .flush([{ ...row, position: { ...row.position, ID: 51 }, balance: { ...row.balance, PositionID: 51 } }]);
    fixture.detectChanges();

    expect(component.loadingMore()).toBeFalse();
    expect(component.positions().length).toBe(51);
    expect(el.querySelector('[data-testid="position-51"]')).not.toBeNull();
    // A short page is the end of the budget: the offer goes and asking again asks nobody.
    expect(component.hasMore()).toBeFalse();
    expect(el.querySelector('[data-testid="load-more"]')).toBeNull();
    component.loadMore();
    http.expectNone('/api/budget/positions?limit=50&offset=51');
  });

  it('keeps the positions it already has when the next page fails', () => {
    const fifty = Array.from({ length: 50 }, (_, i) => ({
      ...row,
      position: { ...row.position, ID: i + 1 },
      balance: { ...row.balance, PositionID: i + 1 },
    }));
    const fixture = TestBed.createComponent(BudgetPositionsComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();
    http.expectOne('/api/budget/positions?limit=50&offset=0').flush(fifty);
    fixture.detectChanges();
    expect(component.hasMore()).toBeTrue();

    component.loadMore();
    http
      .expectOne('/api/budget/positions?limit=50&offset=50')
      .flush('no', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    // The rows that were read are still valid, so the failure is reported beside them, not over them.
    expect(component.positions().length).toBe(50);
    expect(component.error()).toBe('Konnte die weiteren Haushaltsstellen nicht laden.');
    expect(component.loadingMore()).toBeFalse();
    expect(component.hasMore()).toBeTrue();
  });
});
