import { Routes } from '@angular/router';
import { BudgetPositionsComponent } from './budget/positions.component';
import { CashbookDayComponent } from './kasse/cashbook-day.component';
import { PurchaseOrdersComponent } from './procurement/orders.component';

export const routes: Routes = [
  { path: 'kasse/tag', component: CashbookDayComponent },
  { path: 'budget/haushaltsstellen', component: BudgetPositionsComponent },
  { path: 'procurement/bestellungen', component: PurchaseOrdersComponent },
];
