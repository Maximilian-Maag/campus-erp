import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRouteSnapshot, Router, RouterStateSnapshot, UrlTree, provideRouter } from '@angular/router';

import { AuthService } from './auth.service';
import { authGuard } from './auth.guard';

const route = {} as ActivatedRouteSnapshot;
const state = { url: '/kasse/anordnungen' } as RouterStateSnapshot;

function setUp(enabled: boolean, authenticated: boolean): void {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      {
        provide: AuthService,
        useValue: {
          enabled: signal(enabled),
          isAuthenticated: signal(authenticated),
        },
      },
    ],
  });
}

describe('authGuard', () => {
  it('lets a signed-in visitor through', () => {
    setUp(true, true);

    expect(TestBed.runInInjectionContext(() => authGuard(route, state))).toBe(true);
  });

  it('sends an anonymous visitor to the login screen, remembering where they were going', () => {
    setUp(true, false);

    const result = TestBed.runInInjectionContext(() => authGuard(route, state)) as UrlTree;

    expect(result instanceof UrlTree).toBe(true);
    expect(TestBed.inject(Router).serializeUrl(result)).toBe(
      '/login?returnUrl=%2Fkasse%2Fanordnungen',
    );
  });

  it('refuses to redirect the login screen to itself', () => {
    setUp(true, false);

    const loggingIn = { url: '/login?returnUrl=%2Flogin' } as RouterStateSnapshot;

    expect(TestBed.runInInjectionContext(() => authGuard(route, loggingIn))).toBe(true);
  });

  it('lets everyone through where this deployment has no issuer', () => {
    // The local stack runs unauthenticated: the screens work because the API is open, and the
    // guard must not invent a login screen for it.
    setUp(false, false);

    expect(TestBed.runInInjectionContext(() => authGuard(route, state))).toBe(true);
  });
});