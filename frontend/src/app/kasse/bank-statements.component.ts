import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, CashAccount, StatementLine, StatementSummary } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';

/**
 * Kasse — Kontoauszüge (imported bank statements).
 *
 * Lists the imported statements for one of the university's own accounts — newest closing date
 * first — and imports an MT940 file by pasting its content. A statement's lines can be opened
 * without leaving the page.
 *
 * The account filter is the server's (`?account_id=`), so the screen shows one account's
 * statements rather than holding every statement and sifting. Whether an import is a *new* file
 * is the API's decision too: importing the same file twice returns what was already stored, so
 * the screen does not have to guess.
 */
@Component({
  selector: 'app-bank-statements',
  template: `
    <section aria-labelledby="statements-heading">
      <h2 id="statements-heading">Kontoauszüge</h2>

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
        <p role="status">Kontoauszüge werden geladen.</p>
      } @else if (accounts().length === 0 && !loadError()) {
        <p role="status">Es ist kein eigenes Konto angelegt.</p>
      } @else if (!loadError()) {
        <div class="picker">
          <label for="statement-account">Konto</label>
          <select id="statement-account" [value]="account()" (change)="choose($any($event.target).value)">
            @for (a of accounts(); track a.ID) {
              <option [value]="a.ID">{{ a.Name }} ({{ a.IBAN }})</option>
            }
          </select>
        </div>

        @if (statements().length === 0) {
          <p role="status">Für dieses Konto ist kein Auszug eingelesen.</p>
        } @else {
          <table>
            <caption>
              Eingelesene Kontoauszüge mit Zeitraum, Anfangs- und Endsaldo
            </caption>
            <thead>
              <tr>
                <th scope="col">Referenz</th>
                <th scope="col">Nummer</th>
                <th scope="col">Abschluss</th>
                <th scope="col" class="amount">Anfangssaldo</th>
                <th scope="col" class="amount">Endsaldo</th>
                <th scope="col">Zeilen</th>
                <th scope="col">Aktion</th>
              </tr>
            </thead>
            <tbody>
              @for (statement of statements(); track statement.ID) {
                <tr [attr.data-testid]="'statement-' + statement.Reference">
                  <td>{{ statement.Reference }}</td>
                  <td>{{ statement.Number }}</td>
                  <td>{{ day(statement.ClosingDate) }}</td>
                  <td class="amount">{{ euro(statement.OpeningBalanceCents) }}</td>
                  <td class="amount">{{ euro(statement.ClosingBalanceCents) }}</td>
                  <td>{{ statement.LineCount }}</td>
                  <td>
                    <button
                      type="button"
                      [attr.aria-expanded]="open() === statement.ID"
                      [attr.aria-label]="'Zeilen von Auszug ' + statement.Reference + ' anzeigen'"
                      (click)="toggleLines(statement)"
                    >
                      Zeilen
                    </button>
                  </td>
                </tr>
                @if (open() === statement.ID) {
                  <tr [attr.data-testid]="'lines-' + statement.Reference">
                    <td colspan="7">
                      @if (lines().length === 0) {
                        <p role="status">Dieser Auszug hat keine Zeilen.</p>
                      } @else {
                        <table>
                          <caption>
                            Zeilen des Auszugs {{ statement.Reference }}
                          </caption>
                          <thead>
                            <tr>
                              <th scope="col">Nr.</th>
                              <th scope="col">Buchungstag</th>
                              <th scope="col" class="amount">Betrag</th>
                              <th scope="col">Verwendungszweck</th>
                              <th scope="col">Status</th>
                            </tr>
                          </thead>
                          <tbody>
                            @for (line of lines(); track line.LineNo) {
                              <tr [attr.data-testid]="'line-' + line.LineNo">
                                <td>{{ line.LineNo }}</td>
                                <td>{{ day(line.BookingDate) }}</td>
                                <td class="amount">{{ euro(line.AmountCents) }}</td>
                                <td>{{ line.Purpose }}</td>
                                <td>{{ line.Status || 'offen' }}</td>
                              </tr>
                            }
                          </tbody>
                        </table>
                      }
                    </td>
                  </tr>
                }
              }
            </tbody>
          </table>
        }

        <form (submit)="import($event)">
          <h3 id="statement-import-heading">Auszug einlesen</h3>
          <p>
            <label for="statement-content">Inhalt der MT940-Datei</label>
            <textarea
              id="statement-content"
              rows="6"
              [value]="content()"
              (input)="content.set($any($event.target).value)"
            ></textarea>
          </p>
          <button type="submit" [disabled]="importing()">Einlesen</button>
        </form>
      }
    </section>
  `,
})
export class BankStatementsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly accounts = signal<CashAccount[]>([]);
  readonly statements = signal<StatementSummary[]>([]);
  readonly account = signal('');
  readonly content = signal('');
  readonly loading = signal(true);
  readonly importing = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  /** The statement whose lines are open, if any. */
  readonly open = signal<number | null>(null);
  readonly lines = signal<StatementLine[]>([]);

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
          this.loadStatements(id);
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

  loadStatements(accountID: string): void {
    this.open.set(null);
    this.lines.set([]);
    this.api.listBankStatements(accountID).subscribe({
      next: (statements) => {
        this.statements.set(statements ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Konnte die Kontoauszüge nicht laden.');
        this.loading.set(false);
      },
    });
  }

  choose(accountID: string): void {
    this.account.set(accountID);
    this.notice.set(null);
    this.actionError.set(null);
    this.loading.set(true);
    this.loadStatements(accountID);
  }

  /** Opens a statement's lines, or closes them if they are already open. */
  toggleLines(statement: StatementSummary): void {
    if (this.open() === statement.ID) {
      this.open.set(null);
      this.lines.set([]);
      return;
    }
    this.open.set(statement.ID);
    this.lines.set([]);
    this.api.getBankStatement(statement.ID).subscribe({
      next: (loaded) => this.lines.set(loaded?.Lines ?? []),
      error: () => {
        this.open.set(null);
        this.actionError.set('Konnte die Zeilen von Auszug ' + statement.Reference + ' nicht laden.');
      },
    });
  }

  import(event: Event): void {
    event.preventDefault();
    const content = this.content().trim();
    if (content === '') {
      this.actionError.set('Ein Auszug braucht Inhalt.');
      return;
    }
    this.importing.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.importBankStatement({ account_id: Number(this.account()), content }).subscribe({
      next: (statements) => {
        this.importing.set(false);
        this.content.set('');
        const count = (statements ?? []).length;
        this.notice.set(count === 1 ? 'Ein Auszug eingelesen.' : count + ' Auszüge eingelesen.');
        this.loadStatements(this.account());
      },
      error: () => {
        this.importing.set(false);
        this.actionError.set('Konnte den Auszug nicht einlesen.');
      },
    });
  }

  /** Amounts and dates are rendered by the shared formatters, not by each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;
}

