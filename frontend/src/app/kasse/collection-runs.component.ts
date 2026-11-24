import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, CashAccount, CollectionItemRequest, CollectionRun, Mandate } from '../core/api/generated';
import { RUN_SEQUENCE_TYPES, runSequenceKey } from '../core/direct-debit';
import { euroToCents, formatCents, formatDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

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
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.kasseEinzuege.title" subtitleKey="page.kasseEinzuege.subtitle">
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
          <p role="status">{{ 'kasse.collections.accountsLoading' | transloco }}</p>
        } @else if (accounts().length === 0 && !loadError()) {
          <p role="status">{{ 'kasse.collections.noAccount' | transloco }}</p>
        } @else if (!loadError()) {
          <form (submit)="run($event)">
            <h2 id="collection-run-new-heading">{{ 'kasse.collections.createHeading' | transloco }}</h2>
            <div class="field">
              <label for="run-reference">{{ 'common.reference' | transloco }}</label>
              <input
                id="run-reference"
                type="text"
                [value]="reference()"
                (input)="reference.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="run-account">{{ 'common.account' | transloco }}</label>
              <select id="run-account" [value]="account()" (change)="account.set($any($event.target).value)">
                @for (a of accounts(); track a.ID) {
                  <option [value]="a.ID" [selected]="'' + a.ID === account()">{{ a.Name }} ({{ a.IBAN }})</option>
                }
              </select>
            </div>
            <div class="field">
              <label for="run-creditor">{{ 'kasse.collections.creditorId' | transloco }}</label>
              <input
                id="run-creditor"
                type="text"
                [value]="creditorID()"
                (input)="creditorID.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="run-collection-date">{{ 'kasse.collections.collectionDate' | transloco }}</label>
              <input
                id="run-collection-date"
                type="date"
                [value]="collectionDate()"
                (input)="collectionDate.set($any($event.target).value)"
              />
            </div>
            <div class="field">
              <label for="run-sequence">{{ 'common.sequenceType' | transloco }}</label>
              <!-- The selection is carried by the options, not by a value binding on the select:
                   the options are created by @for after the select's own binding runs, so a select
                   starting on anything but its first option would show the first one while the
                   signal, and the POST body, carried the default. -->
              <select id="run-sequence" (change)="changeSequence($any($event.target).value)">
                @for (type of sequenceTypes; track type.value) {
                  <option [value]="type.value" [selected]="type.value === sequenceType()">
                    {{ type.value ? type.value + ' — ' + (type.labelKey | transloco) : (type.labelKey | transloco) }}
                  </option>
                }
              </select>
            </div>
            <div class="field">
              <label for="run-instrument">{{ 'kasse.collections.localInstrument' | transloco }}</label>
              <select id="run-instrument" [value]="localInstrument()" (change)="localInstrument.set($any($event.target).value)">
                <option value="CORE">CORE</option>
                <option value="B2B">B2B</option>
              </select>
            </div>

            <fieldset>
              <legend>{{ 'kasse.collections.sourceLegend' | transloco }}</legend>
              <label>
                <input
                  type="radio"
                  name="run-source"
                  value="open"
                  [checked]="source() === 'open'"
                  (change)="setSource('open')"
                />
                {{ 'kasse.collections.fromOpenItems' | transloco }}
              </label>
              <label>
                <input
                  type="radio"
                  name="run-source"
                  value="mandates"
                  [checked]="source() === 'mandates'"
                  (change)="setSource('mandates')"
                />
                {{ 'kasse.collections.fromMandates' | transloco }}
              </label>
            </fieldset>

            @if (source() === 'open') {
              <div class="field">
                <label for="run-due-by">{{ 'kasse.collections.dueBy' | transloco }}</label>
                <input
                  id="run-due-by"
                  type="date"
                  [value]="dueBy()"
                  (input)="dueBy.set($any($event.target).value)"
                />
              </div>
            } @else {
              <h3 id="run-items-heading">{{ 'kasse.collections.itemsHeading' | transloco }}</h3>
              @if (mandatesError(); as e) {
                <p role="alert" class="error">{{ e }}</p>
              }
              @if (mandatesLoading()) {
                <p role="status">{{ 'kasse.collections.mandatesLoading' | transloco }}</p>
              } @else if (mandates().length === 0) {
                @if (sequenceType() === '') {
                  <p role="status">{{ 'kasse.collections.noMandates' | transloco }}</p>
                } @else {
                  <p role="status">{{ 'kasse.collections.noMandatesForType' | transloco }}</p>
                }
              } @else {
                <div class="field">
                  <label for="item-mandate">{{ 'common.mandate' | transloco }}</label>
                  <select id="item-mandate" [value]="itemMandate()" (change)="itemMandate.set($any($event.target).value)">
                    <option value=""></option>
                    @for (m of mandates(); track m.Reference) {
                      <option [value]="m.Reference" [selected]="'' + m.Reference === itemMandate()">{{ m.Reference }} — {{ m.DebtorName }}</option>
                    }
                  </select>
                </div>
                @if (mandatesHasMore()) {
                  <p>
                    <button
                      type="button"
                      data-testid="load-more-mandates"
                      [disabled]="mandatesLoadingMore()"
                      (click)="loadMoreMandates()"
                    >
                      {{
                        (mandatesLoadingMore()
                          ? 'kasse.collections.loadingMoreMandates'
                          : 'kasse.collections.loadMoreMandates'
                        ) | transloco
                      }}
                    </button>
                  </p>
                }
                <div class="field">
                  <label for="item-amount">{{ 'common.amount' | transloco }}</label>
                  <input
                    id="item-amount"
                    type="text"
                    inputmode="decimal"
                    [value]="itemAmount()"
                    (input)="itemAmount.set($any($event.target).value)"
                  />
                </div>
                <div class="field">
                  <label for="item-purpose">{{ 'common.purpose' | transloco }}</label>
                  <input
                    id="item-purpose"
                    type="text"
                    [value]="itemPurpose()"
                    (input)="itemPurpose.set($any($event.target).value)"
                  />
                </div>
                <button type="button" (click)="addItem()">{{ 'kasse.collections.addItem' | transloco }}</button>
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
                        [attr.aria-label]="
                          'kasse.collections.removeItem' | transloco: { reference: it.mandate_reference }
                        "
                        (click)="removeItem(it.mandate_reference)"
                      >
                        {{ 'kasse.collections.remove' | transloco }}
                      </button>
                    </li>
                  }
                </ul>
              }
            }

            <button type="submit" [disabled]="submitting()">{{ 'kasse.collections.create' | transloco }}</button>
          </form>

          @if (created(); as r) {
            <section aria-labelledby="collection-run-result-heading">
              <h2 id="collection-run-result-heading">{{ 'kasse.collections.createdHeading' | transloco }}</h2>
              <p role="status" [attr.data-testid]="'run-' + r.Reference">
                {{
                  'kasse.collections.result'
                    | transloco
                      : {
                          reference: r.Reference,
                          count: r.ItemCount,
                          sum: euro(r.SumCents),
                          date: day(r.CollectionDate),
                          type: runLabel(r.SequenceType),
                        }
                }}
              </p>
              <button type="button" (click)="fetchFile()">{{ 'kasse.collections.showFile' | transloco }}</button>
              @if (file(); as f) {
                <pre [attr.data-testid]="'run-file-' + r.Reference">{{ f }}</pre>
              }
            </section>
          }
        }
      </section>
    </app-page>
  `,
})
export class CollectionRunsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

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
  /**
   * The types a run may be filed under: the four SEPA types plus "je Mandat", where the run takes each
   * mandate's own type and the file carries one payment instruction per type (core/direct-debit).
   */
  readonly sequenceTypes = RUN_SEQUENCE_TYPES;
  readonly localInstrument = signal('CORE');

  /** Where the run's items come from: the open receivables, or mandates named by hand. */
  readonly source = signal<'open' | 'mandates'>('open');

  readonly mandates = signal<Mandate[]>([]);
  readonly mandatesLoading = signal(false);
  readonly mandatesError = signal<string | null>(null);
  /**
   * One page of mandates, sent explicitly rather than left to the server's default: a picker whose
   * page size changed when the server did would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page of mandates came back full, so there may be another. */
  readonly mandatesHasMore = signal(false);
  readonly mandatesLoadingMore = signal(false);
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
        this.loadError.set(this.i18n.translate('kasse.collections.accountsLoadFailed'));
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

  /**
   * The run's sequence type decides which mandates are collectable, so both change together.
   *
   * Every hand-picked item was chosen from the mandates *the previous sequence type* collects under,
   * so none of them can stay on a run that collects under another one. That follows from how they
   * were picked, which is why it does not need a lookup — and why it cannot be read off the reloaded
   * page either: a page is not the list, and an item picked from the second page would look stale on
   * the first. The items are dropped, and the screen says so rather than letting them vanish.
   */
  changeSequence(value: string): void {
    this.sequenceType.set(value);
    if (this.source() === 'mandates') {
      if (this.items().length > 0) {
        this.items.set([]);
        this.notice.set(this.i18n.translate('kasse.collections.sequenceChanged'));
      }
      this.loadMandates();
    }
  }

  /**
   * Loads the first page of the active mandates the run's sequence type can collect under.
   *
   * The pick is cleared: it is a choice among the options on offer, and a fresh read offers a new
   * first page — the mandate that was picked may sit further down the list now, and a select whose
   * value is not among its options would show nothing while the screen still thinks it holds one.
   */
  loadMandates(): void {
    this.mandatesLoading.set(true);
    this.mandatesError.set(null);
    this.itemMandate.set('');
    this.api.listMandates('true', this.sequenceType(), String(this.pageSize), '0').subscribe({
      next: (list) => {
        const page = list ?? [];
        this.mandates.set(page);
        this.mandatesHasMore.set(page.length === this.pageSize);
        this.mandatesLoading.set(false);
      },
      error: () => {
        this.mandatesError.set(this.i18n.translate('kasse.collections.mandatesLoadFailed'));
        this.mandatesHasMore.set(false);
        this.mandatesLoading.set(false);
      },
    });
  }

  /**
   * Reads the next page of collectable mandates and appends it. A failed page leaves what is already
   * offered where it is — those mandates are still collectable, and blanking the picker would suggest
   * they are not.
   */
  loadMoreMandates(): void {
    if (!this.mandatesHasMore() || this.mandatesLoadingMore()) {
      return;
    }
    this.mandatesLoadingMore.set(true);
    this.mandatesError.set(null);
    this.api
      .listMandates('true', this.sequenceType(), String(this.pageSize), String(this.mandates().length))
      .subscribe({
        next: (list) => {
          const page = list ?? [];
          this.mandates.set([...this.mandates(), ...page]);
          this.mandatesHasMore.set(page.length === this.pageSize);
          this.mandatesLoadingMore.set(false);
        },
        error: () => {
          this.mandatesLoadingMore.set(false);
          this.mandatesError.set(this.i18n.translate('kasse.collections.mandatesLoadMoreFailed'));
        },
      });
  }

  /** Adds the picked mandate and amount to the run's items. The end-to-end id is the mandate. */
  addItem(): void {
    // The mandate comes from the picker, which offers option values that are exact references, so
    // there is nothing to trim — a trim here would be dead code no test could distinguish.
    const mandate = this.itemMandate();
    if (mandate === '') {
      this.itemError.set(this.i18n.translate('kasse.collections.mandateRequired'));
      return;
    }
    if (this.items().some((it) => it.mandate_reference === mandate)) {
      this.itemError.set(this.i18n.translate('kasse.collections.mandateDuplicate'));
      return;
    }
    const cents = euroToCents(this.itemAmount());
    if (cents === null) {
      this.itemError.set(this.i18n.translate('kasse.collections.amountRequired'));
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
      this.actionError.set(this.i18n.translate('kasse.collections.referenceRequired'));
      return;
    }
    if (this.collectionDate().trim() === '' || this.dueBy().trim() === '') {
      this.actionError.set(this.i18n.translate('kasse.collections.datesRequired'));
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
          this.notice.set(this.i18n.translate('kasse.collections.created'));
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set(this.i18n.translate('kasse.collections.createFailed'));
        },
      });
  }

  /** Creates a run from the hand-picked items. */
  private submitItems(): void {
    const reference = this.reference().trim();
    if (reference === '') {
      this.actionError.set(this.i18n.translate('kasse.collections.referenceRequired'));
      return;
    }
    if (this.collectionDate().trim() === '') {
      this.actionError.set(this.i18n.translate('kasse.collections.collectionDateRequired'));
      return;
    }
    if (this.items().length === 0) {
      this.actionError.set(this.i18n.translate('kasse.collections.itemsRequired'));
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
          this.notice.set(this.i18n.translate('kasse.collections.created'));
        },
        error: () => {
          this.submitting.set(false);
          this.actionError.set(this.i18n.translate('kasse.collections.createFailed'));
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
      error: () => this.actionError.set(this.i18n.translate('kasse.collections.fileLoadFailed')),
    });
  }

  /** Amounts and dates are rendered by the shared formatters, not by each screen's own. */
  readonly euro = formatCents;
  readonly day = formatDay;
  /** The word for the type a run collected under — "gemischt" when it has no single one. */
  /** The word for the type a run collected under, in the active language. */
  runLabel(value: string): string {
    return this.i18n.translate(runSequenceKey(value));
  }
}
