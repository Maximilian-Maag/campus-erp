import { Routes } from '@angular/router';

import { AuthCallbackComponent } from './auth/callback.component';
import { authGuard } from './auth/auth.guard';
import { LoginComponent } from './auth/login.component';
import { BudgetPositionsComponent } from './budget/positions.component';
import { BudgetSourcesComponent } from './budget/sources.component';
import { CashbookDayComponent } from './kasse/cashbook-day.component';
import { CashAccountsComponent } from './kasse/cash-accounts.component';
import { DunningComponent } from './kasse/dunning.component';
import { DunningRunComponent } from './kasse/dunning-run.component';
import { ReturnPostingsComponent } from './kasse/return-postings.component';
import { MandatesComponent } from './kasse/mandates.component';
import { PaymentRunsComponent } from './kasse/payment-runs.component';
import { EInvoicesComponent } from './einvoice/invoices.component';
import { FoundationsComponent } from './finanzen/foundations.component';
import { TaxesComponent } from './finanzen/taxes.component';
import { KlrAllocationComponent } from './klr/allocation.component';
import { KlrCostTypesComponent } from './klr/cost-types.component';
import { KlrCostUnitsComponent } from './klr/cost-units.component';
import { CostCentresComponent } from './klr/cost-centres.component';
import { KlrServiceRatesComponent } from './klr/service-rates.component';
import { BankStatementsComponent } from './kasse/bank-statements.component';
import { CollectionRunsComponent } from './kasse/collection-runs.component';
import { PaymentOrdersComponent } from './kasse/payment-orders.component';
import { PurchaseOrdersComponent } from './procurement/orders.component';
import { GoodsReceiptsComponent } from './procurement/goods-receipts.component';
import { AuswertungenComponent } from './reports/auswertungen.component';
import { ImportsComponent } from './etl/imports.component';
import { ReceivablesComponent } from './kasse/receivables.component';
import { NotFoundComponent } from './ui/not-found.component';

/**
 * The screens, as declared. `routes` below adds the sign-in guard to each of them; the two
 * that a visitor must reach without a session are named in `PUBLIC_PATHS`.
 */
const DECLARED: Routes = [
  // The product opens on the screen a clerk starts the day with, rather than on an empty
  // outlet: the root is a redirect, not a screen of its own.
  { path: '', pathMatch: 'full', redirectTo: 'kasse/tag' },
  { path: 'kasse/tag', component: CashbookDayComponent },
  { path: 'kasse/konten', component: CashAccountsComponent },
  { path: 'budget/haushaltsstellen', component: BudgetPositionsComponent },
  { path: 'budget/mittelherkunft', component: BudgetSourcesComponent },
  { path: 'procurement/bestellungen', component: PurchaseOrdersComponent },
  { path: 'procurement/wareneingang', component: GoodsReceiptsComponent },
  { path: 'einvoice/rechnungen', component: EInvoicesComponent },
  { path: 'klr/umlage', component: KlrAllocationComponent },
  { path: 'klr/kostenarten', component: KlrCostTypesComponent },
  { path: 'klr/kostentraeger', component: KlrCostUnitsComponent },
  { path: 'klr/kostenstellen', component: CostCentresComponent },
  { path: 'klr/verrechnungspreise', component: KlrServiceRatesComponent },
  { path: 'kasse/anordnungen', component: PaymentOrdersComponent },
  { path: 'kasse/zahlungslaeufe', component: PaymentRunsComponent },
  { path: 'kasse/kontoauszuege', component: BankStatementsComponent },
  { path: 'kasse/offene-posten', component: ReceivablesComponent },
  { path: 'kasse/einzuege', component: CollectionRunsComponent },
  { path: 'kasse/mahnwesen', component: DunningComponent },
  { path: 'kasse/mahnlauf', component: DunningRunComponent },
  { path: 'kasse/ruecklaeufer', component: ReturnPostingsComponent },
  { path: 'kasse/mandate', component: MandatesComponent },
  { path: 'finanzen/stiftungen', component: FoundationsComponent },
  { path: 'finanzen/steuern', component: TaxesComponent },
  { path: 'auswertungen', component: AuswertungenComponent },
  { path: 'importe', component: ImportsComponent },
  // The sign-in round trip: the login screen, and the address the IdP sends the browser back
  // to. Neither needs a session, which is the point of both — and both must be declared before
  // the catch-all below, or `**` matches them first and the guard sends them to themselves.
  { path: 'login', component: LoginComponent },
  { path: 'auth/callback', component: AuthCallbackComponent },
  // Anything else is a link that leads nowhere: say so rather than render an empty outlet.
  { path: '**', component: NotFoundComponent },
];

/** Routes a visitor reaches without a session. */
const PUBLIC_PATHS: ReadonlySet<string> = new Set(['login', 'auth/callback']);

/**
 * Every other screen requires a session — when the deployment has an issuer at all. The guard
 * is a no-op on the local stack, which runs unauthenticated by design.
 *
 * A redirect cannot carry a guard (`redirectTo` and `canActivate` are refused together by the
 * router: the redirect happens before guards run), and it does not need one — the address it
 * sends the visitor to is guarded.
 */
export const routes: Routes = DECLARED.map((route) =>
  PUBLIC_PATHS.has(route.path ?? '') || route.redirectTo !== undefined
    ? route
    : { ...route, canActivate: [authGuard] },
);
