import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';

import { routes } from './app.routes';
import { authGuard } from './auth/auth.guard';

/**
 * The route table is read by the router, which refuses some shapes outright — a redirect that
 * carries a guard, for instance, throws NG04014 while the application is starting, and that is
 * invisible to every screen test because they navigate a handful of addresses. This spec is
 * the check that the table as a whole is one the router accepts.
 */
describe('the route table', () => {
  it('is a configuration the router accepts', () => {
    TestBed.configureTestingModule({ providers: [provideRouter(routes)] });

    expect(() => TestBed.inject(Router)).not.toThrow();
  });

  it('guards every screen a visitor is supposed to be signed in for', () => {
    const publicPaths = new Set(['login', 'auth/callback']);

    for (const route of routes) {
      if (publicPaths.has(route.path ?? '') || route.redirectTo !== undefined) {
        continue;
      }
      expect(route.canActivate).withContext(route.path ?? '').toContain(authGuard);
    }
  });

  it('leaves the login screen and its callback reachable without a session', () => {
    const byPath = new Map(routes.map((route) => [route.path, route]));

    expect(byPath.get('login')?.canActivate).toBeUndefined();
    expect(byPath.get('auth/callback')?.canActivate).toBeUndefined();
  });

  it('declares the public screens before the catch-all that matches everything', () => {
    // `**` matches `/login` too. Declared first, it swallows the login screen and the guard then
    // redirects the screen to itself — an endless chain of URLs rather than an error.
    const paths = routes.map((route) => route.path);
    const catchAll = paths.indexOf('**');

    expect(catchAll).toBeGreaterThan(-1);
    expect(paths.indexOf('login')).toBeLessThan(catchAll);
    expect(paths.indexOf('auth/callback')).toBeLessThan(catchAll);
  });

  it('never puts a guard on a redirect', () => {
    for (const route of routes) {
      if (route.redirectTo !== undefined) {
        expect(route.canActivate).withContext(route.path ?? '').toBeUndefined();
      }
    }
  });
});