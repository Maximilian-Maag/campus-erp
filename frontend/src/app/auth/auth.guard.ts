import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';

import { AuthService } from './auth.service';

/**
 * Keeps an unauthenticated visitor out of a screen — but only where signing in exists.
 *
 * The interface runs without an issuer on the local stack, and there the guard is a no-op:
 * the screens work because the API is open, not because the client believes it is signed in.
 * With an issuer configured, the visitor is sent to the login screen and brought back to the
 * address they asked for.
 */
export const authGuard: CanActivateFn = (_route, state) => {
  const auth = inject(AuthService);
  if (!auth.enabled() || auth.isAuthenticated()) {
    return true;
  }
  // A guard that redirects to the login screen must never guard the login screen: with the
  // return address in the query, a misordered route table turns that into a redirect loop that
  // ends in a dead tab rather than an error.
  if (state.url.startsWith('/login') || state.url.startsWith('/auth/callback')) {
    return true;
  }
  return inject(Router).createUrlTree(['/login'], { queryParams: { returnUrl: state.url } });
};