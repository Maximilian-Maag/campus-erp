import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, Mandate } from '../core/api/generated';
import { SEQUENCE_TYPES, sequenceKey } from '../core/direct-debit';
import { formatDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * Kasse — SEPA-Lastschriftmandate (docs/21).
 *
 * A direct debit may only be collected from a debtor who has authorised it, under a mandate that
 * says since when and under which sequence type. The collection-run screen offers the mandates a
 * run can gather; this is where they come from — registering one used to be an API call.
 *
 * The filters are the server's, exactly like the collection run's: asking for the active mandates
 * of one sequence type is a question the API answers, not something the screen narrows afterwards.
 * The IBAN's checksum is the server's too (ISO 7064, shared with the rest of Kasse): the screen
 * refuses a missing field, and a wrong IBAN comes back as a refusal with the reason.
 */
@Component({
  selector: 'app-mandates',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.kasseMandate.title" subtitleKey="page.kasseMandate.subtitle">
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

        <div class="filters">
          <p>
            <label for="mandate-state">{{ 'kasse.mandates.holding' | transloco }}</label>
            <select id="mandate-state" (change)="changeActive($any($event.target).value)">
              <option value="" [selected]="active() === ''">{{ 'common.all' | transloco }}</option>
              <option value="true" [selected]="active() === 'true'">{{ 'kasse.mandates.onlyActive' | transloco }}</option>
            </select>
          </p>
          <p>
            <label for="mandate-sequence-filter">{{ 'common.sequenceType' | transloco }}</label>
            <select id="mandate-sequence-filter" [value]="sequenceFilter()" (change)="changeSequenceFilter($any($event.target).value)">
              <option value="">{{ 'common.all' | transloco }}</option>
              @for (type of sequenceTypes; track type.value) {
                <option [value]="type.value" [selected]="'' + type.value === sequenceFilter()">{{ type.value }} — {{ type.labelKey | transloco }}</option>
              }
            </select>
          </p>
        </div>

        @if (loading()) {
          <p role="status">{{ 'kasse.mandates.loading' | transloco }}</p>
        } @else if (mandates().length === 0 && !loadError()) {
          <p role="status">{{ 'kasse.mandates.empty' | transloco }}</p>
        } @else if (!loadError()) {
          <table>
            <caption>
              {{ 'kasse.mandates.caption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.reference' | transloco }}</th>
                <th scope="col">{{ 'common.debtor' | transloco }}</th>
                <th scope="col">{{ 'common.iban' | transloco }}</th>
                <th scope="col">{{ 'common.bic' | transloco }}</th>
                <th scope="col">{{ 'kasse.mandates.signed' | transloco }}</th>
                <th scope="col">{{ 'common.sequenceType' | transloco }}</th>
                <th scope="col">{{ 'kasse.mandates.holding' | transloco }}</th>
                <th scope="col">{{ 'common.action' | transloco }}</th>
              </tr>
            </thead>
            <tbody>
              @for (mandate of mandates(); track mandate.ID) {
                <tr [attr.data-testid]="'mandate-' + mandate.Reference">
                  <td>{{ mandate.Reference }}</td>
                  <td>{{ mandate.DebtorName }}</td>
                  <td>{{ mandate.DebtorIBAN }}</td>
                  <td>{{ mandate.DebtorBIC || '—' }}</td>
                  <td>{{ day(mandate.SignatureDate) }}</td>
                  <td [attr.data-testid]="'sequence-' + mandate.Reference">
                    {{ sequence(mandate.SequenceType) }}
                  </td>
                  <td>{{ (mandate.Active ? 'kasse.mandates.active' : 'kasse.mandates.retired') | transloco }}</td>
                  <td>
                    @if (mandate.Active) {
                      @if (mandate.SequenceType === 'RCUR') {
                        @if (confirmingFinal() === mandate.Reference) {
                          <button
                            type="button"
                            [disabled]="finalling() !== null"
                            [attr.aria-label]="
                              'kasse.mandates.confirmFinalLabel' | transloco: { reference: mandate.Reference }
                            "
                            (click)="markFinal(mandate)"
                          >
                            {{ 'kasse.mandates.confirmFinal' | transloco }}
                          </button>
                          <button type="button" (click)="cancelFinal()">
                            {{ 'kasse.mandates.cancel' | transloco }}
                          </button>
                        } @else {
                          <button
                            type="button"
                            [disabled]="finalling() !== null"
                            [attr.aria-label]="
                              'kasse.mandates.finalLabel' | transloco: { reference: mandate.Reference }
                            "
                            (click)="askFinal(mandate)"
                          >
                            {{ 'kasse.mandates.final' | transloco }}
                          </button>
                        }
                      }
                      @if (confirming() === mandate.Reference) {
                        <button
                          type="button"
                          [disabled]="retiring() !== null"
                          [attr.aria-label]="
                            'kasse.mandates.confirmRetireLabel' | transloco: { reference: mandate.Reference }
                          "
                          (click)="retire(mandate)"
                        >
                          {{ 'kasse.mandates.confirmRetire' | transloco }}
                        </button>
                        <button type="button" (click)="cancelRetire()">
                          {{ 'kasse.mandates.cancel' | transloco }}
                        </button>
                      } @else {
                        <button
                          type="button"
                          [disabled]="retiring() !== null"
                          [attr.aria-label]="
                            'kasse.mandates.retireLabel' | transloco: { reference: mandate.Reference }
                          "
                          (click)="askRetire(mandate)"
                        >
                          {{ 'kasse.mandates.retire' | transloco }}
                        </button>
                      }
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
                data-testid="load-more"
                [disabled]="loadingMore()"
                (click)="loadMore()"
              >
                {{ (loadingMore() ? 'kasse.mandates.loadingMore' : 'kasse.mandates.loadMore') | transloco }}
              </button>
            </p>
          }
        }

        <form (submit)="register($event)">
          <h2 id="mandate-register-heading">{{ 'kasse.mandates.registerHeading' | transloco }}</h2>
          <div class="field">
            <label for="mandate-reference">{{ 'common.reference' | transloco }}</label>
            <input
              id="mandate-reference"
              [value]="reference()"
              (input)="reference.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <label for="mandate-debtor">{{ 'common.debtor' | transloco }}</label>
            <input
              id="mandate-debtor"
              [value]="debtorName()"
              (input)="debtorName.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <label for="mandate-iban">{{ 'common.iban' | transloco }}</label>
            <input id="mandate-iban" [value]="debtorIBAN()" (input)="debtorIBAN.set($any($event.target).value)" />
          </div>
          <div class="field">
            <label for="mandate-bic">{{ 'kasse.mandates.bicOptional' | transloco }}</label>
            <input id="mandate-bic" [value]="debtorBIC()" (input)="debtorBIC.set($any($event.target).value)" />
          </div>
          <div class="field">
            <label for="mandate-signature">{{ 'kasse.mandates.signatureDate' | transloco }}</label>
            <input
              id="mandate-signature"
              type="date"
              [value]="signatureDate()"
              (input)="signatureDate.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <label for="mandate-sequence">{{ 'common.sequenceType' | transloco }}</label>
            <select id="mandate-sequence" (change)="sequenceType.set($any($event.target).value)">
              @for (type of sequenceTypes; track type.value) {
                <option [value]="type.value" [selected]="type.value === sequenceType()">{{ type.value }} — {{ type.labelKey | transloco }}</option>
              }
            </select>
          </div>
          <button type="submit" [disabled]="busy()">{{ 'kasse.mandates.register' | transloco }}</button>
        </form>
      </section>
    </app-page>
  `,
})
export class MandatesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly mandates = signal<Mandate[]>([]);
  /**
   * One page of mandates, sent explicitly rather than left to the server's default: a screen whose
   * page size changed when the server did would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  /**
   * How many rows the server has handed over so far — the offset the next page starts at.
   *
   * It is not the length of `mandates`: a mandate registered from this screen is appended to the
   * list so the operator sees it, but it is not a row the server has counted for paging. Asking
   * for `offset = mandates().length` after that registration starts one row too late and the
   * server row at the boundary is never fetched (#182).
   */
  private readonly serverRows = signal(0);
  readonly loadingMore = signal(false);
  readonly loading = signal(false);
  readonly busy = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);

  /** The filters, as the API takes them: "" means "no narrowing". */
  readonly active = signal('true');
  readonly sequenceFilter = signal('');

  /** The mandate a revocation is being confirmed for, and the one whose revocation is in flight. */
  readonly confirming = signal<string | null>(null);
  readonly retiring = signal<string | null>(null);

  /** The mandate whose last collection is being confirmed, and the one whose request is in flight. */
  readonly confirmingFinal = signal<string | null>(null);
  readonly finalling = signal<string | null>(null);

  readonly reference = signal('');
  readonly debtorName = signal('');
  readonly debtorIBAN = signal('');
  readonly debtorBIC = signal('');
  readonly signatureDate = signal('');
  readonly sequenceType = signal('RCUR');

  readonly sequenceTypes = SEQUENCE_TYPES;

  ngOnInit(): void {
    this.load();
  }

  /**
   * A revocation is effectively final, so it is asked for twice: the row turns into the question
   * and the answer names the mandate again. The mandate that is being revoked is not the one being
   * confirmed, so two rows cannot be in the same state by accident.
   */
  askRetire(mandate: Mandate): void {
    this.actionError.set(null);
    this.notice.set(null);
    this.confirming.set(mandate.Reference);
  }

  cancelRetire(): void {
    this.confirming.set(null);
  }

  /**
   * The last collection is announced once and cannot be taken back, so it is asked for twice, like a
   * revocation. Only a recurring mandate offers it: a first collection has not happened yet, and a
   * one-off is already single.
   */
  askFinal(mandate: Mandate): void {
    this.actionError.set(null);
    this.notice.set(null);
    this.confirmingFinal.set(mandate.Reference);
  }

  cancelFinal(): void {
    this.confirmingFinal.set(null);
  }

  markFinal(mandate: Mandate): void {
    this.finalling.set(mandate.Reference);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.markMandateFinal(mandate.Reference).subscribe({
      next: (final) => {
        // The row now says FNAL: the next run collects this mandate for the last time, and the
        // screen that asked for it says so without a reload.
        this.mandates.update((list) =>
          list.map((m) => (m.Reference === final.Reference ? final : m))
        );
        this.confirmingFinal.set(null);
        this.finalling.set(null);
        this.notice.set(this.i18n.translate('kasse.mandates.finalNotice', { reference: final.Reference }));
      },
      error: () => {
        this.finalling.set(null);
        this.actionError.set(
          this.i18n.translate('kasse.mandates.finalFailed', { reference: mandate.Reference })
        );
      },
    });
  }

  retire(mandate: Mandate): void {
    this.retiring.set(mandate.Reference);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.retireMandate(mandate.Reference).subscribe({
      next: (retired) => {
        // The row stays where it is and now says it is retired: the mandate is soft state, so a
        // revocation is visible on the screen that caused it rather than only after a reload.
        this.mandates.update((list) =>
          list.map((m) => (m.Reference === retired.Reference ? retired : m))
        );
        this.confirming.set(null);
        this.retiring.set(null);
        this.notice.set(this.i18n.translate('kasse.mandates.retiredNotice', { reference: retired.Reference }));
      },
      error: () => {
        this.retiring.set(null);
        this.actionError.set(
          this.i18n.translate('kasse.mandates.retireFailed', { reference: mandate.Reference })
        );
      },
    });
  }

  /** Reads the mandates under the current filters — the server narrows, the screen does not. */
  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listMandates(this.active(), this.sequenceFilter(), String(this.pageSize), '0').subscribe({
      next: (mandates) => {
        const page = mandates ?? [];
        this.mandates.set(page);
        this.serverRows.set(page.length);
        // A full page means there may be more; a short one is the end of the list.
        this.hasMore.set(page.length === this.pageSize);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('kasse.mandates.loadFailed'));
        this.hasMore.set(false);
        this.serverRows.set(0);
        this.loading.set(false);
      },
    });
  }

  /**
   * Reads the next page and appends it. The offset is the number of rows already on screen, which is
   * why the list is never re-sorted in the browser: the pages only line up while the order the server
   * answered with is the order they are shown in.
   *
   * A failed page is reported without clearing what is already there — the mandates that were read
   * are still valid, and blanking the screen would suggest they are not.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api
      .listMandates(this.active(), this.sequenceFilter(), String(this.pageSize), String(this.serverRows()))
      .subscribe({
        next: (mandates) => {
          const page = mandates ?? [];
          this.mandates.set([...this.mandates(), ...page]);
          this.serverRows.update((n: number) => n + page.length);
          this.hasMore.set(page.length === this.pageSize);
          this.loadingMore.set(false);
        },
        error: () => {
          this.loadingMore.set(false);
          this.actionError.set(this.i18n.translate('kasse.mandates.loadMoreFailed'));
        },
      });
  }

  changeActive(value: string): void {
    this.active.set(value);
    this.load();
  }

  changeSequenceFilter(value: string): void {
    this.sequenceFilter.set(value);
    this.load();
  }

  /**
   * Registers a mandate. The screen refuses what it can see is missing — a mandate without a
   * reference, a debtor, an IBAN or a signature date authorises nothing — and leaves the checks it
   * cannot make (the IBAN's checksum, the BIC's shape) to the server, whose refusal is shown as it
   * comes back rather than replaced by a second implementation here.
   */
  register(event: Event): void {
    event.preventDefault();
    this.actionError.set(null);
    this.notice.set(null);

    if (
      this.reference().trim() === '' ||
      this.debtorName().trim() === '' ||
      this.debtorIBAN().trim() === '' ||
      this.signatureDate().trim() === ''
    ) {
      this.actionError.set(this.i18n.translate('kasse.mandates.required'));
      return;
    }
    this.busy.set(true);
    this.api
      .createMandate({
        reference: this.reference(),
        debtor_name: this.debtorName(),
        debtor_iban: this.debtorIBAN(),
        debtor_bic: this.debtorBIC(),
        signature_date: this.signatureDate(),
        sequence_type: this.sequenceType(),
      })
      .subscribe({
        next: (saved) => {
          // Appended, not re-sorted: the pages only line up while the screen shows the server's
          // order, and re-sorting would also change what `offset` means for the next page (#182).
          this.mandates.update((list) => [...list, saved]);
          this.reference.set('');
          this.debtorName.set('');
          this.debtorIBAN.set('');
          this.debtorBIC.set('');
          this.signatureDate.set('');
          this.notice.set(this.i18n.translate('kasse.mandates.registered', { reference: saved.Reference }));
          this.busy.set(false);
        },
        error: () => {
          this.actionError.set(this.i18n.translate('kasse.mandates.registerFailed'));
          this.busy.set(false);
        },
      });
  }

  /** The word for a sequence type, from the list the collection run reads too. */
  sequence(value: string): string {
    return this.i18n.translate(sequenceKey(value));
  }

  /** Dates come from the shared formatters, not from each screen's own. */
  readonly day = formatDay;
}
