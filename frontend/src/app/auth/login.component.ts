import { Component, effect, inject, signal } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';

import { PageComponent } from '../ui/page.component';
import { AuthFailure, AuthService } from './auth.service';

/**
 * The sign-in screen.
 *
 * One action, and it is the only one: the product has no password of its own — the university
 * IdP decides who the clerk is, and the API decides what that person may do.
 *
 * A deployment without an issuer never shows this screen: it has no sign-in to offer, so the
 * visitor is sent straight to the screen they asked for.
 */
@Component({
  selector: 'app-login',
  imports: [PageComponent, TranslocoPipe],
  template: `
    <app-page titleKey="page.login.title" subtitleKey="page.login.subtitle">
      <section>
        @if (errorKey(); as key) {
          <p role="alert" class="error">{{ key | transloco }}</p>
        }
        <p class="lede">{{ 'auth.explanation' | transloco }}</p>
        <p>
          <button type="button" (click)="signIn()" data-testid="sign-in">
            {{ 'auth.signIn' | transloco }}
          </button>
        </p>
      </section>
    </app-page>
  `,
})
export class LoginComponent {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  readonly errorKey = signal<string | null>(null);

  constructor() {
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