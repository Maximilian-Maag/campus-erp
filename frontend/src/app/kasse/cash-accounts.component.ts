import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, CashAccount } from '../core/api/generated';

/**
 * Kasse — Eigene Konten (the university's own bank accounts).
 *
 * Every Kasse screen offers one of the university's own accounts before it can ask about one, and
 * until now there was no way to create the first: a fresh install dead-ended on "es ist kein
 * eigenes Konto angelegt". This is where an account is entered — the master data the payment run,
 * the collection run, the statements and the cash book all draw on.
 *
 * The IBAN and BIC are the API's to validate (ISO 7064 mod-97 and ISO 9362, shared with the rest
 * of Kasse): the screen states what it is handed and reports a refusal as it comes back rather
 * than re-deriving the rule. Amounts are not shown here — an account carries none.
 */
@Component({
  selector: 'app-cash-accounts',
  template: `
    <section aria-labelledby="cash-accounts-heading">
      <h2 id="cash-accounts-heading">Eigene Konten</h2>

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
        <p role="status">Konten werden geladen.</p>
      } @else if (!loadError()) {
        @if (accounts().length === 0) {
          <p role="status">Es ist noch kein eigenes Konto angelegt.</p>
        } @else {
          <table>
            <caption>
              Eigene Konten mit Name, IBAN, BIC und Währung
            </caption>
            <thead>
              <tr>
                <th scope="col">Name</th>
                <th scope="col">IBAN</th>
                <th scope="col">BIC</th>
                <th scope="col">Währung</th>
              </tr>
            </thead>
            <tbody>
              @for (account of accounts(); track account.ID) {
                <tr [attr.data-testid]="'account-' + account.ID">
                  <td>{{ account.Name }}</td>
                  <td class="iban">{{ account.IBAN }}</td>
                  <td>{{ account.BIC }}</td>
                  <td>{{ account.Currency }}</td>
                </tr>
              }
            </tbody>
          </table>
        }

        <form (submit)="create($event)">
          <h3 id="cash-account-new-heading">Konto anlegen</h3>
          <p>
            <label for="account-name">Name</label>
            <input
              id="account-name"
              type="text"
              [value]="name()"
              (input)="name.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="account-iban">IBAN</label>
            <input
              id="account-iban"
              type="text"
              [value]="iban()"
              (input)="iban.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="account-bic">BIC</label>
            <input
              id="account-bic"
              type="text"
              [value]="bic()"
              (input)="bic.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="account-currency">Währung</label>
            <input
              id="account-currency"
              type="text"
              [value]="currency()"
              (input)="currency.set($any($event.target).value)"
            />
          </p>
          <button type="submit" [disabled]="submitting()">Konto anlegen</button>
        </form>
      }
    </section>
  `,
})
export class CashAccountsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly accounts = signal<CashAccount[]>([]);
  readonly loading = signal(true);
  readonly submitting = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  readonly name = signal('');
  readonly iban = signal('');
  readonly bic = signal('');
  readonly currency = signal('EUR');

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listCashAccounts().subscribe({
      next: (accounts) => {
        this.accounts.set(accounts ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Konnte die Konten nicht laden.');
        this.loading.set(false);
      },
    });
  }

  create(event: Event): void {
    event.preventDefault();
    const name = this.name().trim();
    if (name === '') {
      this.actionError.set('Ein Name ist erforderlich.');
      return;
    }
    const iban = this.iban().trim();
    if (iban === '') {
      this.actionError.set('Eine IBAN ist erforderlich.');
      return;
    }
    const currency = this.currency().trim();
    if (currency === '') {
      this.actionError.set('Eine Währung ist erforderlich.');
      return;
    }
    this.submitting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api
      .createCashAccount({ name, iban, bic: this.bic().trim(), currency })
      .subscribe({
        next: (account) => {
          this.submitting.set(false);
          // The new account joins the list the server holds; a reload would only re-read it.
          this.accounts.set([...this.accounts(), account]);
          this.notice.set('Konto angelegt.');
          this.name.set('');
          this.iban.set('');
          this.bic.set('');
          this.currency.set('EUR');
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set('Konnte das Konto nicht anlegen.');
        },
      });
  }
}
