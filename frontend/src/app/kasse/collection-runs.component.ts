import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, CashAccount, CollectionItemRequest, CollectionRun, Mandate } from '../core/api/generated';
import { SEQUENCE_TYPES } from '../core/direct-debit';
import { euroToCents, formatCents, formatDay } from '../core/format';

/**
 * Kasse — Einzüge (SEPA direct-debit collection runs).
 *
 * A run produces one pain.008 file for one collection date and sequence type, and the screen
 * offers the two ways the API accepts items: gather the open receivables due by a date, or name
 * the mandates and amounts by hand. Either way the run's own fields are stated once, and the file
 * the run produced is shown.
 *
 * What the run can gather — an item due, under an active mandate matching the sequence type — is
 * the API's decision: the screen reports a refusal (nothing due, a foreign currency, a due-by
 * after the collection date) as it comes back rather than re-deriving the rule. The hand-picked
 * flow is only offered the mandates the run's sequence type can actually collect under, so a
 * choice that the API would refuse is not offered in the first place.
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
            <label for="run-sequence">Sequenztyp</label>
            <select id="run-sequence" [value]="sequenceType()" (change)="changeSequence($any($event.target).value)">
              @for (type of sequenceTypes; track type.value) {
                <option [value]="type.value">{{ type.value }} — {{ type.label }}</option>
              }
            </select>
          </p>
          <p>
            <label for="run-instrument">Local Instrument</label>
            <select id="run-instrument" [value]="localInstrument()" (change)="localInstrument.set($any($event.target).value)">
              <option value="CORE">CORE</option>
              <option value="B2B">B2B</option>
            </select>
          </p>

          <fieldset>
            <legend>Postenquelle</legend>
            <label>
              <input
                type="radio"
                name="run-source"
                value="open"
                [checked]="source() === 'open'"
                (change)="setSource('open')"
              />
              Aus offenen Posten
            </label>
            <label>
              <input
                type="radio"
                name="run-source"
                value="mandates"
                [checked]="source() === 'mandates'"
                (change)="setSource('mandates')"
              />
              Aus gewählten Mandaten
            </label>
          </fieldset>

          @if (source() === 'open') {
            <p>
              <label for="run-due-by">Fälligkeit bis</label>
              <input
                id="run-due-by"
                type="date"
                [value]="dueBy()"
                (input)="dueBy.set($any($event.target).value)"
              />
            </p>
          } @else {
            <h4 id="run-items-heading">Posten</h4>
            @if (mandatesError(); as e) {
              <p role="alert" class="error">{{ e }}</p>
            }
            @if (mandatesLoading()) {
              <p role="status">Mandate werden geladen.</p>
            } @else if (mandates().length === 0) {
              <p role="status">Es ist kein aktives Mandat für diesen Sequenztyp angelegt.</p>
            } @else {
              <p>
                <label for="item-mandate">Mandat</label>
                <select id="item-mandate" [value]="itemMandate()" (change)="itemMandate.set($any($event.target).value)">
                  <option value=""></option>
                  @for (m of mandates(); track m.Reference) {
                    <option [value]="m.Reference">{{ m.Reference }} — {{ m.DebtorName }}</option>
                  }
                </select>
              </p>
              <p>
                <label for="item-amount">Betrag</label>
                <input
                  id="item-amount"
                  type="text"
                  inputmode="decimal"
                  [value]="itemAmount()"
                  (input)="itemAmount.set($any($event.target).value)"
                />
              </p>
              <p>
                <label for="item-purpose">Verwendungszweck</label>
                <input
                  id="item-purpose"
                  type="text"
                  [value]="itemPurpose()"
                  (input)="itemPurpose.set($any($event.target).value)"
                />
              </p>
              <button type="button" (click)="addItem()">Posten hinzufügen</button>
            }
            @if (itemError(); as e) {
              <p role="alert" class="error">{{ e }}</p>
            }
            @if (items().length > 0) {
              <ul aria-labelledby="run-items-heading">
                @for (it of items(); track it.mandate_reference) {
                  <li [attr.data-testid]="'item-' + it.mandate_reference">
                    {{ it.mandate_reference }}: {{ euro(it.amount_cents) }}
                    <button
                      type="button"
                      [attr.aria-label]="'Posten ' + it.mandate_reference + ' entfernen'"
                      (click)="removeItem(it.mandate_reference)"
                    >
                      Entfernen
                    </button>
                  </li>
                }
              </ul>
            }
          }

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
  /** The sequence types, from the one list the mandate screen uses too (core/direct-debit). */
  readonly sequenceTypes = SEQUENCE_TYPES;
  readonly localInstrument = signal('CORE');

  /** Where the run's items come from: the open receivables, or mandates named by hand. */
  readonly source = signal<'open' | 'mandates'>('open');

  readonly mandates = signal<Mandate[]>([]);
  readonly mandatesLoading = signal(false);
  readonly mandatesError = signal<string | null>(null);
  readonly itemMandate = signal('');
  readonly itemAmount = signal('');
  readonly itemPurpose = signal('');
  readonly itemError = signal<string | null>(null);
  readonly items = signal<CollectionItemRequest[]>([]);

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

  /** Switches the item source; the hand-picked flow loads the mandates it can offer. */
  setSource(source: 'open' | 'mandates'): void {
    this.source.set(source);
    if (source === 'mandates') {
      this.loadMandates();
    }
  }

  /** The run's sequence type decides which mandates are collectable, so both change together. */
  changeSequence(value: string): void {
    this.sequenceType.set(value);
    if (this.source() === 'mandates') {
      this.loadMandates();
    }
  }

  /** Loads the active mandates the run's sequence type can collect under. */
  loadMandates(): void {
    this.mandatesLoading.set(true);
    this.mandatesError.set(null);
    this.api.listMandates('true', this.sequenceType()).subscribe({
      next: (list) => {
        const mandates = list ?? [];
        this.mandates.set(mandates);
        // A mandate this sequence type no longer offers cannot be collected on the run, so a
        // chosen item under one is dropped rather than submitted for the API to refuse, and the
        // picker's stale choice is cleared. Clearing an already-empty choice is a no-op, so the
        // guard is just "not offered", not "not offered and non-empty".
        this.items.set(this.items().filter((it) => mandates.some((m) => m.Reference === it.mandate_reference)));
        if (!mandates.some((m) => m.Reference === this.itemMandate())) {
          this.itemMandate.set('');
        }
        this.mandatesLoading.set(false);
      },
      error: () => {
        this.mandatesError.set('Konnte die Mandate nicht laden.');
        this.mandatesLoading.set(false);
      },
    });
  }

  /** Adds the picked mandate and amount to the run's items. The end-to-end id is the mandate. */
  addItem(): void {
    // The mandate comes from the picker, which offers option values that are exact references, so
    // there is nothing to trim — a trim here would be dead code no test could distinguish.
    const mandate = this.itemMandate();
    if (mandate === '') {
      this.itemError.set('Ein Mandat ist erforderlich.');
      return;
    }
    if (this.items().some((it) => it.mandate_reference === mandate)) {
      this.itemError.set('Dieses Mandat ist bereits gewählt.');
      return;
    }
    const cents = euroToCents(this.itemAmount());
    if (cents === null) {
      this.itemError.set('Ein positiver Betrag ist erforderlich.');
      return;
    }
    this.items.set([
      ...this.items(),
      {
        mandate_reference: mandate,
        end_to_end_id: mandate,
        amount_cents: cents,
        purpose: this.itemPurpose().trim(),
      },
    ]);
    this.itemError.set(null);
    this.itemAmount.set('');
    this.itemPurpose.set('');
  }

  /** Removes a chosen item by its mandate reference. */
  removeItem(mandate: string): void {
    this.items.set(this.items().filter((it) => it.mandate_reference !== mandate));
  }

  /**
   * Submits the run. The source decides how its items are given: hand-picked items are one POST
   * with the items, otherwise the run is created from the open receivables due by the date.
   */
  run(event: Event): void {
    event.preventDefault();
    if (this.source() === 'mandates') {
      this.submitItems();
      return;
    }
    const reference = this.reference().trim();
    if (reference === '') {
      this.actionError.set('Eine Referenz ist erforderlich.');
      return;
    }
    if (this.collectionDate().trim() === '' || this.dueBy().trim() === '') {
      this.actionError.set('Einzugsdatum und Fälligkeit sind erforderlich.');
      return;
    }
    this.resetResult();
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

  /** Creates a run from the hand-picked items. */
  private submitItems(): void {
    const reference = this.reference().trim();
    if (reference === '') {
      this.actionError.set('Eine Referenz ist erforderlich.');
      return;
    }
    if (this.collectionDate().trim() === '') {
      this.actionError.set('Einzugsdatum ist erforderlich.');
      return;
    }
    if (this.items().length === 0) {
      this.actionError.set('Mindestens ein Posten ist erforderlich.');
      return;
    }
    this.resetResult();
    this.api
      .createCollectionRun({
        reference,
        account_id: Number(this.account()),
        creditor_id: this.creditorID().trim(),
        collection_date: this.collectionDate().trim(),
        sequence_type: this.sequenceType(),
        local_instrument: this.localInstrument(),
        items: this.items(),
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

  /** Clears the previous result and marks the screen busy before a create. */
  private resetResult(): void {
    this.submitting.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.created.set(null);
    this.file.set(null);
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
