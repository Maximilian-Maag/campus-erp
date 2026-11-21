import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, CashAccount, StatementLine, StatementSummary } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

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
 * the screen does not have to guess. All copy is a message key; the API's own status values are
 * shown as the API reports them.
 */
@Component({
  selector: 'app-bank-statements',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.kasseKontoauszuege.title" subtitleKey="page.kasseKontoauszuege.subtitle">
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

        @if (loading()) {
          <p role="status">{{ 'kasse.statements.loading' | transloco }}</p>
        } @else if (accounts().length === 0 && !loadError()) {
          <p role="status">{{ 'kasse.accounts.none' | transloco }}</p>
        } @else if (!loadError()) {
          <div class="picker">
            <label for="statement-account">{{ 'common.account' | transloco }}</label>
            <select id="statement-account" [value]="account()" (change)="choose($any($event.target).value)">
              @for (a of accounts(); track a.ID) {
                <option [value]="a.ID">{{ a.Name }} ({{ a.IBAN }})</option>
              }
            </select>
          </div>

          @if (statements().length === 0) {
            <p role="status">{{ 'kasse.statements.empty' | transloco }}</p>
          } @else {
            <table>
              <caption>
                {{ 'kasse.statements.caption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'common.reference' | transloco }}</th>
                  <th scope="col">{{ 'common.number' | transloco }}</th>
                  <th scope="col">{{ 'kasse.statements.closing' | transloco }}</th>
                  <th scope="col" class="amount">{{ 'kasse.statements.openingBalance' | transloco }}</th>
                  <th scope="col" class="amount">{{ 'kasse.statements.closingBalance' | transloco }}</th>
                  <th scope="col">{{ 'kasse.statements.lines' | transloco }}</th>
                  <th scope="col">{{ 'common.action' | transloco }}</th>
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
                        [attr.aria-label]="'kasse.statements.linesLabel' | transloco: { reference: statement.Reference }"
                        (click)="toggleLines(statement)"
                      >
                        {{ 'kasse.statements.lines' | transloco }}
                      </button>
                    </td>
                  </tr>
                  @if (open() === statement.ID) {
                    <tr [attr.data-testid]="'lines-' + statement.Reference">
                      <td colspan="7">
                        @if (lines().length === 0) {
                          <p role="status">{{ 'kasse.statements.noLines' | transloco }}</p>
                        } @else {
                          <table>
                            <caption>
                              {{ 'kasse.statements.linesCaption' | transloco: { reference: statement.Reference } }}
                            </caption>
                            <thead>
                              <tr>
                                <th scope="col">{{ 'kasse.statements.lineNo' | transloco }}</th>
                                <th scope="col">{{ 'kasse.statements.bookingDate' | transloco }}</th>
                                <th scope="col" class="amount">{{ 'common.amount' | transloco }}</th>
                                <th scope="col">{{ 'common.purpose' | transloco }}</th>
                                <th scope="col">{{ 'common.status' | transloco }}</th>
                              </tr>
                            </thead>
                            <tbody>
                              @for (line of lines(); track line.LineNo) {
                                <tr [attr.data-testid]="'line-' + line.LineNo">
                                  <td>{{ line.LineNo }}</td>
                                  <td>{{ day(line.BookingDate) }}</td>
                                  <td class="amount">{{ euro(line.AmountCents) }}</td>
                                  <td>{{ line.Purpose }}</td>
                                  <td>{{ line.Status ? line.Status : ('kasse.statements.lineOpen' | transloco) }}</td>
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
            @if (hasMore()) {
              <p>
                <button
                  type="button"
                  data-testid="load-more"
                  [disabled]="loadingMore()"
                  (click)="loadMore()"
                >
                  {{ (loadingMore() ? 'kasse.common.loadingMore' : 'kasse.common.loadMore') | transloco }}
                </button>
              </p>
            }
          }

          <form (submit)="import($event)">
            <h2 id="statement-import-heading">{{ 'kasse.statements.importHeading' | transloco }}</h2>
            <div class="field">
              <label for="statement-content">{{ 'kasse.statements.content' | transloco }}</label>
              <textarea
                id="statement-content"
                rows="6"
                [value]="content()"
                (input)="content.set($any($event.target).value)"
              ></textarea>
            </div>
            <div class="field">
              <button type="submit" [disabled]="importing()">
                {{ 'kasse.statements.import' | transloco }}
              </button>
            </div>
          </form>
        }
      </section>
    </app-page>
  `,
})
export class BankStatementsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly accounts = signal<CashAccount[]>([]);
  readonly statements = signal<StatementSummary[]>([]);
  /**
   * One page of statements, sent explicitly rather than left to the server's default: a screen whose
   * page size changed when the server did would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
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
        this.loadError.set(this.i18n.translate('kasse.accounts.loadFailed'));
        this.loading.set(false);
      },
    });
  }

  loadStatements(accountID: string): void {
    this.open.set(null);
    this.lines.set([]);
    this.hasMore.set(false);
    this.api.listBankStatements(accountID, String(this.pageSize), '0').subscribe({
      next: (statements) => {
        const page = statements ?? [];
        this.statements.set(page);
        // A full page means there may be more; a short one is the end of the list.
        this.hasMore.set(page.length === this.pageSize);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('kasse.statements.loadFailed'));
        this.loading.set(false);
      },
    });
  }

  /**
   * Reads the next page of the chosen account and appends it. The offset is the number of statements
   * already on screen, which is why the list is never re-sorted in the browser: the pages only line up
   * while the order the server answered with is the order they are shown in.
   *
   * A failed page is reported without clearing what is there — the statements already read are still
   * valid, and blanking the screen would suggest they are not.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api
      .listBankStatements(this.account(), String(this.pageSize), String(this.statements().length))
      .subscribe({
        next: (statements) => {
          const page = statements ?? [];
          this.statements.set([...this.statements(), ...page]);
          this.hasMore.set(page.length === this.pageSize);
          this.loadingMore.set(false);
        },
        error: () => {
          this.loadingMore.set(false);
          this.actionError.set(this.i18n.translate('kasse.statements.loadMoreFailed'));
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
        this.actionError.set(
          this.i18n.translate('kasse.statements.linesFailed', { reference: statement.Reference })
        );
      },
    });
  }

  import(event: Event): void {
    event.preventDefault();
    const content = this.content().trim();
    if (content === '') {
      this.actionError.set(this.i18n.translate('kasse.statements.contentRequired'));
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
        this.notice.set(
          count === 1
            ? this.i18n.translate('kasse.statements.importedOne')
            : this.i18n.translate('kasse.statements.importedMany', { count })
        );
        this.loadStatements(this.account());
      },
      error: () => {
        this.importing.set(false);
        this.actionError.set(this.i18n.translate('kasse.statements.importFailed'));
      },
    });
  }

  /** Amounts and dates are rendered by the shared formatters, not by each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;
}
