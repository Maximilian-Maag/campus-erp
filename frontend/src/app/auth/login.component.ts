import { Component, DOCUMENT, effect, inject, signal } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';

import { LanguageService } from '../i18n/language.service';
import { AuthFailure, AuthService } from './auth.service';

/**
 * The sign-in screen.
 *
 * One action, and it is the only one: the product has no password of its own — the university
 * IdP decides who the clerk is, and the API decides what that person may do.
 *
 * It is the one screen that is not a stack of records, so it is not an `<app-page>`: the
 * heading, the one sentence of explanation and the one action sit in a single card, centred on
 * the page, with the navigation out of the way (the shell shows none for a visitor without a
 * session). The card carries no styles of its own — `.auth`, `.auth-card`, `.auth-title`,
 * `.auth-submit` and the rest are part of the design system in `src/styles/layout`, so the
 * screen is themed by the same tokens as every other one.
 *
 * A deployment without an issuer never shows this screen: it has no sign-in to offer, so the
 * visitor is sent straight to the screen they asked for.
 */
@Component({
  selector: 'app-login',
  imports: [TranslocoPipe],
  template: `
    <div class="auth">
      <div class="auth-card">
        <div class="auth-intro">
          <h1 class="auth-title" data-testid="page-title">{{ 'page.login.title' | transloco }}</h1>
          <p class="auth-subtitle">{{ 'page.login.subtitle' | transloco }}</p>
        </div>

        @if (errorKey(); as key) {
          <p role="alert" class="error">{{ key | transloco }}</p>
        }

        <p class="auth-explanation">{{ 'auth.explanation' | transloco }}</p>

        <button type="button" class="auth-submit" (click)="signIn()" data-testid="sign-in">
          {{ 'auth.signIn' | transloco }}
        </button>

        <p class="auth-note">{{ 'auth.redirectNote' | transloco }}</p>
      </div>
    </div>
  `,
})
export class LoginComponent {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly i18n = inject(TranslocoService);
  private readonly document = inject(DOCUMENT);
  private readonly activeLanguage = inject(LanguageService).language;

  readonly errorKey = signal<string | null>(null);

  constructor() {
    effect(() => {
      // The tab title belongs to the screen, as it does on every `<app-page>` screen.
      this.activeLanguage();
      const screen = this.i18n.translate('page.login.title');
      this.document.title = `${screen} · ${this.i18n.translate('app.name')}`;
    });

    effect(() => {
      // No issuer, no login screen: there is nothing here to do.
      if (!this.auth.enabled()) {
        void this.router.navigateByUrl(this.returnUrl(), { replaceUrl: true });
      }
    });
  }

  signIn(): void {
    this.errorKey.set(null);
    this.auth.signIn(this.returnUrl()).catch((error: unknown) => {
      this.errorKey.set(error instanceof AuthFailure ? error.messageKey : 'auth.failed');
    });
  }

  private returnUrl(): string {
    const requested = this.route.snapshot.queryParamMap.get('returnUrl') ?? '';
    // Only a path inside this application: an absolute URL would make the login screen an
    // open redirect.
    return requested.startsWith('/') && !requested.startsWith('//') ? requested : '/';
  }
}
