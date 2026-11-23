import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { catchError, from, switchMap, throwError } from 'rxjs';

import { AuthService } from './auth.service';

/**
 * Sends the caller's bearer token and handles its expiry.
 *
 * A token is refreshed before it is used, so a request never fails because it left a second
 * too late. A 401 means the API rejected the token anyway — revoked, wrong audience, a clock
 * far out — and the only honest answer is to forget the session and ask again at the IdP.
 *
 * With no issuer configured nothing is added: the request goes out as it always did.
 */
export const authInterceptor: HttpInterceptorFn = (request, next) => {
  const auth = inject(AuthService);
  const router = inject(Router);

  if (!auth.enabled() || !request.url.startsWith('/api')) {
    return next(request);
  }

  return from(auth.freshToken()).pipe(
    switchMap((token) => {
      const authorised =
        token === null ? request : request.clone({ setHeaders: { Authorization: `Bearer ${token}` } });
      return next(authorised).pipe(
        catchError((error: unknown) => {
          if (error instanceof HttpErrorResponse && error.status === 401 && token !== null) {
            auth.expire();
            // `returnUrl` so the clerk lands back on the screen they were working on.
            void router.navigate(['/login'], {
              queryParams: { returnUrl: router.url },
            });
          }
          return throwError(() => error);
        }),
      );
    }),
  );
};