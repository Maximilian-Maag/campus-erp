import { Component, OnInit, inject, signal } from '@angular/core';
import { CampusErpApi, CostType, CostTypeAccount } from '../core/api/generated';

/**
 * KLR — Kostenarten und ihre Kontenüberleitung.
 *
 * A cost type says *what* a cost is. The catalogue is small and its code is its identity, so the
 * screen lists it and creates entries. The bridge from the chart of accounts to the cost types is
 * shown from both directions: the accounts a chosen cost type owns, and — for a typed account
 * code — the cost type it belongs to. That second direction is what tells an operator whether a
 * mapping already exists before they try to make it.
 */
@Component({
  selector: 'app-klr-cost-types',
  template: `
    <section aria-labelledby="klr-cost-types-heading">
      <h2 id="klr-cost-types-heading">Kostenarten</h2>

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
        <p role="status">Kostenarten werden geladen.</p>
      } @else if (!loadError()) {
        @if (types().length === 0) {
          <p role="status">Es sind keine Kostenarten angelegt.</p>
        } @else {
          <h3 id="klr-types-heading">Katalog</h3>
          <table aria-labelledby="klr-types-heading">
            <caption>
              Angelegte Kostenarten
            </caption>
            <thead>
              <tr>
                <th scope="col">Schlüssel</th>
                <th scope="col">Bezeichnung</th>
                <th scope="col">Konten</th>
              </tr>
            </thead>
            <tbody>
              @for (type of types(); track type.ID) {
                <tr [attr.data-testid]="'cost-type-' + type.ID" [class.chosen]="String(type.ID) === chosen()">
                  <td [attr.data-testid]="'cost-type-code-' + type.ID">{{ type.Code }}</td>
                  <td [attr.data-testid]="'cost-type-name-' + type.ID">{{ type.Name }}</td>
                  <td>
                    <button type="button" [attr.data-testid]="'choose-' + type.ID" (click)="choose(type)">
                      Konten anzeigen
                    </button>
                  </td>
                </tr>
              }
            </tbody>
          </table>

          @if (chosen(); as id) {
            <h3 id="klr-accounts-heading">Konten der gewählten Kostenart</h3>
            @if (accounts().length === 0) {
              <p role="status" data-testid="no-accounts">
                Dieser Kostenart ist noch kein Konto zugeordnet.
              </p>
            } @else {
              <table aria-labelledby="klr-accounts-heading">
                <caption>
                  Zugeordnete Konten
                </caption>
                <thead>
                  <tr>
                    <th scope="col">Konto</th>
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
              <h4 id="klr-map-heading">Konto zuordnen</h4>
              <p>
                <label for="klr-account-code">Kontonummer</label>
                <input
                  id="klr-account-code"
                  type="text"
                  [value]="accountCode()"
                  (input)="accountCode.set($any($event.target).value)"
                />
              </p>
              <button type="submit" data-testid="map-account" [disabled]="mapping()">Zuordnen</button>
            </form>
          }
        }

        <form (submit)="create($event)">
          <h3 id="klr-create-heading">Kostenart anlegen</h3>
          <p>
            <label for="klr-code">Schlüssel</label>
            <input id="klr-code" type="text" [value]="code()" (input)="code.set($any($event.target).value)" />
          </p>
          <p>
            <label for="klr-name">Bezeichnung</label>
            <input id="klr-name" type="text" [value]="name()" (input)="name.set($any($event.target).value)" />
          </p>
          <button type="submit" data-testid="create-cost-type" [disabled]="saving()">Anlegen</button>
        </form>
      }

      <form (submit)="lookup($event)">
        <h3 id="klr-lookup-heading">Konto nachschlagen</h3>
        <p>
          <label for="klr-lookup-code">Kontonummer</label>
          <input
            id="klr-lookup-code"
            type="text"
            [value]="lookupCode()"
            (input)="lookupCode.set($any($event.target).value)"
          />
        </p>
        <button type="submit" data-testid="lookup-account">Nachschlagen</button>
        @if (lookupResult(); as found) {
          <p role="status" data-testid="lookup-result">{{ found.Code }} {{ found.Name }}</p>
        }
        @if (lookupMiss()) {
          <p role="status" data-testid="lookup-miss">Für dieses Konto ist keine Kostenart hinterlegt.</p>
        }
      </form>
    </section>
  `,
})
export class KlrCostTypesComponent implements OnInit {
  private readonly api = inject(CampusErpApi);

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
    this.api.listCostTypes().subscribe({
      next: (types) => {
        const list = types ?? [];
        this.types.set(list);
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
        this.loadError.set('Konnte die Kostenarten nicht laden.');
        this.loading.set(false);
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
      error: () => this.loadError.set('Konnte die Konten nicht laden.'),
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
      this.actionError.set('Schlüssel und Bezeichnung sind erforderlich.');
      return;
    }
    this.saving.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.createCostType({ code: this.code(), name: this.name() }).subscribe({
      next: (created) => {
        this.saving.set(false);
        this.notice.set('Kostenart angelegt.');
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
        this.actionError.set('Konnte die Kostenart nicht anlegen.');
      },
    });
  }

  map(event: Event): void {
    event.preventDefault();
    const account = this.accountCode().trim();
    if (!account) {
      this.actionError.set('Eine Kontonummer ist erforderlich.');
      return;
    }
    this.mapping.set(true);
    this.actionError.set(null);
    this.notice.set(null);
    this.api.mapCostTypeAccount(this.chosen(), { account_code: account }).subscribe({
      next: () => {
        this.mapping.set(false);
        this.notice.set('Konto zugeordnet.');
        this.accountCode.set('');
        this.loadAccounts();
      },
      error: () => {
        this.mapping.set(false);
        this.actionError.set('Konnte das Konto nicht zuordnen.');
      },
    });
  }

  lookup(event: Event): void {
    event.preventDefault();
    const account = this.lookupCode().trim();
    if (!account) {
      this.actionError.set('Eine Kontonummer ist erforderlich.');
      return;
    }
    this.actionError.set(null);
    this.lookupResult.set(null);
    this.lookupMiss.set(false);
    this.api.getCostTypeByAccount(account).subscribe({
      next: (found) => this.lookupResult.set(found),
      // An account nobody has mapped is an answer, not a failure: the API says so with a 404.
      error: () => this.lookupMiss.set(true),
    });
  }
}
