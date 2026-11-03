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
      }
    </section>
  `,
})
export class BudgetPositionsComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

  readonly positions = signal<BudgetPositionResponse[]>([]);
  readonly loading = signal(true);
  readonly error = signal<string | null>(null);

  // Loading starts when the screen is shown, not when it is constructed: the initial state is
  // then observable (and testable) before any request is made.
  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.error.set(null);
    this.api.listBudgetPositions().subscribe({
      next: (rows) => {
        this.positions.set(rows ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.error.set('Konnte die Mittelübersicht nicht laden.');
        this.loading.set(false);
      },
    });
  }

  /** Amounts are rendered by the shared formatter, not by each screen's own. */
  readonly euro = formatCents;
}
