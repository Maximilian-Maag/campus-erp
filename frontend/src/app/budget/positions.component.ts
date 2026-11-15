import { Component, OnInit, inject, signal } from '@angular/core';
import { BudgetPositionResponse, CampusErpApi } from '../core/api/generated';
import { formatCents } from '../core/format';

/**
 * Budget — Mittelübersicht.
 *
 * Every position the budget holds, with what it was given and what is bound or spent against
 * it. Read-only: the screen shows the budget, it does not move it. The available amount comes
 * from the API rather than being recomputed here, so the figure on screen is the one the
 * Mittelüberwachung actually applies.
 */
@Component({
  selector: 'app-budget-positions',
  template: `
    <section aria-labelledby="budget-heading">
      <h2 id="budget-heading">Mittelübersicht</h2>

      @if (error(); as e) {
        <p role="alert" class="error">{{ e }}</p>
      }

      @if (loading()) {
        <p role="status">Haushaltsstellen werden geladen.</p>
      } @else if (positions().length === 0 && !error()) {
        <p role="status">Es sind keine Haushaltsstellen angelegt.</p>
      } @else if (!error()) {
        <table>
          <caption>
            Ansatz, Verpflichtungen und Ist je Haushaltsstelle, Beträge in Euro
          </caption>
          <thead>
            <tr>
              <th scope="col">Jahr</th>
              <th scope="col">Kapitel</th>
              <th scope="col">Titel</th>
              <th scope="col">Bezeichnung</th>
              <th scope="col" class="amount">Ansatz</th>
              <th scope="col" class="amount">Verpflichtet</th>
              <th scope="col" class="amount">Ist</th>
              <th scope="col" class="amount">Verfügbar</th>
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
        @if (hasMore()) {
          <p>
            <button
              type="button"
              data-testid="load-more"
              [disabled]="loadingMore()"
              (click)="loadMore()"
            >
              {{ loadingMore() ? 'Weitere werden geladen …' : 'Weitere laden' }}
            </button>
          </p>
        }
      }
    </section>
  `,
})
export class BudgetPositionsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

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
        this.error.set('Konnte die Mittelübersicht nicht laden.');
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
          this.error.set('Konnte die weiteren Haushaltsstellen nicht laden.');
        },
      });
  }

  /** Amounts are rendered by the shared formatter, not by each screen's own. */
  readonly euro = formatCents;
}
