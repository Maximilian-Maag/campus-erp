import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { BudgetPositionResponse, CampusErpApi } from '../core/api/generated';
import { formatCents } from '../core/format';
import { PageComponent } from '../ui/page.component';

/**
 * Budget — Mittelübersicht.
 *
 * Every position the budget holds, with what it was given and what is bound or spent against
 * it. Read-only: the screen shows the budget, it does not move it. The available amount comes
 * from the API rather than being recomputed here, so the figure on screen is the one the
 * Mittelüberwachung actually applies.
 *
 * The heading comes from `app-page`, so the screen names itself in the same words the navigation
 * uses for it; the column headings are the shared vocabulary, so "Ansatz" means the same figure
 * here as on every other Haushalt screen.
 */
@Component({
  selector: 'app-budget-positions',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.haushaltStellen.title" subtitleKey="page.haushaltStellen.subtitle">
      <section>
        @if (error(); as e) {
          <p role="alert" class="error">{{ e }}</p>
        }

        @if (loading()) {
          <p role="status">{{ 'haushalt.positions.loading' | transloco }}</p>
        } @else if (positions().length === 0 && !error()) {
          <p role="status">{{ 'haushalt.positions.empty' | transloco }}</p>
        } @else if (!error()) {
          <table>
            <caption>
              {{ 'haushalt.positions.caption' | transloco }}
            </caption>
            <thead>
              <tr>
                <th scope="col">{{ 'common.year' | transloco }}</th>
                <th scope="col">{{ 'common.chapter' | transloco }}</th>
                <th scope="col">{{ 'common.title' | transloco }}</th>
                <th scope="col">{{ 'common.description' | transloco }}</th>
                <th scope="col" class="amount">{{ 'haushalt.common.allocated' | transloco }}</th>
                <th scope="col" class="amount">{{ 'haushalt.common.committed' | transloco }}</th>
                <th scope="col" class="amount">{{ 'haushalt.common.actual' | transloco }}</th>
                <th scope="col" class="amount">{{ 'haushalt.common.available' | transloco }}</th>
              </tr>
            </thead>
            <tbody>
              @for (row of positions(); track row.position.ID) {
                <tr [attr.data-testid]="'position-' + row.position.ID">
                  <td>{{ row.position.Year }}</td>
                  <td>{{ row.position.Chapter }}</td>
                  <td>{{ row.position.Title }}</td>
                  <td>{{ row.position.Description }}</td>
                  <td class="amount">{{ euro(row.balance.AllocatedCents) }}</td>
                  <td class="amount">{{ euro(row.balance.CommittedCents) }}</td>
                  <td class="amount">{{ euro(row.balance.ActualCents) }}</td>
                  <td class="amount" [attr.data-testid]="'available-' + row.position.ID">
                    {{ euro(row.available_cents) }}
                  </td>
                </tr>
              }
            </tbody>
          </table>
          @if (moreError(); as e) {
            <p role="alert" class="error">{{ e }}</p>
          }
          @if (hasMore()) {
            <p>
              <button
                type="button"
                data-testid="load-more"
                [disabled]="loadingMore()"
                (click)="loadMore()"
              >
                {{
                  (loadingMore() ? 'haushalt.positions.loadingMore' : 'haushalt.positions.loadMore')
                    | transloco
                }}
              </button>
            </p>
          }
        }
      </section>
    </app-page>
  `,
})
export class BudgetPositionsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly positions = signal<BudgetPositionResponse[]>([]);
  readonly loading = signal(true);
  readonly error = signal<string | null>(null);
  /**
   * One page of positions, sent explicitly rather than left to the server's default: a screen whose
   * page size changed when the server did would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
  /** A page that failed, kept apart from a failed first read so the rows already shown stay. */
  readonly moreError = signal<string | null>(null);

  // Loading starts when the screen is shown, not when it is constructed: the initial state is
  // then observable (and testable) before any request is made.
  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.error.set(null);
    this.api.listBudgetPositions(String(this.pageSize), '0').subscribe({
      next: (rows) => {
        const page = rows ?? [];
        this.positions.set(page);
        // A full page means there may be more; a short one is the end of the budget.
        this.hasMore.set(page.length === this.pageSize);
        this.loading.set(false);
      },
      error: () => {
        this.error.set(this.i18n.translate('haushalt.positions.loadFailed'));
        this.hasMore.set(false);
        this.loading.set(false);
      },
    });
  }

  /**
   * Reads the next page and appends it. The offset is the number of rows already on screen, which is
   * why the list is never re-sorted in the browser: the pages only line up while the order the server
   * answered with is the order they are shown in — a budget is read by year, then chapter, then title.
   *
   * A failed page is reported without clearing what is already there — the positions that were read
   * are still valid, and blanking the screen would suggest they are not.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.error.set(null);
    this.api
      .listBudgetPositions(String(this.pageSize), String(this.positions().length))
      .subscribe({
        next: (rows) => {
          const page = rows ?? [];
          this.positions.set([...this.positions(), ...page]);
          this.hasMore.set(page.length === this.pageSize);
          this.loadingMore.set(false);
        },
        error: () => {
          this.loadingMore.set(false);
          this.moreError.set(this.i18n.translate('haushalt.positions.loadMoreFailed'));
        },
      });
  }

  /** Amounts are rendered by the shared formatter, not by each screen's own. */
  readonly euro = formatCents;
}
