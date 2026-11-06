import { Component, OnInit, inject, signal } from '@angular/core';
import {
  CampusErpApi,
  Foundation,
  FoundationBalance,
  FoundationBook,
  FoundationBooking,
} from '../core/api/generated';
import { euroToCents, formatCents } from '../core/format';

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
  template: `
    <section aria-labelledby="foundations-heading">
      <h2 id="foundations-heading">Stiftungen</h2>

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
        <h3 id="foundations-filter-heading">Nach Jahr anzeigen</h3>
        <p>
          <label for="foundation-filter-year">Jahr</label>
          <input
            id="foundation-filter-year"
            type="text"
            inputmode="numeric"
            [value]="filterYear()"
            (input)="filterYear.set($any($event.target).value)"
          />
          <button type="submit">Anzeigen</button>
        </p>
        <p>Ohne Jahr werden alle Jahre angezeigt.</p>
      </form>

      @if (loading()) {
        <p role="status">Stiftungen werden geladen.</p>
      } @else if (!loadError()) {
        @if (foundations().length === 0) {
          <p role="status">Es ist noch keine Stiftung zu diesem Jahr angelegt.</p>
        } @else {
          <table>
            <caption>
              Stiftungen mit Jahr, Nummer, Bezeichnung und Übertrag
            </caption>
            <thead>
              <tr>
                <th scope="col">Jahr</th>
                <th scope="col">Nummer</th>
                <th scope="col">Bezeichnung</th>
                <th scope="col">Übertrag</th>
                <th scope="col">Bücher</th>
              </tr>
            </thead>
            <tbody>
              @for (foundation of foundations(); track foundation.ID) {
                <tr [attr.data-testid]="'foundation-' + foundation.ID">
                  <td>{{ foundation.Year }}</td>
                  <td>{{ foundation.Number }}</td>
                  <td>{{ foundation.Description }}</td>
                  <td>{{ foundation.CarryOver ? 'ja' : 'nein' }}</td>
                  <td>
                    <button type="button" (click)="select(foundation)">Bücher anzeigen</button>
                  </td>
                </tr>
              }
            </tbody>
          </table>
        }

        <form (submit)="createFoundation($event)">
          <h3 id="foundation-new-heading">Stiftung anlegen</h3>
          <p>
            <label for="foundation-year">Haushaltsjahr</label>
            <input
              id="foundation-year"
              type="text"
              inputmode="numeric"
              [value]="newYear()"
              (input)="newYear.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="foundation-number">Nummer</label>
            <input
              id="foundation-number"
              type="text"
              [value]="newNumber()"
              (input)="newNumber.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="foundation-description">Bezeichnung</label>
            <input
              id="foundation-description"
              type="text"
              [value]="newDescription()"
              (input)="newDescription.set($any($event.target).value)"
            />
          </p>
          <p>
            <input
              id="foundation-carry-over"
              type="checkbox"
              [checked]="newCarryOver()"
              (change)="newCarryOver.set($any($event.target).checked)"
            />
            <label for="foundation-carry-over">Übertrag möglich</label>
          </p>
          <button type="submit" [disabled]="submitting()">Stiftung anlegen</button>
        </form>
      }

      @if (selectedFoundation(); as foundation) {
        <section
          aria-labelledby="books-heading"
          [attr.data-testid]="'books-of-' + foundation.ID"
        >
          <h3 id="books-heading">Stiftungsbücher — {{ foundation.Year }}/{{ foundation.Number }}</h3>

          @if (booksLoading()) {
            <p role="status">Bücher werden geladen.</p>
          } @else if (books().length === 0) {
            <p role="status">Diese Stiftung hat noch kein Buch.</p>
          } @else {
            <table>
              <caption>
                Bücher dieser Stiftung mit Referenz, Währung und anlegendem Konto
              </caption>
              <thead>
                <tr>
                  <th scope="col">Referenz</th>
                  <th scope="col">Währung</th>
                  <th scope="col">Angelegt von</th>
                  <th scope="col">Buchungen</th>
                </tr>
              </thead>
              <tbody>
                @for (book of books(); track book.ID) {
                  <tr [attr.data-testid]="'book-' + book.ID">
                    <td>{{ book.Reference }}</td>
                    <td>{{ book.Currency }}</td>
                    <td>{{ book.CreatedBy }}</td>
                    <td>
                      <button type="button" (click)="selectBook(book)">Buchungen anzeigen</button>
                    </td>
                  </tr>
                }
              </tbody>
            </table>
          }

          <form (submit)="createBook($event)">
            <h4 id="book-new-heading">Buch anlegen</h4>
            <p>
              <label for="book-reference">Referenz</label>
              <input
                id="book-reference"
                type="text"
                [value]="reference()"
                (input)="reference.set($any($event.target).value)"
              />
            </p>
            <p>
              <label for="book-currency">Währung</label>
              <input
                id="book-currency"
                type="text"
                [value]="currency()"
                (input)="currency.set($any($event.target).value)"
              />
            </p>
            <button type="submit" [disabled]="submitting()">Buch anlegen</button>
          </form>

          @if (selectedBook(); as book) {
            <section aria-labelledby="bookings-heading" [attr.data-testid]="'bookings-of-' + book.ID">
              <h4 id="bookings-heading">Buchungen — {{ book.Reference }}</h4>

              @if (balance(); as b) {
                <p>
                  Einnahmen {{ euro(b.IncomeCents) }} — Ausgaben {{ euro(b.ExpenseCents) }} — Saldo
                  {{ euro(net(b)) }}
                </p>
              }

              @if (bookings().length === 0) {
                <p role="status">Dieses Buch hat noch keine Buchung.</p>
              } @else {
                <table>
                  <caption>
                    Buchungen dieses Buches mit Art, Betrag, Verwendungszweck und erfassendem Konto
                  </caption>
                  <thead>
                    <tr>
                      <th scope="col">Art</th>
                      <th scope="col">Betrag</th>
                      <th scope="col">Verwendungszweck</th>
                      <th scope="col">Erfasst von</th>
                    </tr>
                  </thead>
                  <tbody>
                    @for (booking of bookings(); track booking.ID) {
                      <tr [attr.data-testid]="'booking-' + booking.ID">
                        <td>{{ kindLabel(booking.Kind) }}</td>
                        <td>{{ euro(booking.AmountCents) }}</td>
                        <td>{{ booking.Purpose }}</td>
                        <td>{{ booking.BookedBy }}</td>
                      </tr>
                    }
                  </tbody>
                </table>
              }

              <form (submit)="recordBooking($event)">
                <h5 id="booking-new-heading">Buchung erfassen</h5>
                <p>
                  <label for="booking-kind">Art</label>
                  <select
                    id="booking-kind"
                    [value]="bookingKind()"
                    (change)="bookingKind.set($any($event.target).value)"
                  >
                    <option value="income">Einnahme</option>
                    <option value="expense">Ausgabe</option>
                  </select>
                </p>
                <p>
                  <label for="booking-amount">Betrag in Euro</label>
                  <input
                    id="booking-amount"
                    type="text"
                    inputmode="decimal"
                    [value]="bookingAmount()"
                    (input)="bookingAmount.set($any($event.target).value)"
                  />
                </p>
                <p>
                  <label for="booking-purpose">Verwendungszweck</label>
                  <input
                    id="booking-purpose"
                    type="text"
                    [value]="purpose()"
                    (input)="purpose.set($any($event.target).value)"
                  />
                </p>
                <button type="submit" [disabled]="submitting()">Buchung erfassen</button>
              </form>
            </section>
          }
        </section>
      }
    </section>
  `,
})
export class FoundationsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

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
  readonly balance = signal<FoundationBalance | null>(null);
  readonly bookingKind = signal('income');
  readonly bookingAmount = signal('');
  readonly purpose = signal('');

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
        this.loadError.set('Konnte die Stiftungen nicht laden.');
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
      this.actionError.set('Ein Jahr ist erforderlich.');
      return;
    }
    const number = this.newNumber().trim();
    if (number === '') {
      this.actionError.set('Eine Nummer ist erforderlich.');
      return;
    }
    const description = this.newDescription().trim();
    if (description === '') {
      this.actionError.set('Eine Bezeichnung ist erforderlich.');
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
          this.notice.set('Stiftung angelegt.');
          this.newNumber.set('');
          this.newDescription.set('');
          // The list is the server's answer to the filter above, so it is read again rather than
          // appended to: an appended foundation would sit in a year the filter excludes.
          this.loadFoundations();
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set('Konnte die Stiftung nicht anlegen.');
        },
      });
  }

  /** Selects a foundation and reads its books. */
  select(foundation: Foundation): void {
    this.selectedFoundation.set(foundation);
    this.selectedBook.set(null);
    this.bookings.set([]);
    this.balance.set(null);
    this.booksLoading.set(true);
    this.actionError.set(null);
    this.api.listFoundationBooks(foundation.ID).subscribe({
      next: (books) => {
        this.books.set(books ?? []);
        this.booksLoading.set(false);
      },
      error: () => {
        this.actionError.set('Konnte die Bücher nicht laden.');
        this.booksLoading.set(false);
      },
    });
  }

  createBook(event: Event): void {
    event.preventDefault();
    const foundation = this.selectedFoundation();
    if (foundation === null) {
      this.actionError.set('Zuerst eine Stiftung wählen.');
      return;
    }
    const reference = this.reference().trim();
    if (reference === '') {
      this.actionError.set('Eine Referenz ist erforderlich.');
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
          this.notice.set('Buch angelegt.');
          this.reference.set('');
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set('Konnte das Buch nicht anlegen.');
        },
      });
  }

  /** Selects a book and reads its movements and its derived balance. */
  selectBook(book: FoundationBook): void {
    this.selectedBook.set(book);
    this.actionError.set(null);
    this.api.listFoundationBookings(book.ID).subscribe({
      next: (bookings) => this.bookings.set(bookings ?? []),
      error: () => this.actionError.set('Konnte die Buchungen nicht laden.'),
    });
    this.loadBalance(book);
  }

  recordBooking(event: Event): void {
    event.preventDefault();
    const book = this.selectedBook();
    if (book === null) {
      this.actionError.set('Zuerst ein Buch wählen.');
      return;
    }
    const cents = euroToCents(this.bookingAmount());
    if (cents === null) {
      this.actionError.set('Ein Betrag in Euro ist erforderlich.');
      return;
    }
    const purpose = this.purpose().trim();
    if (purpose === '') {
      this.actionError.set('Ein Verwendungszweck ist erforderlich.');
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
          this.bookings.set([...this.bookings(), booking]);
          this.notice.set('Buchung erfasst.');
          this.bookingAmount.set('');
          this.purpose.set('');
          // The balance is derived from the movements, so the server recomputes it.
          this.loadBalance(book);
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set('Konnte die Buchung nicht erfassen.');
        },
      });
  }

  /** The balance's net: income minus expense. */
  net(balance: FoundationBalance): number {
    return balance.IncomeCents - balance.ExpenseCents;
  }

  /** A movement's kind as the screen names it. */
  kindLabel(kind: string): string {
    return kind === 'income' ? 'Einnahme' : kind === 'expense' ? 'Ausgabe' : kind;
  }

  private loadBalance(book: FoundationBook): void {
    this.api.getFoundationBalance(book.ID).subscribe({
      next: (balance) => this.balance.set(balance),
      error: () => this.actionError.set('Konnte den Saldo nicht laden.'),
    });
  }
}
