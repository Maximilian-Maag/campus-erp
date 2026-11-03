import { Routes } from '@angular/router';
import { BudgetPositionsComponent } from './budget/positions.component';
import { CashbookDayComponent } from './kasse/cashbook-day.component';

export const routes: Routes = [
  { path: 'kasse/tag', component: CashbookDayComponent },
  { path: 'budget/haushaltsstellen', component: BudgetPositionsComponent },
];
