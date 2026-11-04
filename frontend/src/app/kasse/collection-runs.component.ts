import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, CashAccount, CollectionRun } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';

/**
 * Kasse — Einzüge (SEPA direct-debit collection runs).
 *
 * A run selects the open receivables due by a date and produces one pain.008 file. The screen
 * offers the university's own accounts, states the run, and shows the file the run produced. What
 * the run can gather — an item due, under an active mandate matching the sequence type — is the
 * API's decision: the screen reports a refusal (nothing due, a foreign currency, a due-by after
 * the collection date) as it comes back rather than re-deriving the rule.
 */
@Component({
  selector: 'app-collection-runs',
  template: `
    <section aria-labelledby="collection-runs-heading">
      <h2 id="collection-runs-heading">Einzüge</h2>

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
      } @else if (accounts().length === 0 && !loadError()) {
        <p role="status">Es ist kein eigenes Konto angelegt.</p>
      } @else if (!loadError()) {
        <form (submit)="run($event)">
          <h3 id="collection-run-new-heading">Einzug erstellen</h3>
          <p>
            <label for="run-reference">Referenz</label>
            <input
              id="run-reference"
              type="text"
              [value]="reference()"
              (input)="reference.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="run-account">Konto</label>
            <select id="run-account" [value]="account()" (change)="account.set($any($event.target).value)">
              @for (a of accounts(); track a.ID) {
                <option [value]="a.ID">{{ a.Name }} ({{ a.IBAN }})</option>
              }
            </select>
          </p>
          <p>
            <label for="run-creditor">Gläubiger-ID</label>
            <input
              id="run-creditor"
              type="text"
              [value]="creditorID()"
              (input)="creditorID.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="run-collection-date">Einzugsdatum</label>
            <input
              id="run-collection-date"
              type="date"
              [value]="collectionDate()"
              (input)="collectionDate.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="run-due-by">Fälligkeit bis</label>
            <input
              id="run-due-by"
              type="date"
              [value]="dueBy()"
              (input)="dueBy.set($any($event.target).value)"
            />
          </p>
          <p>
            <label for="run-sequence">Sequenztyp</label>
            <select id="run-sequence" [value]="sequenceType()" (change)="sequenceType.set($any($event.target).value)">
              <option value="FRST">FRST — erstmalig</option>
              <option value="RCUR">RCUR — wiederkehrend</option>
              <option value="OOFF">OOFF — einmalig</option>
              <option value="FNAL">FNAL — letztmalig</option>
            </select>
          </p>
          <p>
            <label for="run-instrument">Local Instrument</label>
            <select id="run-instrument" [value]="localInstrument()" (change)="localInstrument.set($any($event.target).value)">
              <option value="CORE">CORE</option>
              <option value="B2B">B2B</option>
            </select>
          </p>
          <button type="submit" [disabled]="submitting()">Einzug erstellen</button>
        </form>

        @if (created(); as r) {
          <section aria-labelledby="collection-run-result-heading">
            <h3 id="collection-run-result-heading">Erzeugter Einzug</h3>
            <p role="status" [attr.data-testid]="'run-' + r.Reference">
              {{ r.Reference }}: {{ r.ItemCount }} Posten, Summe {{ euro(r.SumCents) }},
              Einzugsdatum {{ day(r.CollectionDate) }}
            </p>
            <button type="button" (click)="fetchFile()">Datei anzeigen</button>
            @if (file(); as f) {
              <pre [attr.data-testid]="'run-file-' + r.Reference">{{ f }}</pre>
            }
          </section>
        }
      }
    </section>
  `,
})
export class CollectionRunsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly accounts = signal<CashAccount[]>([]);
  readonly account = signal('');
  readonly loading = signal(true);
  readonly submitting = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  readonly created = signal<CollectionRun | null>(null);
  readonly file = signal<string | null>(null);

  readonly reference = signal('');
  readonly creditorID = signal('');
  readonly collectionDate = signal('');
  readonly dueBy = signal('');
  readonly sequenceType = signal('RCUR');
  readonly localInstrument = signal('CORE');

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
        this.account.set(this.account() || String(list[0]?.ID ?? ''));
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Konnte die Konten nicht laden.');
        this.loading.set(false);
      },
    });
  }

  /** Creates a run that selects the open receivables due by the entered date. */
  run(event: Event): void {
    event.preventDefault();
    const reference = this.reference().trim();
    if (reference === '') {
      this.actionError.set('Eine Referenz ist erforderlich.');
      return;
    }
    if (this.collectionDate().trim() === '' || this.dueBy().trim() === '') {
      this.actionError.set('Einzugsdatum und Fälligkeit sind erforderlich.');
      return;
    }
    this.submitting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.created.set(null);
    this.file.set(null);
    this.api
      .createCollectionRunFromReceivables({
        reference,
        account_id: Number(this.account()),
        creditor_id: this.creditorID().trim(),
        collection_date: this.collectionDate().trim(),
        sequence_type: this.sequenceType(),
        local_instrument: this.localInstrument(),
        due_by: this.dueBy().trim(),
      })
      .subscribe({
        next: (run) => {
          this.submitting.set(false);
          this.created.set(run);
          this.notice.set('Einzug erstellt.');
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set('Konnte den Einzug nicht erstellen.');
        },
      });
  }

  /** Reads the file the run produced (the stored pain.008). */
  fetchFile(): void {
    const run = this.created();
    if (!run) {
      return;
    }
    this.api.getCollectionRunFile(run.Reference).subscribe({
      next: (file) => this.file.set(file),
      error: () => this.actionError.set('Konnte die Datei nicht laden.'),
    });
  }

  /** Amounts and dates are rendered by the shared formatters, not by each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;
}
