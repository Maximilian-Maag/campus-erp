import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, CashAccount, KasseOrderSummary, PaymentRun } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';

/**
 * Kasse — Zahlungsläufe (SEPA credit-transfer payment runs).
 *
 * The credit-transfer counterpart of the collection run: a run gathers approved payment orders
 * into one pain.001 file for one execution date, and stores the file so the run is reproducible.
 * The screen offers one of the university's own accounts, an execution date and the approved
 * orders to include, creates the run, and then shows the file it produced — which can also be
 * handed to the bank channel.
 *
 * Which orders a run may gather — approved ones, each paid at most once — is the API's decision:
 * the screen offers the orders the server reports as approved and reports a refusal (an
 * unapproved order, an already-paid one, a duplicate reference) as it comes back rather than
 * re-deriving the rule.
 */
@Component({
  selector: 'app-payment-runs',
  template: `
    <section aria-labelledby="payment-runs-heading">
      <h2 id="payment-runs-heading">Zahlungsläufe</h2>

      @if (loadError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }
      @if (actionError(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }
      @if (notice(); as n) {
        <p role="status" class="notice">{{ n }}</p>
      }

      @if (loading()) {
        <p role="status">Konten und Anordnungen werden geladen.</p>
      } @else if (accounts().length === 0 && !loadError()) {
        <p role="status">Es ist kein eigenes Konto angelegt.</p>
      } @else if (!loadError()) {
        @if (orders().length === 0) {
          <p role="status">Es ist keine genehmigte Zahlungsanordnung vorhanden.</p>
        } @else {
          <form (submit)="create($event)">
            <h3 id="payment-run-new-heading">Zahlungslauf erstellen</h3>
            <p>
              <label for="payment-run-reference">Referenz</label>
              <input
                id="payment-run-reference"
                type="text"
                [value]="reference()"
                (input)="reference.set($any($event.target).value)"
              />
            </p>
            <p>
              <label for="payment-run-account">Konto</label>
              <select
                id="payment-run-account"
                [value]="account()"
                (change)="account.set($any($event.target).value)"
              >
                @for (a of accounts(); track a.ID) {
                  <option [value]="a.ID">{{ a.Name }} ({{ a.IBAN }})</option>
                }
              </select>
            </p>
            <p>
              <label for="payment-run-execution-date">Ausführungsdatum</label>
              <input
                id="payment-run-execution-date"
                type="date"
                [value]="executionDate()"
                (input)="executionDate.set($any($event.target).value)"
              />
            </p>

            <h4 id="payment-run-orders-heading">Genehmigte Zahlungsanordnungen</h4>
            <table>
              <caption>
                Genehmigte Anordnungen mit Verwendungszweck, Betrag und Fälligkeit
              </caption>
              <thead>
                <tr>
                  <th scope="col">Auswahl</th>
                  <th scope="col">Referenz</th>
                  <th scope="col">Verwendungszweck</th>
                  <th scope="col" class="amount">Betrag</th>
                  <th scope="col">Fällig</th>
                </tr>
              </thead>
              <tbody>
                @for (order of orders(); track order.ID) {
                  <tr [attr.data-testid]="'order-' + order.Reference">
                    <td>
                      <input
                        type="checkbox"
                        [checked]="isSelected(order.ID)"
                        (change)="toggle(order.ID)"
                        [attr.aria-label]="'Anordnung ' + order.Reference + ' auswählen'"
                      />
                    </td>
                    <td>{{ order.Reference }}</td>
                    <td>{{ order.Purpose }}</td>
                    <td class="amount">{{ euro(order.AmountCents) }}</td>
                    <td>{{ day(order.DueDate) }}</td>
                  </tr>
                }
              </tbody>
            </table>
            <p role="status" [attr.data-testid]="'selected-sum'">
              Ausgewählt: {{ selected().length }} Anordnungen, Summe {{ euro(selectedSum()) }}
            </p>
            <button type="submit" [disabled]="submitting()">Zahlungslauf erstellen</button>
          </form>
        }

        @if (created(); as r) {
          <section aria-labelledby="payment-run-result-heading">
            <h3 id="payment-run-result-heading">Erzeugter Zahlungslauf</h3>
            <p role="status" [attr.data-testid]="'run-' + r.Reference">
              {{ r.Reference }}: {{ r.ItemCount }} Posten, Summe {{ euro(r.SumCents) }},
              Ausführung {{ day(r.ExecutionDate) }}
            </p>
            <button type="button" (click)="fetchFile()">Datei anzeigen</button>
            <button type="button" [disabled]="exporting()" (click)="queueExport()">
              An Bankkanal übergeben
            </button>
            @if (file(); as f) {
              <pre [attr.data-testid]="'run-file-' + r.Reference">{{ f }}</pre>
            }
          </section>
        }
      }
    </section>
  `,
})
export class PaymentRunsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly accounts = signal<CashAccount[]>([]);
  readonly account = signal('');
  readonly orders = signal<KasseOrderSummary[]>([]);
  readonly loading = signal(true);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  readonly reference = signal('');
  readonly executionDate = signal('');
  /** The ids of the approved orders this run is to gather. */
  readonly selected = signal<number[]>([]);
  readonly submitting = signal(false);
  readonly exporting = signal(false);
  readonly created = signal<PaymentRun | null>(null);
  readonly file = signal<string | null>(null);

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listCashAccounts().subscribe({
      next: (accounts) => {
        const list = accounts ?? [];
        this.accounts.set(list);
        const id = this.account() || String(list[0]?.ID ?? '');
        this.account.set(id);
        if (id) {
          this.loadOrders();
        } else {
          this.loading.set(false);
        }
      },
      error: () => {
        this.loadError.set('Konnte die Konten nicht laden.');
        this.loading.set(false);
      },
    });
  }

  /** Loads the orders a run may gather — only the approved ones, the server's filter. */
  loadOrders(): void {
    this.api.listPaymentOrders('approved').subscribe({
      next: (orders) => {
        this.orders.set(orders ?? []);
        // An order that is no longer offered cannot be gathered, so a stale selection is dropped.
        this.selected.set(
          this.selected().filter((id) => this.orders().some((o) => o.ID === id))
        );
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Konnte die Zahlungsanordnungen nicht laden.');
        this.loading.set(false);
      },
    });
  }

  isSelected(id: number): boolean {
    return this.selected().includes(id);
  }

  toggle(id: number): void {
    this.selected.set(
      this.isSelected(id) ? this.selected().filter((s) => s !== id) : [...this.selected(), id]
    );
  }

  /** The sum of the selected orders, so the operator sees what the run will move. */
  selectedSum(): number {
    return this.orders()
      .filter((o) => this.isSelected(o.ID))
      .reduce((sum, o) => sum + o.AmountCents, 0);
  }

  create(event: Event): void {
    event.preventDefault();
    const reference = this.reference().trim();
    if (reference === '') {
      this.actionError.set('Eine Referenz ist erforderlich.');
      return;
    }
    if (this.executionDate().trim() === '') {
      this.actionError.set('Ein Ausführungsdatum ist erforderlich.');
      return;
    }
    if (this.selected().length === 0) {
      this.actionError.set('Mindestens eine Anordnung ist erforderlich.');
      return;
    }
    this.submitting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.created.set(null);
    this.file.set(null);
    this.api
      .createPaymentRun({
        reference,
        account_id: Number(this.account()),
        execution_date: this.executionDate().trim(),
        order_ids: this.selected(),
      })
      .subscribe({
        next: (run) => {
          this.submitting.set(false);
          this.created.set(run);
          this.notice.set('Zahlungslauf erstellt.');
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set('Konnte den Zahlungslauf nicht erstellen.');
        },
      });
  }

  /** Reads the file the run produced (the stored pain.001). */
  fetchFile(): void {
    const run = this.created();
    if (!run) {
      return;
    }
    this.api.getPaymentRunFile(run.ID).subscribe({
      next: (file) => this.file.set(file),
      error: () => this.actionError.set('Konnte die Datei nicht laden.'),
    });
  }

  /** Hands the run's file to the bank channel; the API queues the work and answers with the job. */
  queueExport(): void {
    const run = this.created();
    if (!run) {
      return;
    }
    this.exporting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.requestSepaExport(run.ID).subscribe({
      next: (queued) => {
        this.exporting.set(false);
        this.notice.set('Übergabe an den Bankkanal beauftragt (Job ' + queued.job_id + ').');
      },
      error: () => {
        this.exporting.set(false);
        this.actionError.set('Konnte die Übergabe nicht beauftragen.');
      },
    });
  }

  /** Amounts and dates are rendered by the shared formatters, not by each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;
}
