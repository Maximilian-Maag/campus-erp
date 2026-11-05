import { Routes } from '@angular/router';
import { BudgetPositionsComponent } from './budget/positions.component';
import { CashbookDayComponent } from './kasse/cashbook-day.component';
import { EInvoicesComponent } from './einvoice/invoices.component';
import { KlrAllocationComponent } from './klr/allocation.component';
import { KlrCostTypesComponent } from './klr/cost-types.component';
import { KlrCostUnitsComponent } from './klr/cost-units.component';
import { KlrServiceRatesComponent } from './klr/service-rates.component';
import { BankStatementsComponent } from './kasse/bank-statements.component';
import { CollectionRunsComponent } from './kasse/collection-runs.component';
import { PaymentOrdersComponent } from './kasse/payment-orders.component';
import { PurchaseOrdersComponent } from './procurement/orders.component';
import { ReceivablesComponent } from './kasse/receivables.component';

export const routes: Routes = [
  { path: 'kasse/tag', component: CashbookDayComponent },
  { path: 'budget/haushaltsstellen', component: BudgetPositionsComponent },
  { path: 'procurement/bestellungen', component: PurchaseOrdersComponent },
  { path: 'einvoice/rechnungen', component: EInvoicesComponent },
  { path: 'klr/umlage', component: KlrAllocationComponent },
  { path: 'klr/kostenarten', component: KlrCostTypesComponent },
  { path: 'klr/kostentraeger', component: KlrCostUnitsComponent },
  { path: 'klr/verrechnungspreise', component: KlrServiceRatesComponent },
  { path: 'kasse/anordnungen', component: PaymentOrdersComponent },
  { path: 'kasse/kontoauszuege', component: BankStatementsComponent },
  { path: 'kasse/offene-posten', component: ReceivablesComponent },
  { path: 'kasse/einzuege', component: CollectionRunsComponent },
];
