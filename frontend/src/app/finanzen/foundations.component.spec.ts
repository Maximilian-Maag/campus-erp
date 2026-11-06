import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { FoundationsComponent } from './foundations.component';
import {
  CampusErpApi,
  Foundation,
  FoundationBalance,
  FoundationBook,
  FoundationBooking,
} from '../core/api/generated';

describe('FoundationsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [FoundationsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const foundation = (
    ID: number,
    Year: number,
    Number: string,
    over: Partial<Foundation> = {},
  ): Foundation => ({ ID, Year, Number, Description: 'Deutschlandstipendium', CarryOver: false, ...over });
  const stiftung = foundation(1, 2026, 'STIFT-1');

  const book = (ID: number, Reference: string, over: Partial<FoundationBook> = {}): FoundationBook => ({
    ID,
    FoundationID: 1,
    Reference,
    Currency: 'EUR',
    CreatedBy: 'alice',
    ...over,
  });
  const buch = book(7, 'B-1');

  const booking = (ID: number, over: Partial<FoundationBooking> = {}): FoundationBooking => ({
    ID,
    BookID: 7,
    Kind: 'income',
    AmountCents: 42500,
    Purpose: 'Spende',
    BookedBy: 'alice',
    ...over,
  });

  const balance = (over: Partial<FoundationBalance> = {}): FoundationBalance => ({
    BookID: 7,
    IncomeCents: 50000,
    ExpenseCents: 7500,
    ...over,
  });

  /** The screen with its foundations loaded. */
  function loaded(foundations: Foundation[] = [stiftung]) {
    const fixture = TestBed.createComponent(FoundationsComponent);
    fixture.detectChanges();
    http.expectOne('/api/foundations').flush(foundations);
    fixture.detectChanges();
    return fixture;
  }

  /** The screen with a foundation selected and its books read. */
  function withBooks(foundations: Foundation[] = [stiftung], books: FoundationBook[] = [buch]) {
    const fixture = loaded(foundations);
    fixture.componentInstance.select(foundations[0]);
    http.expectOne(`/api/foundations/${foundations[0].ID}/books`).flush(books);
    fixture.detectChanges();
    return fixture;
  }

  /** The screen with a book selected, its movements read and its balance read. */
  function withBook(books: FoundationBook[] = [buch], bookings: FoundationBooking[] = [booking(3)], bal = balance()) {
    const fixture = withBooks([stiftung], books);
    fixture.componentInstance.selectBook(books[0]);
    http.expectOne(`/api/foundation-books/${books[0].ID}/bookings`).flush(bookings);
    http.expectOne(`/api/foundation-books/${books[0].ID}/balance`).flush(bal);
    fixture.detectChanges();
    return fixture;
  }

  it('starts unloaded and empty, with the currency defaulting', () => {
    const fixture = TestBed.createComponent(FoundationsComponent);
    const component = fixture.componentInstance;

    expect(component.foundations()).toEqual([]);
    expect(component.loading()).toBeTrue();
    expect(component.books()).toEqual([]);
    expect(component.booksLoading()).toBeFalse();
    expect(component.bookings()).toEqual([]);
    expect(component.balance()).toBeNull();
    expect(component.selectedFoundation()).toBeNull();
    expect(component.selectedBook()).toBeNull();
    expect(component.filterYear()).toBe('');
    expect(component.newYear()).toBe('');
    expect(component.newNumber()).toBe('');
    expect(component.newDescription()).toBe('');
    expect(component.newCarryOver()).toBeFalse();
    expect(component.reference()).toBe('');
    expect(component.currency()).toBe('EUR');
    expect(component.bookingKind()).toBe('income');
    expect(component.bookingAmount()).toBe('');
    expect(component.purpose()).toBe('');
    expect(component.notice()).toBeNull();
    expect(component.actionError()).toBeNull();
    expect(component.loadError()).toBeNull();
  });

  it('loads the foundations', () => {
    const fixture = loaded([stiftung, foundation(2, 2025, 'STIFT-2')]);

    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.foundations().map((f) => f.Number)).toEqual([
      'STIFT-1',
      'STIFT-2',
    ]);
  });

  it('shows the loading state again on a reload', () => {
    const fixture = loaded([stiftung]);

    fixture.componentInstance.loadFoundations();
    expect(fixture.componentInstance.loading()).toBeTrue();
    http.expectOne('/api/foundations').flush([foundation(2, 2025, 'STIFT-2')]);
    fixture.detectChanges();

    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.foundations().map((f) => f.Number)).toEqual(['STIFT-2']);
  });

  it('sends the typed year as the filter, and no year at all when none is typed', () => {
    const api = TestBed.inject(CampusErpApi);
    const spy = spyOn(api, 'listFoundations').and.callThrough();
    const fixture = loaded([]);

    // A padded year is sent trimmed: the server parses it, but it is not asked to parse padding.
    fixture.componentInstance.filterYear.set('  2025  ');
    fixture.componentInstance.filter(new Event('submit'));
    http.expectOne('/api/foundations?year=2025').flush([foundation(2, 2025, 'STIFT-2')]);
    fixture.detectChanges();
    expect(fixture.componentInstance.foundations().map((f) => f.Year)).toEqual([2025]);
    expect(spy).toHaveBeenCalledWith('2025');

    fixture.componentInstance.filterYear.set('   ');
    fixture.componentInstance.filter(new Event('submit'));
    http.expectOne('/api/foundations').flush([]);
    fixture.detectChanges();
    // A year that is not typed is sent as no year at all, not as an empty year.
    expect(spy).toHaveBeenCalledWith(undefined);
  });

  it('treats an empty response as no foundations and says so', () => {
    const fixture = TestBed.createComponent(FoundationsComponent);
    fixture.detectChanges();
    http.expectOne('/api/foundations').flush(null);
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.foundations()).toEqual([]);
    expect(component.loading()).toBeFalse();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('keine Stiftung');
  });

  it('reports a failed load instead of an empty list', () => {
    const fixture = TestBed.createComponent(FoundationsComponent);
    fixture.detectChanges();
    http.expectOne('/api/foundations').flush('nope', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    const component = fixture.componentInstance;
    expect(component.loadError()).toBe('Konnte die Stiftungen nicht laden.');
    expect(component.loading()).toBeFalse();
  });

  it('reports a year the server refuses as a failed load', () => {
    const fixture = loaded([]);
    fixture.componentInstance.filterYear.set('heuer');
    fixture.componentInstance.filter(new Event('submit'));
    http
      .expectOne('/api/foundations?year=heuer')
      .flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(fixture.componentInstance.loadError()).toBe('Konnte die Stiftungen nicht laden.');
  });

  it('creates a foundation, trims what was typed and reads the list again', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;
    component.newYear.set(' 2026 ');
    component.newNumber.set(' STIFT-9 ');
    component.newDescription.set('  Stipendium  ');
    component.newCarryOver.set(true);

    component.createFoundation(new Event('submit'));

    expect(component.submitting()).toBeTrue();
    const post = http.expectOne('/api/foundations');
    expect(post.request.method).toBe('POST');
    expect(post.request.body).toEqual({
      year: 2026,
      number: 'STIFT-9',
      description: 'Stipendium',
      carry_over: true,
    });
    post.flush(foundation(9, 2026, 'STIFT-9', { CarryOver: true }));

    // The list is the server's answer to the filter, so it is read again rather than appended to.
    http.expectOne('/api/foundations').flush([foundation(9, 2026, 'STIFT-9', { CarryOver: true })]);
    fixture.detectChanges();

    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBe('Stiftung angelegt.');
    expect(component.foundations().map((f) => f.ID)).toEqual([9]);
    expect(component.newNumber()).toBe('');
    expect(component.newDescription()).toBe('');
    const row = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="foundation-9"]');
    expect(row?.textContent).toContain('STIFT-9');
  });

  it('refuses a blank or non-numeric year, a blank number and a blank description', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;

    component.createFoundation(new Event('submit'));
    expect(component.actionError()).toBe('Ein Jahr ist erforderlich.');

    // A whitespace-only year is a missing year, and a word is not one either.
    component.newYear.set('   ');
    component.createFoundation(new Event('submit'));
    expect(component.actionError()).toBe('Ein Jahr ist erforderlich.');

    component.newYear.set('heuer');
    component.createFoundation(new Event('submit'));
    expect(component.actionError()).toBe('Ein Jahr ist erforderlich.');

    component.newYear.set('2026');
    component.createFoundation(new Event('submit'));
    expect(component.actionError()).toBe('Eine Nummer ist erforderlich.');

    component.newNumber.set('STIFT-9');
    component.createFoundation(new Event('submit'));
    expect(component.actionError()).toBe('Eine Bezeichnung ist erforderlich.');

    expect(component.submitting()).toBeFalse();
    http.expectNone('/api/foundations');
  });

  it('names the failure when the foundation is refused, and keeps what was typed', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;
    component.newYear.set('2026');
    component.newNumber.set('STIFT-1');
    component.newDescription.set('Stipendium');

    component.createFoundation(new Event('submit'));
    http.expectOne('/api/foundations').flush('no', { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Stiftung nicht anlegen.');
    expect(component.submitting()).toBeFalse();
    expect(component.foundations()).toEqual([]);
    expect(component.newNumber()).toBe('STIFT-1');
  });

  it('selects a foundation and reads its books', () => {
    const fixture = withBooks([stiftung], [buch, book(8, 'B-2', { Currency: 'CHF' })]);
    const component = fixture.componentInstance;

    expect(component.selectedFoundation()?.ID).toBe(1);
    expect(component.booksLoading()).toBeFalse();
    expect(component.books().map((b) => b.Reference)).toEqual(['B-1', 'B-2']);
    const text = (fixture.nativeElement as HTMLElement).textContent;
    expect(text).toContain('Stiftungsbücher — 2026/STIFT-1');
    expect(text).toContain('CHF');
  });

  it('shows the loading state while the books are read', () => {
    const fixture = loaded([stiftung]);
    fixture.componentInstance.select(stiftung);

    expect(fixture.componentInstance.booksLoading()).toBeTrue();
    http.expectOne('/api/foundations/1/books').flush([buch]);
    fixture.detectChanges();
    expect(fixture.componentInstance.booksLoading()).toBeFalse();
  });

  it('says so when a foundation has no book yet', () => {
    const fixture = withBooks([stiftung], []);
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('noch kein Buch');
  });

  it('treats an empty book response as no books', () => {
    const fixture = loaded([stiftung]);
    fixture.componentInstance.select(stiftung);
    http.expectOne('/api/foundations/1/books').flush(null);
    fixture.detectChanges();
    expect(fixture.componentInstance.books()).toEqual([]);
  });

  it('reports a failed books load', () => {
    const fixture = loaded([stiftung]);
    fixture.componentInstance.select(stiftung);
    http
      .expectOne('/api/foundations/1/books')
      .flush('no', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    expect(fixture.componentInstance.actionError()).toBe('Konnte die Bücher nicht laden.');
    expect(fixture.componentInstance.booksLoading()).toBeFalse();
  });

  it('drops the selected book and its movements when another foundation is selected', () => {
    const fixture = withBook();
    const component = fixture.componentInstance;
    expect(component.selectedBook()?.ID).toBe(7);

    component.select(foundation(2, 2025, 'STIFT-2'));
    expect(component.selectedBook()).toBeNull();
    expect(component.bookings()).toEqual([]);
    expect(component.balance()).toBeNull();
    http.expectOne('/api/foundations/2/books').flush([]);
  });

  it('creates a book for the selected foundation', () => {
    const fixture = withBooks([stiftung], []);
    const component = fixture.componentInstance;
    component.reference.set('  B-9  ');
    component.currency.set(' chf ');

    component.createBook(new Event('submit'));

    expect(component.submitting()).toBeTrue();
    const post = http.expectOne('/api/foundation-books');
    expect(post.request.method).toBe('POST');
    expect(post.request.body).toEqual({ foundation_id: 1, reference: 'B-9', currency: 'chf' });
    post.flush(book(9, 'B-9', { Currency: 'chf' }));
    fixture.detectChanges();

    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBe('Buch angelegt.');
    expect(component.books().map((b) => b.ID)).toEqual([9]);
    expect(component.reference()).toBe('');
    const row = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="book-9"]');
    expect(row?.textContent).toContain('B-9');
  });

  it('refuses a book without a foundation, and a blank reference', () => {
    const fixture = loaded([]);
    const component = fixture.componentInstance;
    component.reference.set('B-1');

    component.createBook(new Event('submit'));
    expect(component.actionError()).toBe('Zuerst eine Stiftung wählen.');

    component.select(stiftung);
    component.reference.set('   ');
    component.createBook(new Event('submit'));
    expect(component.actionError()).toBe('Eine Referenz ist erforderlich.');

    expect(component.submitting()).toBeFalse();
    http.expectOne('/api/foundations/1/books').flush([]);
    http.expectNone('/api/foundation-books');
  });

  it('names the failure when the book is refused, and keeps what was typed', () => {
    const fixture = withBooks([stiftung], []);
    const component = fixture.componentInstance;
    component.reference.set('B-1');

    component.createBook(new Event('submit'));
    http.expectOne('/api/foundation-books').flush('no', { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte das Buch nicht anlegen.');
    expect(component.submitting()).toBeFalse();
    expect(component.books()).toEqual([]);
    expect(component.reference()).toBe('B-1');
  });

  it('selects a book and reads its movements and its balance', () => {
    const fixture = withBook([buch], [booking(3), booking(4, { Kind: 'expense', AmountCents: 7500 })]);
    const component = fixture.componentInstance;

    expect(component.selectedBook()?.ID).toBe(7);
    expect(component.bookings().map((b) => b.ID)).toEqual([3, 4]);
    expect(component.balance()?.IncomeCents).toBe(50000);
    const text = (fixture.nativeElement as HTMLElement).textContent;
    expect(text).toContain('Buchungen — B-1');
    expect(text).toContain('Einnahmen 500,00');
    expect(text).toContain('Ausgaben 75,00');
    expect(text).toContain('Saldo 425,00');
    expect(text).toContain('Einnahme');
    expect(text).toContain('Ausgabe');
  });

  it('says so when a book has no movement yet', () => {
    const fixture = withBook([buch], []);
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('noch keine Buchung');
  });

  it('treats an empty movements response as no movements', () => {
    const fixture = withBook([buch], null as unknown as FoundationBooking[]);
    expect(fixture.componentInstance.bookings()).toEqual([]);
  });

  it('reports a failed movements load and a failed balance load', () => {
    const fixture = withBooks([stiftung], [buch]);
    const component = fixture.componentInstance;

    component.selectBook(buch);
    http
      .expectOne('/api/foundation-books/7/bookings')
      .flush('no', { status: 500, statusText: 'Server Error' });
    http
      .expectOne('/api/foundation-books/7/balance')
      .flush('no', { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte den Saldo nicht laden.');
    expect(component.bookings()).toEqual([]);
    expect(component.balance()).toBeNull();
  });

  it('reports a failed movements load on its own', () => {
    const fixture = withBooks([stiftung], [buch]);
    const component = fixture.componentInstance;

    component.selectBook(buch);
    http
      .expectOne('/api/foundation-books/7/bookings')
      .flush('no', { status: 500, statusText: 'Server Error' });
    http.expectOne('/api/foundation-books/7/balance').flush(balance());
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Buchungen nicht laden.');
    expect(component.balance()?.IncomeCents).toBe(50000);
  });

  it('books a movement in euro, joins it to the movements and reads the balance again', () => {
    const fixture = withBook([buch], []);
    const component = fixture.componentInstance;
    component.bookingKind.set('expense');
    component.bookingAmount.set(' 425,00 ');
    component.purpose.set('  Auszahlung  ');

    component.recordBooking(new Event('submit'));

    expect(component.submitting()).toBeTrue();
    const post = http.expectOne('/api/foundation-books/7/bookings');
    expect(post.request.method).toBe('POST');
    expect(post.request.body).toEqual({ kind: 'expense', amount_cents: 42500, purpose: 'Auszahlung' });
    post.flush(booking(5, { Kind: 'expense', AmountCents: 42500, Purpose: 'Auszahlung' }));

    // The balance is derived, so it is read again rather than adjusted here.
    http.expectOne('/api/foundation-books/7/balance').flush(balance({ IncomeCents: 0, ExpenseCents: 42500 }));
    fixture.detectChanges();

    expect(component.submitting()).toBeFalse();
    expect(component.notice()).toBe('Buchung erfasst.');
    expect(component.bookings().map((b) => b.ID)).toEqual([5]);
    expect(component.bookingAmount()).toBe('');
    expect(component.purpose()).toBe('');
    const row = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="booking-5"]');
    expect(row?.textContent).toContain('Auszahlung');
  });

  it('refuses a movement without a book, without an amount and without a purpose', () => {
    const fixture = withBooks([stiftung], [buch]);
    const component = fixture.componentInstance;

    component.recordBooking(new Event('submit'));
    expect(component.actionError()).toBe('Zuerst ein Buch wählen.');

    component.selectBook(buch);
    http.expectOne('/api/foundation-books/7/bookings').flush([]);
    http.expectOne('/api/foundation-books/7/balance').flush(balance());

    component.recordBooking(new Event('submit'));
    expect(component.actionError()).toBe('Ein Betrag in Euro ist erforderlich.');

    // An amount that is not a positive euro amount is refused here, not by the API.
    component.bookingAmount.set('0,00');
    component.recordBooking(new Event('submit'));
    expect(component.actionError()).toBe('Ein Betrag in Euro ist erforderlich.');

    component.bookingAmount.set('425,00');
    component.recordBooking(new Event('submit'));
    expect(component.actionError()).toBe('Ein Verwendungszweck ist erforderlich.');

    expect(component.submitting()).toBeFalse();
    http.expectNone('/api/foundation-books/7/bookings');
  });

  it('names the failure when the movement is refused, and keeps what was typed', () => {
    const fixture = withBook([buch], []);
    const component = fixture.componentInstance;
    component.bookingAmount.set('425,00');
    component.purpose.set('Spende');

    component.recordBooking(new Event('submit'));
    http
      .expectOne('/api/foundation-books/7/bookings')
      .flush('no', { status: 422, statusText: 'Unprocessable' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Konnte die Buchung nicht erfassen.');
    expect(component.submitting()).toBeFalse();
    expect(component.bookings()).toEqual([]);
    expect(component.bookingAmount()).toBe('425,00');
    expect(component.purpose()).toBe('Spende');
  });

  it('nets a balance and names a movement kind', () => {
    const component = loaded([]).componentInstance;

    expect(component.net(balance())).toBe(42500);
    expect(component.net(balance({ IncomeCents: 0, ExpenseCents: 7500 }))).toBe(-7500);
    expect(component.kindLabel('income')).toBe('Einnahme');
    expect(component.kindLabel('expense')).toBe('Ausgabe');
    expect(component.kindLabel('transfer')).toBe('transfer');
  });
});
