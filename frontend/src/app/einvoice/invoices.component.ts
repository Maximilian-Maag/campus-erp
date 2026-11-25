import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, InvoiceSummary } from '../core/api/generated';
import { formatCents, formatDay } from '../core/format';
import { PageComponent } from '../ui/page.component';

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
  imports: [FormsModule, PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.erechnungRechnungen.title" subtitleKey="page.erechnungRechnungen.subtitle">
      <section>
        <div class="import">
          <label for="xrechnung">{{ 'erechnung.invoices.xml' | transloco }}</label>
          <textarea
            id="xrechnung"
            name="xrechnung"
            rows="6"
            [ngModel]="xml()"
            (ngModelChange)="xml.set($event)"
          ></textarea>
          <button type="button" [disabled]="importing()" (click)="importInvoice()">
            {{ 'erechnung.invoices.importButton' | transloco }}
          </button>
        </div>

        @if (loadError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }
        @if (actionError(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }

        @if (loading()) {
          <p role="status">{{ 'erechnung.invoices.loading' | transloco }}</p>
        } @else if (invoices().length === 0 && !loadError()) {
          <p role="status">{{ 'erechnung.invoices.empty' | transloco }}</p>
        } @else if (!loadError()) {
          <table>
            <caption>
              {{ 'erechnung.invoices.caption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.number' | transloco }}</th>
                <th scope="col">{{ 'erechnung.invoices.direction' | transloco }}</th>
                <th scope="col">{{ 'common.date' | transloco }}</th>
                <th scope="col">{{ 'common.reference' | transloco }}</th>
                <th scope="col" class="amount">{{ 'common.amount' | transloco }}</th>
                <th scope="col">{{ 'erechnung.invoices.allocation' | transloco }}</th>
                <th scope="col">{{ 'erechnung.invoices.documents' | transloco }}</th>
                <th scope="col">{{ 'common.action' | transloco }}</th>
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
                      {{
                        (invoice.MatchedOrderNumber
                          ? 'erechnung.invoices.matchedOrder'
                          : 'erechnung.invoices.unmatched'
                        ) | transloco: { number: invoice.MatchedOrderNumber }
                      }}
                    }
                  </td>
                  <td [attr.data-testid]="'documents-' + invoice.Number">
                    <a [href]="documentUrl(invoice.ID, 'xrechnung')">{{ 'erechnung.invoices.ubl' | transloco }}</a>
                    <a [href]="documentUrl(invoice.ID, 'cii')">{{ 'erechnung.invoices.cii' | transloco }}</a>
                  </td>
                  <td>
                    @if (invoice.Direction === 'inbound' && !invoice.MatchedOrderID) {
                      <button
                        type="button"
                        [disabled]="matching() !== null"
                        [attr.aria-label]="'erechnung.invoices.matchLabel' | transloco: { number: invoice.Number }"
                        (click)="match(invoice)"
                      >
                        {{ 'erechnung.invoices.match' | transloco }}
                      </button>
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
                {{ (loadingMore() ? 'erechnung.invoices.loadingMore' : 'erechnung.invoices.loadMore') | transloco }}
              </button>
            </p>
          }
        }
      </section>
    </app-page>
  `,
})
export class EInvoicesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly invoices = signal<InvoiceSummary[]>([]);
  /**
   * One page of invoices, sent explicitly rather than left to the server's default: a screen whose
   * page size changed when the server did would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
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
    this.api.listEInvoices(String(this.pageSize), '0').subscribe({
      next: (invoices) => {
        const page = invoices ?? [];
        this.invoices.set(page);
        // A full page means there may be more; a short one is the end of the list.
        this.hasMore.set(page.length === this.pageSize);
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('erechnung.invoices.loadFailed'));
        this.hasMore.set(false);
        this.loading.set(false);
      },
    });
  }

  /**
   * Reads the next page and appends it. The offset is the number of rows already on screen, which is
   * why the list is never re-sorted in the browser: the pages only line up while the order the server
   * answered with is the order they are shown in.
   *
   * A failed page is reported without clearing what is already there — the invoices that were read
   * are still valid, and blanking the screen would suggest they are not.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api.listEInvoices(String(this.pageSize), String(this.invoices().length)).subscribe({
      next: (invoices) => {
        const page = invoices ?? [];
        this.invoices.set([...this.invoices(), ...page]);
        this.hasMore.set(page.length === this.pageSize);
        this.loadingMore.set(false);
      },
      error: () => {
        this.loadingMore.set(false);
        this.actionError.set(this.i18n.translate('erechnung.invoices.loadMoreFailed'));
      },
    });
  }

  importInvoice(): void {
    if (this.xml().trim() === '') {
      this.actionError.set(this.i18n.translate('erechnung.invoices.noXml'));
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
        this.actionError.set(this.i18n.translate('erechnung.invoices.importFailed'));
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
        this.actionError.set(
          this.i18n.translate('erechnung.invoices.matchFailed', { number: invoice.Number }),
        );
      },
    });
  }

  /**
   * Where an invoice's document is served: one model, two syntaxes — UBL 2.1 as XRechnung, and
   * ZUGFeRD/Factur-X as CII (docs/09).
   */
  documentUrl(id: number, syntax: 'xrechnung' | 'cii'): string {
    return `/api/e-invoices/${id}/${syntax}`;
  }

  /** Which way the invoice goes, in the words the screen uses. */
  direction(direction: string): string {
    switch (direction) {
      case 'inbound':
        return this.i18n.translate('erechnung.invoices.inbound');
      case 'outbound':
        return this.i18n.translate('erechnung.invoices.outbound');
      default:
        return direction;
    }
  }

  /** Dates and amounts come from the shared formatters, not from each screen's own. */
  readonly day = formatDay;
  readonly euro = formatCents;
}
