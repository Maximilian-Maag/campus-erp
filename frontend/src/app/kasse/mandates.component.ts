import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, Mandate } from '../core/api/generated';
import { SEQUENCE_TYPES, sequenceLabel } from '../core/direct-debit';
import { formatDay } from '../core/format';

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
  template: `
    <section aria-labelledby="mandates-heading">
      <h2 id="mandates-heading">SEPA-Lastschriftmandate</h2>

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
          <label for="mandate-state">Bestand</label>
          <select id="mandate-state" [value]="active()" (change)="changeActive($any($event.target).value)">
            <option value="">alle</option>
            <option value="true">nur aktive</option>
          </select>
        </p>
        <p>
          <label for="mandate-sequence-filter">Sequenztyp</label>
          <select id="mandate-sequence-filter" [value]="sequenceFilter()" (change)="changeSequenceFilter($any($event.target).value)">
            <option value="">alle</option>
            @for (type of sequenceTypes; track type.value) {
              <option [value]="type.value">{{ type.value }} — {{ type.label }}</option>
            }
          </select>
        </p>
      </div>

      @if (loading()) {
        <p role="status">Mandate werden geladen.</p>
      } @else if (mandates().length === 0 && !loadError()) {
        <p role="status">Es sind keine Mandate erfasst.</p>
      } @else if (!loadError()) {
        <table>
          <caption>
            Lastschriftmandate mit Schuldner, Unterschriftsdatum und Sequenztyp
          </caption>
          <thead>
            <tr>
              <th scope="col">Referenz</th>
              <th scope="col">Schuldner</th>
              <th scope="col">IBAN</th>
              <th scope="col">BIC</th>
              <th scope="col">Unterschrieben</th>
              <th scope="col">Sequenztyp</th>
              <th scope="col">Bestand</th>
              <th scope="col">Aktion</th>
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
                <td>{{ mandate.Active ? 'aktiv' : 'zurückgezogen' }}</td>
                <td>
                  @if (mandate.Active) {
                    @if (confirming() === mandate.Reference) {
                      <button
                        type="button"
                        [disabled]="retiring() !== null"
                        [attr.aria-label]="'Mandat ' + mandate.Reference + ' wirklich zurückziehen'"
                        (click)="retire(mandate)"
                      >
                        Ja, zurückziehen
                      </button>
                      <button type="button" (click)="cancelRetire()">Abbrechen</button>
                    } @else {
                      <button
                        type="button"
                        [disabled]="retiring() !== null"
                        [attr.aria-label]="'Mandat ' + mandate.Reference + ' zurückziehen'"
                        (click)="askRetire(mandate)"
                      >
                        Zurückziehen
                      </button>
                    }
                  }
                </td>
              </tr>
            }
          </tbody>
        </table>
      }

      <form (submit)="register($event)">
        <h3 id="mandate-register-heading">Mandat aufnehmen</h3>
        <p>
          <label for="mandate-reference">Referenz</label>
          <input
            id="mandate-reference"
            [value]="reference()"
            (input)="reference.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="mandate-debtor">Schuldner</label>
          <input
            id="mandate-debtor"
            [value]="debtorName()"
            (input)="debtorName.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="mandate-iban">IBAN</label>
          <input id="mandate-iban" [value]="debtorIBAN()" (input)="debtorIBAN.set($any($event.target).value)" />
        </p>
        <p>
          <label for="mandate-bic">BIC (optional)</label>
          <input id="mandate-bic" [value]="debtorBIC()" (input)="debtorBIC.set($any($event.target).value)" />
        </p>
        <p>
          <label for="mandate-signature">Unterschriftsdatum</label>
          <input
            id="mandate-signature"
            type="date"
            [value]="signatureDate()"
            (input)="signatureDate.set($any($event.target).value)"
          />
        </p>
        <p>
          <label for="mandate-sequence">Sequenztyp</label>
          <select id="mandate-sequence" [value]="sequenceType()" (change)="sequenceType.set($any($event.target).value)">
            @for (type of sequenceTypes; track type.value) {
              <option [value]="type.value">{{ type.value }} — {{ type.label }}</option>
            }
          </select>
        </p>
        <button type="submit" [disabled]="busy()">Mandat aufnehmen</button>
      </form>
    </section>
  `,
})
export class MandatesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly mandates = signal<Mandate[]>([]);
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
        this.notice.set(`Mandat ${retired.Reference} zurückgezogen.`);
      },
      error: () => {
        this.retiring.set(null);
        this.actionError.set(`Mandat ${mandate.Reference} konnte nicht zurückgezogen werden.`);
      },
    });
  }

  /** Reads the mandates under the current filters — the server narrows, the screen does not. */
  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listMandates(this.active(), this.sequenceFilter()).subscribe({
      next: (mandates) => {
        this.mandates.set(mandates ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set('Die Mandate konnten nicht geladen werden.');
        this.loading.set(false);
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
      this.actionError.set('Referenz, Schuldner, IBAN und Unterschriftsdatum sind erforderlich.');
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
          this.mandates.update((list) => [...list, saved].sort((a, b) => a.Reference.localeCompare(b.Reference)));
          this.reference.set('');
          this.debtorName.set('');
          this.debtorIBAN.set('');
          this.debtorBIC.set('');
          this.signatureDate.set('');
          this.notice.set(`Mandat ${saved.Reference} aufgenommen.`);
          this.busy.set(false);
        },
        error: () => {
          this.actionError.set('Das Mandat konnte nicht aufgenommen werden.');
          this.busy.set(false);
        },
      });
  }

  /** The German word for a sequence type, from the list the collection run reads too. */
  sequence(value: string): string {
    return sequenceLabel(value);
  }

  /** Dates come from the shared formatters, not from each screen's own. */
  readonly day = formatDay;
}
