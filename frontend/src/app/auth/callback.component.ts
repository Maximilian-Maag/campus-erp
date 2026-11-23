import { Component, inject, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';

import { PageComponent } from '../ui/page.component';
import { AuthFailure, AuthService } from './auth.service';

/**
 * Where the IdP sends the browser back to.
 *
 * It completes the exchange on arrival and replaces itself with the screen the user asked
 * for, so the callback address never stays in the history. A refused or failed sign-in says
 * so and offers the way back to the login screen rather than an empty page.
 */
@Component({
  selector: 'app-auth-callback',
  imports: [PageComponent, RouterLink, TranslocoPipe],
  template: `
    <app-page titleKey="page.loginCallback.title" subtitleKey="page.loginCallback.subtitle">
      <section>
        @if (errorKey(); as key) {
          <p role="alert" class="error" data-testid="callback-error">{{ key | transloco }}</p>
          <p>
            <a routerLink="/login" data-testid="callback-retry">{{ 'auth.tryAgain' | transloco }}</a>
          </p>
        } @else {
          <p role="status" data-testid="callback-progress">{{ 'auth.working' | transloco }}</p>
        }
      </section>
    </app-page>
  `,
})
export class AuthCallbackComponent {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  readonly errorKey = signal<string | null>(null);

  constructor() {
    void this.complete();
  }

  private async complete(): Promise<void> {
    if (!this.auth.enabled()) {
      await this.router.navigateByUrl('/', { replaceUrl: true });
      return;
    }
    // The query string is read once, from the snapshot: the exchange is not re-run when the
    // screen re-renders.
    const query = new URLSearchParams(this.route.snapshot.queryParams as Record<string, string>);
    try {
      const returnUrl = await this.auth.completeSignIn(query);
      await this.router.navigateByUrl(returnUrl, { replaceUrl: true });
    } catch (error: unknown) {
      this.errorKey.set(error instanceof AuthFailure ? error.messageKey : 'auth.failed');
    }
  }
}