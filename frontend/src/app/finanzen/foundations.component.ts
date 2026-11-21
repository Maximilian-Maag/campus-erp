import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import {
  CampusErpApi,
  Foundation,
  FoundationBalance,
  FoundationBook,
  FoundationBooking,
} from '../core/api/generated';
import { euroToCents, formatCents } from '../core/format';
import { problemCode } from '../core/problem';
import { PageComponent } from '../ui/page.component';

/**
 * Finanzen — Stiftungen (a foundation, its books, and a book's movements).
 *
 * A foundation is money held for a purpose (legacy `kbs.stift`), and the canonical model gives
 * each foundation its own book so one foundation's movements can never be mixed with another's
 * (slice S6, `docs/29`). The API could already create all three and read a book's movements and
 * balance — but a foundation could not be found and a book could not be listed, so a fresh
 * install could create a book and then never reach it again. This is where they are found.
 *
 * The screen reads in one direction — foundation, book, movements — because that is the model:
 * a book belongs to exactly one foundation, and a movement to exactly one book. Each level is
 * asked of the server for the level above it (`GET /api/foundations/{id}/books`), never filtered
 * in the browser, so nothing here can show one foundation's books under another's name.
 *
 * The year is sent as typed and parsed by the server, which is the only side that knows which
 * years exist; a refusal comes back as a failed load rather than as an empty list. Amounts are
 * typed in euro and converted once, through the shared `euroToCents` (the inverse of the
 * `formatCents` every figure is shown with).
 */
