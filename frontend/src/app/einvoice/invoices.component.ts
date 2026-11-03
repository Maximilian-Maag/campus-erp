import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { CampusErpApi, InvoiceSummary } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';

/**
 * E-invoicing — Eingangsrechnungen.
 *
 * Reads a received XRechnung in, lists the invoices, and matches one to the purchase order its
 * buyer reference names (docs/09, docs/12). The match converts that order's commitment into an
 * actual, so it is the step that turns a document into a booking.
 *
 * Only an invoice this system *received* can be matched: one we issued is going out, and its
 * row does not offer the action.
 */
@Component({
  selector: 'app-e-invoices',
  imports: [FormsModule],
  template: `
    <section aria-labelledby="einvoices-heading">
      <h2 id="einvoices-heading">Eingangsrechnungen</h2>

      <div class="import">
        <label for="xrechnung">XRechnung (XML)</label>
        <textarea
          id="xrechnung"
          name="xrechnung"
          rows="6"
          [ngModel]="xml()"
          (ngModelChange)="xml.set($event)"
        ></textarea>
        <button type="button" [disabled]="importing()" (click)="importInvoice()">
          Rechnung einlesen
        </button>
      </div>

      @if (loadError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }
      @if (actionError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }

      @if (loading()) {
        <p role="status">Rechnungen werden geladen.</p>
      } @else if (invoices().length === 0 && !loadError()) {
        <p role="status">Es sind keine Rechnungen erfasst.</p>
      } @else if (!loadError()) {
        <table>
          <caption>
            Eingangsrechnungen mit Datum, Betrag und der Bestellung, der sie zugeordnet sind
          </caption>
          <thead>
            <tr>
              <th scope="col">Nummer</th>
              <th scope="col">Richtung</th>
              <th scope="col">Datum</th>
              <th scope="col">Referenz</th>
              <th scope="col" class="amount">Betrag</th>
              <th scope="col">Zuordnung</th>
              <th scope="col">Aktion</th>
            </tr>
          </thead>
          <tbody>
            @for (invoice of invoices(); track invoice.ID) {
              <tr [attr.data-testid]="'invoice-' + invoice.Number">
                <td>{{ invoice.Number }}</td>
                <td [attr.data-testid]="'direction-' + invoice.Number">{{ direction(invoice.Direction) }}</td>
                <td>{{ day(invoice.IssueDate) }}</td>
                <td>{{ invoice.BuyerReference }}</td>
                <td class="amount">{{ euro(invoice.GrossCents) }}</td>
                <td [attr.data-testid]="'match-' + invoice.Number">
                  @if (invoice.Direction !== 'inbound') {
                    —
                  } @else {
                    {{ invoice.MatchedOrderID ? 'Bestellung ' + invoice.MatchedOrderID : 'offen' }}
                  }
                </td>
                <td>
                  @if (invoice.Direction === 'inbound' && !invoice.MatchedOrderID) {
                    <button
                      type="button"
                      [disabled]="matching() !== null"
                      [attr.aria-label]="'Rechnung ' + invoice.Number + ' der Bestellung zuordnen'"
                      (click)="match(invoice)"
                    >
                      Zuordnen
                    </button>
                  }
                </td>
              </tr>
            }
          </tbody>
        </table>
      }
    </section>
  `,
})
export class EInvoicesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly invoices = signal<InvoiceSummary[]>([]);
  readonly xml = signal('');
  readonly loading = signal(true);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  /** The document being read in, and the invoice being matched, so neither can be submitted twice. */
  readonly importing = signal(false);
  readonly matching = signal<number | null>(null);

  // Loading starts when the screen is shown, not when it is constructed: the initial state is
  // then observable (and testable) before any request is made.
  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listEInvoices().subscribe({
      next: (invoices) => {
        this.invoices.set(invoices ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Konnte die Rechnungen nicht laden.');
        this.loading.set(false);
      },
    });
  }

  importInvoice(): void {
    if (this.xml().trim() === '') {
      this.actionError.set('Es ist kein XML eingegeben.');
      return;
    }
    this.importing.set(true);
    this.actionError.set(null);
    this.api.receiveEInvoice(this.xml()).subscribe({
      next: () => {
        this.importing.set(false);
        this.xml.set('');
        this.load();
      },
      error: () => {
        this.importing.set(false);
        this.actionError.set('Konnte die Rechnung nicht einlesen.');
      },
    });
  }

  match(invoice: InvoiceSummary): void {
    this.matching.set(invoice.ID);
    this.actionError.set(null);
    this.api.matchEInvoice(invoice.ID).subscribe({
      next: () => {
        this.matching.set(null);
        this.load();
      },
      error: () => {
        this.matching.set(null);
        this.actionError.set('Konnte die Rechnung ' + invoice.Number + ' nicht zuordnen.');
      },
    });
  }

  /** Which way the invoice goes, in the words the screen uses. */
  direction(direction: string): string {
    switch (direction) {
      case 'inbound':
        return 'Eingang';
      case 'outbound':
        return 'Ausgang';
      default:
        return direction;
    }
  }

  /** Dates and amounts come from the shared formatters, not from each screen's own. */
  readonly day = formatDay;
  readonly euro = formatCents;
}
