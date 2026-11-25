import { Component, OnInit, inject, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CampusErpApi, CostType, CostTypeAccount } from '../core/api/generated';
import { PageComponent } from '../ui/page.component';

/**
 * KLR — Kostenarten und ihre Kontenüberleitung.
 *
 * A cost type says *what* a cost is. The catalogue is small and its code is its identity, so the
 * screen lists it and creates entries. The bridge from the chart of accounts to the cost types is
 * shown from both directions: the accounts a chosen cost type owns, and — for a typed account
 * code — the cost type it belongs to. That second direction is what tells an operator whether a
 * mapping already exists before they try to make it.
 *
 * Every label and every message is a key: the catalogue reads the same way in either language,
 * and the cost type's own code and name are shown as the API reports them.
 */
@Component({
  selector: 'app-klr-cost-types',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.klrKostenarten.title" subtitleKey="page.klrKostenarten.subtitle">
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

        @if (loading()) {
          <p role="status">{{ 'klr.costTypes.loading' | transloco }}</p>
        } @else if (!loadError()) {
          @if (types().length === 0) {
            <p role="status">{{ 'klr.common.noCostTypes' | transloco }}</p>
          } @else {
            <h2 id="klr-types-heading">{{ 'klr.costTypes.catalogHeading' | transloco }}</h2>
            <table aria-labelledby="klr-types-heading">
              <caption>
                {{ 'klr.costTypes.catalogCaption' | transloco }}
              </caption>
              <thead>
                <tr>
                  <th scope="col">{{ 'common.code' | transloco }}</th>
                  <th scope="col">{{ 'common.description' | transloco }}</th>
                  <th scope="col">{{ 'klr.costTypes.accountsColumn' | transloco }}</th>
                </tr>
              </thead>
              <tbody>
                @for (type of types(); track type.ID) {
                  <tr [attr.data-testid]="'cost-type-' + type.ID" [class.chosen]="String(type.ID) === chosen()">
                    <td [attr.data-testid]="'cost-type-code-' + type.ID">{{ type.Code }}</td>
                    <td [attr.data-testid]="'cost-type-name-' + type.ID">{{ type.Name }}</td>
                    <td>
                      <button type="button" [attr.data-testid]="'choose-' + type.ID" (click)="choose(type)">
                        {{ 'klr.costTypes.showAccounts' | transloco }}
                      </button>
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
                  {{ (loadingMore() ? 'klr.common.loadingMore' : 'klr.common.loadMore') | transloco }}
                </button>
              </p>
            }

            @if (chosen(); as id) {
              <h2 id="klr-accounts-heading">{{ 'klr.costTypes.accountsHeading' | transloco }}</h2>
              @if (accounts().length === 0) {
                <p role="status" data-testid="no-accounts">
                  {{ 'klr.costTypes.noAccounts' | transloco }}
                </p>
              } @else {
                <table aria-labelledby="klr-accounts-heading">
                  <caption>
                    {{ 'klr.costTypes.accountsCaption' | transloco }}
                  </caption>
                  <thead>
                    <tr>
                      <th scope="col">{{ 'common.account' | transloco }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    @for (account of accounts(); track account.ID) {
                      <tr [attr.data-testid]="'account-' + account.AccountCode">
                        <td>{{ account.AccountCode }}</td>
                      </tr>
                    }
                  </tbody>
                </table>
              }

              <form (submit)="map($event)">
                <h3 id="klr-map-heading">{{ 'klr.costTypes.mapHeading' | transloco }}</h3>
                <div class="field">
                  <label for="klr-account-code">{{ 'klr.common.accountNumber' | transloco }}</label>
                  <input
                    id="klr-account-code"
                    type="text"
                    [value]="accountCode()"
                    (input)="accountCode.set($any($event.target).value)"
                  />
                </div>
                <div class="field">
                  <button type="submit" data-testid="map-account" [disabled]="mapping()">
                    {{ 'klr.common.map' | transloco }}
                  </button>
                </div>
              </form>
            }
          }

          <form (submit)="create($event)">
            <h2 id="klr-create-heading">{{ 'klr.costTypes.createHeading' | transloco }}</h2>
            <div class="field">
              <label for="klr-code">{{ 'common.code' | transloco }}</label>
              <input id="klr-code" type="text" [value]="code()" (input)="code.set($any($event.target).value)" />
            </div>
            <div class="field">
              <label for="klr-name">{{ 'common.description' | transloco }}</label>
              <input id="klr-name" type="text" [value]="name()" (input)="name.set($any($event.target).value)" />
            </div>
            <div class="field">
              <button type="submit" data-testid="create-cost-type" [disabled]="saving()">
                {{ 'klr.common.create' | transloco }}
              </button>
            </div>
          </form>
        }

        <form (submit)="lookup($event)">
          <h2 id="klr-lookup-heading">{{ 'klr.costTypes.lookupHeading' | transloco }}</h2>
          <div class="field">
            <label for="klr-lookup-code">{{ 'klr.common.accountNumber' | transloco }}</label>
            <input
              id="klr-lookup-code"
              type="text"
              [value]="lookupCode()"
              (input)="lookupCode.set($any($event.target).value)"
            />
          </div>
          <div class="field">
            <button type="submit" data-testid="lookup-account">{{ 'klr.costTypes.lookupButton' | transloco }}</button>
          </div>
          @if (lookupResult(); as found) {
            <p role="status" data-testid="lookup-result">{{ found.Code }} {{ found.Name }}</p>
          }
          @if (lookupMiss()) {
            <p role="status" data-testid="lookup-miss">{{ 'klr.costTypes.lookupMiss' | transloco }}</p>
          }
        </form>
      </section>
    </app-page>
  `,
})
export class KlrCostTypesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);
  private readonly i18n = inject(TranslocoService);

  readonly types = signal<CostType[]>([]);
  readonly accounts = signal<CostTypeAccount[]>([]);
  readonly chosen = signal('');
  readonly code = signal('');
  readonly name = signal('');
  readonly accountCode = signal('');
  readonly lookupCode = signal('');
  readonly loading = signal(true);
  readonly saving = signal(false);
  readonly mapping = signal(false);
  /**
   * One page of cost types, sent explicitly rather than left to the server's default: a chart of
   * accounts grows with the institution, and a screen whose page size changed when the server did
   * would be a surprise nobody could see in the code.
   */
  private readonly pageSize = 50;
  /** True while the last page came back full, so there may be another. */
  readonly hasMore = signal(false);
  readonly loadingMore = signal(false);
  readonly loadError = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  readonly lookupResult = signal<CostType | null>(null);
  readonly lookupMiss = signal(false);

  readonly String = String;

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.api.listCostTypes(String(this.pageSize), '0').subscribe({
      next: (types) => {
        const list = types ?? [];
        this.types.set(list);
        // A full page means there may be more; a short one is the end of the catalogue.
        this.hasMore.set(list.length === this.pageSize);
        // A cost type that was chosen before a reload stays chosen, so the accounts below are
        // never about a different row than the one the operator picked.
        const stillThere = list.some((t) => String(t.ID) === this.chosen());
        if (!stillThere) {
          this.chosen.set(list.length > 0 ? String(list[0].ID) : '');
        }
        // The accounts follow the selection. With nothing chosen the list is empty, which is what
        // loadAccounts already does — there is no second rule about it here.
        this.loadAccounts();
        this.loading.set(false);
      },
      error: () => {
        this.loadError.set(this.i18n.translate('klr.common.loadCostTypesFailed'));
        this.hasMore.set(false);
        this.loading.set(false);
      },
    });
  }

  /**
   * Reads the next page of the catalogue and appends it. The offset is the number of rows already on
   * screen, which is why the list is never re-sorted in the browser: the pages only line up while the
   * order the server answered with — by code — is the order they are shown in.
   *
   * A failed page is reported without clearing what is already there, and it leaves the selection
   * alone: the cost type whose accounts are shown is still a row of the catalogue.
   */
  loadMore(): void {
    if (!this.hasMore() || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.actionError.set(null);
    this.api.listCostTypes(String(this.pageSize), String(this.types().length)).subscribe({
      next: (types) => {
        const page = types ?? [];
        this.types.set([...this.types(), ...page]);
        this.hasMore.set(page.length === this.pageSize);
        this.loadingMore.set(false);
      },
      error: () => {
        this.loadingMore.set(false);
        this.actionError.set(this.i18n.translate('klr.common.loadMoreCostTypesFailed'));
      },
    });
  }

  loadAccounts(): void {
    const id = this.chosen();
    if (!id) {
      this.accounts.set([]);
      return;
    }
    this.api.listCostTypeAccounts(id).subscribe({
      next: (accounts) => this.accounts.set(accounts ?? []),
      error: () => this.loadError.set(this.i18n.translate('klr.costTypes.loadAccountsFailed')),
    });
  }

  choose(type: CostType): void {
    this.chosen.set(String(type.ID));
    this.actionError.set(null);
    this.notice.set(null);
    this.loadAccounts();
  }

  create(event: Event): void {
    event.preventDefault();
    if (!this.code().trim() || !this.name().trim()) {
      this.actionError.set(this.i18n.translate('klr.costTypes.createRequired'));
      return;
    }
    this.saving.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.createCostType({ code: this.code(), name: this.name() }).subscribe({
      next: (created) => {
        this.saving.set(false);
        this.notice.set(this.i18n.translate('klr.costTypes.created'));
        this.code.set('');
        this.name.set('');
        // The new cost type is the one the operator just made, so the screen moves to it.
        if (created) {
          this.chosen.set(String(created.ID));
        }
        this.load();
      },
      error: () => {
        this.saving.set(false);
        this.actionError.set(this.i18n.translate('klr.costTypes.createFailed'));
      },
    });
  }

  map(event: Event): void {
    event.preventDefault();
    const account = this.accountCode().trim();
    if (!account) {
      this.actionError.set(this.i18n.translate('klr.common.accountNumberRequired'));
      return;
    }
    this.mapping.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.mapCostTypeAccount(this.chosen(), { account_code: account }).subscribe({
      next: () => {
        this.mapping.set(false);
        this.notice.set(this.i18n.translate('klr.costTypes.accountMapped'));
        this.accountCode.set('');
        this.loadAccounts();
      },
      error: () => {
        this.mapping.set(false);
        this.actionError.set(this.i18n.translate('klr.costTypes.mapFailed'));
      },
    });
  }

  lookup(event: Event): void {
    event.preventDefault();
    const account = this.lookupCode().trim();
    if (!account) {
      this.actionError.set(this.i18n.translate('klr.common.accountNumberRequired'));
      return;
    }
    this.actionError.set(null);
    this.lookupResult.set(null);
    this.lookupMiss.set(false);
    this.api.getCostTypeByAccount(account).subscribe({
      next: (found) => this.lookupResult.set(found),
      // An account nobody has mapped is an answer, not a failure: the API says so with a 404.
      // Anything else — a session expiry, a timeout — is a failure and must not claim the
      // account is unmapped (#176).
      error: (err: unknown) => {
        const status = (err as { status?: number } | null)?.status;
        if (status === 404) {
          this.lookupMiss.set(true);
          return;
        }
        this.actionError.set(this.i18n.translate('klr.costTypes.lookupFailed'));
      },
    });
  }
}