@Component({
  selector: 'app-foundations',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.finanzenStiftungen.title" subtitleKey="page.finanzenStiftungen.subtitle">
      <section>
        @if (loadError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (actionError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (notice(); as n) {
          <p role="status" class="notice">{{ n }}</p>
        }

        <form (submit)="filter($event)">
          <h2 id="foundations-filter-heading">{{ 'finanzen.foundations.filterHeading' | transloco }}</h2>
          <div class="field">
            <label for="foundation-filter-year">{{ 'common.year' | transloco }}</label>
            <input
              id="foundation-filter-year"
              type="text"
              inputmode="numeric"
              [value]="filterYear()"
              (input)="filterYear.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <button type="submit">{{ 'finanzen.foundations.show' | transloco }}</button>
          </div>
          <p>{{ 'finanzen.foundations.filterHint' | transloco }}</p>
        </form>

        @if (loading()) {
          <p role="status">{{ 'finanzen.foundations.loading' | transloco }}</p>
        } @else if (!loadError()) {
          @if (foundations().length === 0) {
            <p role="status">{{ 'finanzen.foundations.empty' | transloco }}</p>
          } @else {
            <table>
              <caption>
                {{ 'finanzen.foundations.listCaption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'common.year' | transloco }}</th>
                  <th scope="col">{{ 'common.number' | transloco }}</th>
                  <th scope="col">{{ 'common.description' | transloco }}</th>
                  <th scope="col">{{ 'finanzen.foundations.carryOver' | transloco }}</th>
                  <th scope="col">{{ 'finanzen.foundations.books' | transloco }}</th>
                </tr>
              </thead>
              <tbody>
                @for (foundation of foundations(); track foundation.ID) {
                  <tr [attr.data-testid]="'foundation-' + foundation.ID">
                    <td>{{ foundation.Year }}</td>
                    <td>{{ foundation.Number }}</td>
                    <td>{{ foundation.Description }}</td>
                    <td>
                      {{
                        (foundation.CarryOver
                          ? 'finanzen.foundations.yes'
                          : 'finanzen.foundations.no'
                        ) | transloco
                      }}
                    </td>
                    <td>
                      <button type="button" (click)="select(foundation)">
                        {{ 'finanzen.foundations.showBooks' | transloco }}
                      </button>
                    </td>
                  </tr>
                }
              </tbody>
            </table>
          }

          <form (submit)="createFoundation($event)">
            <h2 id="foundation-new-heading">{{ 'finanzen.foundations.newTitle' | transloco }}</h2>
            <div class="field">
              <label for="foundation-year">{{ 'finanzen.foundations.budgetYear' | transloco }}</label>
              <input
                id="foundation-year"
                type="text"
                inputmode="numeric"
                [value]="newYear()"
                (input)="newYear.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="foundation-number">{{ 'common.number' | transloco }}</label>
              <input
                id="foundation-number"
                type="text"
                [value]="newNumber()"
                (input)="newNumber.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="foundation-description">{{ 'common.description' | transloco }}</label>
              <input
                id="foundation-description"
                type="text"
                [value]="newDescription()"
                (input)="newDescription.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <input
                id="foundation-carry-over"
                type="checkbox"
                [checked]="newCarryOver()"
                (change)="newCarryOver.set($any($event.target).checked)"
              />
              <label for="foundation-carry-over">{{ 'finanzen.foundations.carryOverAllowed' | transloco }}</label>
            </div>
            <div class="field">
              <button type="submit" [disabled]="submitting()">
                {{ 'finanzen.foundations.submit' | transloco }}
              </button>
            </div>
          </form>
        }

        @if (selectedFoundation(); as foundation) {
          <section aria-labelledby="books-heading" [attr.data-testid]="'books-of-' + foundation.ID">
            <h2 id="books-heading">
              {{
                'finanzen.foundations.booksHeading'
                  | transloco: { year: foundation.Year, number: foundation.Number }
              }}
            </h2>

            @if (booksLoading()) {
              <p role="status">{{ 'finanzen.foundations.booksLoading' | transloco }}</p>
            } @else if (books().length === 0) {
              <p role="status">{{ 'finanzen.foundations.booksEmpty' | transloco }}</p>
            } @else {
              <table>
                <caption>
                  {{ 'finanzen.foundations.booksCaption' | transloco }}
                </caption>
                <thead>
                  <tr>
                    <th scope="col">{{ 'common.reference' | transloco }}</th>
                    <th scope="col">{{ 'common.currency' | transloco }}</th>
                    <th scope="col">{{ 'finanzen.foundations.createdBy' | transloco }}</th>
                    <th scope="col">{{ 'finanzen.foundations.bookings' | transloco }}</th>
                    <th scope="col">{{ 'finanzen.foundations.closing' | transloco }}</th>
                  </tr>
                </thead>
                <tbody>
                  @for (book of books(); track book.ID) {
                    <tr [attr.data-testid]="'book-' + book.ID">
                      <td>{{ book.Reference }}</td>
                      <td>{{ book.Currency }}</td>
                      <td>{{ book.CreatedBy }}</td>
                      <td>
                        <button type="button" (click)="selectBook(book)">
                          {{ 'finanzen.foundations.showBookings' | transloco }}
                        </button>
                      </td>
                      <td>
                        <button
                          type="button"
                          [attr.data-testid]="'carry-over-' + book.ID"
                          [disabled]="carryingOver() === book.ID"
                          (click)="carryOver(book)"
                        >
                          {{
                            carryingOver() === book.ID
                              ? ('finanzen.foundations.carryingOver' | transloco)
                              : ('finanzen.foundations.carryOverAction' | transloco)
                          }}
                        </button>
                      </td>
                    </tr>
                  }
                </tbody>
              </table>
            }

            <form (submit)="createBook($event)">
              <h3 id="book-new-heading">{{ 'finanzen.foundations.newBookTitle' | transloco }}</h3>
              <div class="field">
                <label for="book-reference">{{ 'common.reference' | transloco }}</label>
                <input
                  id="book-reference"
                  type="text"
                  [value]="reference()"
                  (input)="reference.set($any($event.target).value)"
                />
              </div>
              <div class="field">
                <label for="book-currency">{{ 'common.currency' | transloco }}</label>
                <input
                  id="book-currency"
                  type="text"
                  [value]="currency()"
                  (input)="currency.set($any($event.target).value)"
                />
              </div>
              <div class="field">
                <button type="submit" [disabled]="submitting()">
                  {{ 'finanzen.foundations.submitBook' | transloco }}
                </button>
              </div>
            </form>

            @if (selectedBook(); as book) {
              <section aria-labelledby="bookings-heading" [attr.data-testid]="'bookings-of-' + book.ID">
                <h3 id="bookings-heading">
                  {{ 'finanzen.foundations.bookingsHeading' | transloco: { reference: book.Reference } }}
                </h3>

                @if (balance(); as b) {
                  <p>
                    {{
                      'finanzen.foundations.balance'
                        | transloco
                          : {
                              income: euro(b.IncomeCents),
                              expense: euro(b.ExpenseCents),
                              net: euro(net(b)),
                            }
                    }}
                  </p>
                }

                @if (bookings().length === 0) {
                  <p role="status">{{ 'finanzen.foundations.bookingsEmpty' | transloco }}</p>
                } @else {
                  <table>
                    <caption>
                      {{ 'finanzen.foundations.bookingsCaption' | transloco }}
                    </caption>
                    <thead>
                      <tr>
                        <th scope="col">{{ 'common.type' | transloco }}</th>
                        <th scope="col">{{ 'common.amount' | transloco }}</th>
                        <th scope="col">{{ 'common.purpose' | transloco }}</th>
                        <th scope="col">{{ 'finanzen.foundations.bookedBy' | transloco }}</th>
                        <th scope="col">{{ 'finanzen.foundations.correction' | transloco }}</th>
                      </tr>
                    </thead>
                    <tbody>
                      @for (booking of bookings(); track booking.ID) {
                        <tr [attr.data-testid]="'booking-' + booking.ID">
                          <td>{{ kindLabel(booking.Kind) }}</td>
                          <td>{{ euro(booking.AmountCents) }}</td>
                          <td>{{ booking.Purpose }}</td>
                          <td>{{ booking.BookedBy }}</td>
                          <td>
                            @if (booking.ReversedBy) {
                              <span [attr.data-testid]="'reversed-' + booking.ID">{{
                                'finanzen.foundations.reversed' | transloco
                              }}</span>
                            } @else if (booking.Reverses) {
                              <span [attr.data-testid]="'counter-' + booking.ID">{{
                                'finanzen.foundations.counterBooking' | transloco
                              }}</span>
                            } @else {
                              <button
                                type="button"
                                [attr.data-testid]="'reverse-' + booking.ID"
                                [disabled]="reversing() === booking.ID"
                                (click)="reverse(booking)"
                              >
                                {{
                                  reversing() === booking.ID
                                    ? ('finanzen.foundations.reversing' | transloco)
                                    : ('common.cancel' | transloco)
                                }}
                              </button>
                            }
                          </td>
                        </tr>
                      }
                    </tbody>
                  </table>
                  @if (hasMore()) {
                    <p>
                      <button
                        type="button"
                        data-testid="load-more-bookings"
                        [disabled]="loadingMore()"
                        (click)="loadMore()"
                      >
                        {{
                          loadingMore()
                            ? ('finanzen.foundations.loadingMore' | transloco)
                            : ('finanzen.foundations.loadMore' | transloco)
                        }}
                      </button>
                    </p>
                  }
                }

                <form (submit)="recordBooking($event)">
                  <h4 id="booking-new-heading">{{ 'finanzen.foundations.newBookingTitle' | transloco }}</h4>
                  <div class="field">
                    <label for="booking-kind">{{ 'common.type' | transloco }}</label>
                    <select
                      id="booking-kind"
                      [value]="bookingKind()"
                      (change)="bookingKind.set($any($event.target).value)"
                    >
                      <option value="income">{{ 'finanzen.foundations.income' | transloco }}</option>
                      <option value="expense">{{ 'finanzen.foundations.expense' | transloco }}</option>
                    </select>
                  </div>
                  <div class="field">
                    <label for="booking-amount">{{ 'finanzen.foundations.amountEuro' | transloco }}</label>
                    <input
                      id="booking-amount"
                      type="text"
                      inputmode="decimal"
                      [value]="bookingAmount()"
                      (input)="bookingAmount.set($any($event.target).value)"
                    />
                  </div>
                  <div class="field">
                    <label for="booking-purpose">{{ 'common.purpose' | transloco }}</label>
                    <input
                      id="booking-purpose"
                      type="text"
                      [value]="purpose()"
                      (input)="purpose.set($any($event.target).value)"
                    />
                  </div>
                  <div class="field">
                    <button type="submit" [disabled]="submitting()">
                      {{ 'finanzen.foundations.submitBooking' | transloco }}
                    </button>
                  </div>
                </form>
              </section>
            }
          </section>
        }
      </section>
    </app-page>
  `,
})
export class FoundationsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly foundations = signal<Foundation[]>([]);
  readonly loading = signal(true);
  readonly submitting = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  readonly filterYear = signal('');
  readonly newYear = signal('');
  readonly newNumber = signal('');
  readonly newDescription = signal('');
  readonly newCarryOver = signal(false);

  readonly selectedFoundation = signal<Foundation | null>(null);
  readonly books = signal<FoundationBook[]>([]);
  readonly booksLoading = signal(false);
  readonly reference = signal('');
  readonly currency = signal('EUR');

  readonly selectedBook = signal<FoundationBook | null>(null);
  readonly bookings = signal<FoundationBooking[]>([]);
  /**
   * One page of movements, sent explicitly rather than left to the server's default: a screen whose
   * page size changed when the server did would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page of movements came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
  readonly balance = signal<FoundationBalance | null>(null);
  readonly bookingKind = signal('income');
  readonly bookingAmount = signal('');
  readonly purpose = signal('');
  /** The book whose carry-over is in flight, so its button says so and cannot be pressed twice. */
  readonly carryingOver = signal<number | null>(null);
  /** The movement whose cancellation is in flight, for the same reason. */
  readonly reversing = signal<number | null>(null);

  readonly euro = formatCents;

  ngOnInit(): void {
    this.loadFoundations();
  }

  /** Reads the foundations, narrowed to the typed year — or to every year when none is typed. */
  loadFoundations(): void {
    this.loading.set(true);
    this.loadError.set(null);
    const year = this.filterYear().trim();
    this.api.listFoundations(year === '' ? undefined : year).subscribe({
      next: (foundations) => {
        this.foundations.set(foundations ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('finanzen.foundations.loadFailed'));
        this.loading.set(false);
      },
    });
  }

  filter(event: Event): void {
    event.preventDefault();
    this.loadFoundations();
  }

  createFoundation(event: Event): void {
    event.preventDefault();
    const rawYear = this.newYear().trim();
    const year = Number(rawYear);
    if (rawYear === '' || !Number.isInteger(year)) {
      this.actionError.set(this.i18n.translate('finanzen.foundations.yearRequired'));
      return;
    }
    const number = this.newNumber().trim();
    if (number === '') {
      this.actionError.set(this.i18n.translate('finanzen.foundations.numberRequired'));
      return;
    }
    const description = this.newDescription().trim();
    if (description === '') {
      this.actionError.set(this.i18n.translate('finanzen.foundations.descriptionRequired'));
      return;
    }
    this.submitting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api
      .createFoundation({ year, number, description, carry_over: this.newCarryOver() })
      .subscribe({
        next: () => {
          this.submitting.set(false);
          this.notice.set(this.i18n.translate('finanzen.foundations.created'));
          this.newNumber.set('');
          this.newDescription.set('');
          // The list is the server's answer to the filter above, so it is read again rather than
          // appended to: an appended foundation would sit in a year the filter excludes.
          this.loadFoundations();
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set(this.i18n.translate('finanzen.foundations.createFailed'));
        },
      });
  }

  /** Selects a foundation and reads its books. */
  select(foundation: Foundation): void {
    this.selectedFoundation.set(foundation);
    this.selectedBook.set(null);
    this.bookings.set([]);
    this.hasMore.set(false);
    this.balance.set(null);
    this.booksLoading.set(true);
    this.actionError.set(null);
    this.api.listFoundationBooks(foundation.ID).subscribe({
      next: (books) => {
        this.books.set(books ?? []);
        this.booksLoading.set(false);
      },
      error: () => {
        this.actionError.set(this.i18n.translate('finanzen.foundations.booksLoadFailed'));
        this.booksLoading.set(false);
      },
    });
  }

  /**
   * Closes a year's book into the next: the net becomes the opening movement of the same
   * foundation's book in the following year. The server decides whether that is allowed — the
   * foundation must carry, the book must have a balance, and the next year must be open — so the
   * screen only names the refusal it gets back.
   */
  carryOver(book: FoundationBook): void {
    this.actionError.set(null);
    this.notice.set(null);
    this.carryingOver.set(book.ID);
    this.api.carryOverFoundationBook(book.ID, { reference: '' }).subscribe({
      next: (res) => {
        this.carryingOver.set(null);
        this.notice.set(
          this.i18n.translate('finanzen.foundations.carriedOver', {
            cents: formatCents(res.cents),
            year: res.year,
            reference: res.target_reference,
            nextYear: res.year + 1,
          }),
        );
      },
      error: (err: unknown) => {
        this.carryingOver.set(null);
        this.actionError.set(carryOverMessage(err, this.i18n));
      },
    });
  }

  /**
   * Cancels one movement. The server appends a counter-movement beside it and marks the original, so
   * the screen reads the movements and the balance again rather than adjusting them here: the balance
   * is derived, and the mistake stays on the record with a counter-movement next to it.
   */
  reverse(booking: FoundationBooking): void {
    const book = this.selectedBook();
    if (book === null) {
      return;
    }
    this.actionError.set(null);
    this.notice.set(null);
    this.reversing.set(booking.ID);
    this.api.reverseFoundationBooking(booking.ID, { reason: '' }).subscribe({
      next: (res) => {
        this.reversing.set(null);
        this.notice.set(
          this.i18n.translate('finanzen.foundations.reversedDone', {
            original: res.original_id,
            counter: res.counter_booking_id,
          }),
        );
        // The counter-movement is appended by the server at the end of the book, so a screen holding
        // a page of it starts again: a page is only a page while it starts at the first movement.
        this.loadBookings(book);
        this.loadBalance(book);
      },
      error: (err: unknown) => {
        this.reversing.set(null);
        this.actionError.set(reversalMessage(err, this.i18n));
      },
    });
  }

  createBook(event: Event): void {
    event.preventDefault();
    const foundation = this.selectedFoundation();
    if (foundation === null) {
      this.actionError.set(this.i18n.translate('finanzen.foundations.foundationRequired'));
      return;
    }
    const reference = this.reference().trim();
    if (reference === '') {
      this.actionError.set(this.i18n.translate('finanzen.foundations.referenceRequired'));
      return;
    }
    this.submitting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api
      .createFoundationBook({
        foundation_id: foundation.ID,
        reference,
        currency: this.currency().trim(),
      })
      .subscribe({
        next: (book) => {
          this.submitting.set(false);
          // A new book joins the list this foundation was read for; a reload would only re-read it.
          this.books.set([...this.books(), book]);
          this.notice.set(this.i18n.translate('finanzen.foundations.bookCreated'));
          this.reference.set('');
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set(this.i18n.translate('finanzen.foundations.bookCreateFailed'));
        },
      });
  }

  /** Selects a book and reads its movements and its derived balance. */
  selectBook(book: FoundationBook): void {
    this.selectedBook.set(book);
    this.actionError.set(null);
    this.loadBookings(book);
    this.loadBalance(book);
  }

  /**
   * Reads the first page of a book's movements and forgets the rest, which is what a fresh question
   * means: the movements are ordered by id, so a page is only a page while it starts at the first
   * one. The offset of the next page is the number of movements already on screen, which is why the
   * list is never re-sorted in the browser.
   */
  private loadBookings(book: FoundationBook): void {
    this.hasMore.set(false);
    this.api.listFoundationBookings(book.ID, String(this.pageSize), '0').subscribe({
      next: (bookings) => {
        const page = bookings ?? [];
        this.bookings.set(page);
        // A full page means there may be more; a short one is the end of the book's movements.
        this.hasMore.set(page.length === this.pageSize);
      },
      error: () => this.actionError.set(this.i18n.translate('finanzen.foundations.bookingsLoadFailed')),
    });
  }

  /**
   * Reads the next page of movements and appends it. A failed page is reported without clearing what
   * is already there — the movements that were read are still valid, and blanking the screen would
   * suggest they are not.
   */
  loadMore(): void {
    const book = this.selectedBook();
    if (book === null || !this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api
      .listFoundationBookings(book.ID, String(this.pageSize), String(this.bookings().length))
      .subscribe({
        next: (bookings) => {
          const page = bookings ?? [];
          this.bookings.set([...this.bookings(), ...page]);
          this.hasMore.set(page.length === this.pageSize);
          this.loadingMore.set(false);
        },
        error: () => {
          this.loadingMore.set(false);
          this.actionError.set(this.i18n.translate('finanzen.foundations.moreBookingsLoadFailed'));
        },
      });
  }

  recordBooking(event: Event): void {
    event.preventDefault();
    const book = this.selectedBook();
    if (book === null) {
      this.actionError.set(this.i18n.translate('finanzen.foundations.bookRequired'));
      return;
    }
    const cents = euroToCents(this.bookingAmount());
    if (cents === null) {
      this.actionError.set(this.i18n.translate('finanzen.foundations.amountRequired'));
      return;
    }
    const purpose = this.purpose().trim();
    if (purpose === '') {
      this.actionError.set(this.i18n.translate('finanzen.foundations.purposeRequired'));
      return;
    }
    this.submitting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api
      .bookFoundation(book.ID, { kind: this.bookingKind(), amount_cents: cents, purpose })
      .subscribe({
        next: (booking) => {
          this.submitting.set(false);
          // The new movement is the last one by id, so appending it is only right while the screen
          // holds every movement before it. With pages still unread it is not the next row: the page
          // the offset would return starts before it, and appending would either duplicate it later
          // or show it out of order — so the screen reads the book again from its first page.
          if (this.hasMore()) {
            this.loadBookings(book);
          } else {
            this.bookings.set([...this.bookings(), booking]);
          }
          this.notice.set(this.i18n.translate('finanzen.foundations.bookingRecorded'));
          this.bookingAmount.set('');
          this.purpose.set('');
          // The balance is derived from the movements, so the server recomputes it.
          this.loadBalance(book);
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set(this.i18n.translate('finanzen.foundations.bookingFailed'));
        },
      });
  }

  /** The balance's net: income minus expense. */
  net(balance: FoundationBalance): number {
    return balance.IncomeCents - balance.ExpenseCents;
  }

  /** A movement's kind as the screen names it. */
  kindLabel(kind: string): string {
    return kind === 'income'
      ? this.i18n.translate('finanzen.foundations.income')
      : kind === 'expense'
        ? this.i18n.translate('finanzen.foundations.expense')
        : kind;
  }

  private loadBalance(book: FoundationBook): void {
    this.api.getFoundationBalance(book.ID).subscribe({
      next: (balance) => this.balance.set(balance),
      error: () => this.actionError.set(this.i18n.translate('finanzen.foundations.balanceLoadFailed')),
    });
  }
}

/**
 * Names the refusal a cancellation came back with. The two refusals ask for different things: a
 * movement already cancelled is a mistake by the operator, while a counter-movement that cannot be
 * cancelled is a rule they may not know yet.
 */
function reversalMessage(err: unknown, i18n: TranslocoService): string {
  switch (problemCode(err)) {
    case 'kasse.booking_already_reversed':
      return i18n.translate('finanzen.foundations.alreadyReversed');
    case 'kasse.reversal_not_reversible':
      return i18n.translate('finanzen.foundations.counterNotReversible');
    case 'kasse.booking_not_found':
      return i18n.translate('finanzen.foundations.bookingGone');
    default:
      return i18n.translate('finanzen.foundations.reversalFailed');
  }
}

/**
 * Names the refusal a carry-over came back with, so the operator knows what to do next rather than
 * reading "Konnte das Jahr nicht abschließen" for four different reasons.
 */
function carryOverMessage(err: unknown, i18n: TranslocoService): string {
  switch (problemCode(err)) {
    case 'kasse.carry_over_not_allowed':
      return i18n.translate('finanzen.foundations.carryOverNotAllowed');
    case 'kasse.carry_over_nothing':
      return i18n.translate('finanzen.foundations.carryOverNothing');
    case 'kasse.carry_over_target_missing':
      return i18n.translate('finanzen.foundations.carryOverTargetMissing');
    case 'kasse.carry_over_target_not_empty':
      return i18n.translate('finanzen.foundations.carryOverTargetNotEmpty');
    default:
      return i18n.translate('finanzen.foundations.carryOverFailed');
  }
}
