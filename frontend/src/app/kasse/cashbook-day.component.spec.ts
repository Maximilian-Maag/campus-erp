import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { CashbookDayComponent } from './cashbook-day.component';
import { Day } from '../core/api/generated';

describe('CashbookDayComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [CashbookDayComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('has sensible defaults', () => {
    const c = TestBed.createComponent(CashbookDayComponent).componentInstance;
    expect(c.businessDate()).toBe('2026-01-02');
    expect(c.openingCents()).toBe(0);
    expect(c.day()).toBeNull();
    expect(c.error()).toBeNull();
  });

  it('coerces the amount input to a number', () => {
    const fixture = TestBed.createComponent(CashbookDayComponent);
    fixture.detectChanges();
    const input = fixture.nativeElement.querySelector('#opening-cents') as HTMLInputElement;
    input.value = '42';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    fixture.detectChanges();
    expect(fixture.componentInstance.openingCents()).toBe(42);
  });

  it('posts a new day through the generated client and renders it', () => {
    const fixture = TestBed.createComponent(CashbookDayComponent);
    const component = fixture.componentInstance;
    component.businessDate.set('2026-02-03');
    component.openingCents.set(5000);

    component.open();

    const req = http.expectOne('/api/cashbook/days');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ business_date: '2026-02-03', opening_cents: 5000 });

    const day: Day = { ID: 7, BusinessDate: '2026-02-03T00:00:00Z', Status: 'open', OpeningCents: 5000, ClosingCents: 0 };
    req.flush(day);

    fixture.detectChanges();
    expect(component.day()?.ID).toBe(7);
    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="day-id"]')?.textContent).toContain('7');
    expect(html.querySelector('[data-testid="day-status"]')?.textContent).toContain('open');
  });

  it('shows an error when the request fails', () => {
    const fixture = TestBed.createComponent(CashbookDayComponent);
    fixture.componentInstance.open();

    http.expectOne('/api/cashbook/days').flush('nope', { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();

    expect(fixture.componentInstance.error()).toBe('Konnte den Kassenbuch-Tag nicht eröffnen.');
    expect((fixture.nativeElement as HTMLElement).querySelector('[role="alert"]')).toBeTruthy();
  });
});
