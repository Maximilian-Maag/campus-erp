import { DOCUMENT } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { signal } from '@angular/core';

import { AuthConfig, AuthConfigService } from './auth-config';
import { AuthFailure, AuthService } from './auth.service';
import { challengeFor, rememberSignIn } from './pkce';
import { SESSION_KEY, claimsOf } from './session';

const ISSUER = 'https://idp.uni.ac.at/realms/campus';
const DISCOVERY = `${ISSUER}/.well-known/openid-configuration`;
const AUTHORIZE = `${ISSUER}/protocol/openid-connect/auth`;
const TOKEN = `${ISSUER}/protocol/openid-connect/token`;

/** Lets the service's promise chain run: it resolves the discovery cache before it sends. */
const settle = (): Promise<void> => new Promise((resolve) => setTimeout(resolve));

const SIGNED_IN: AuthConfig = {
  enabled: true,
  issuer: ISSUER,
  clientId: 'campus-erp',
  scope: 'openid profile email',
  redirectUri: '',
};

function b64(value: object): string {
  return btoa(JSON.stringify(value)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/** A token with claims but no signature the interface would check. */
function token(claims: object): string {
  return `${b64({ alg: 'RS256' })}.${b64(claims)}.sig`;
}

describe('AuthService', () => {
  let http: HttpTestingController;
  let auth: AuthService;
  let assigned: jasmine.Spy;
  let config: ReturnType<typeof signal<AuthConfig>>;

  function setUp(overrides: Partial<AuthConfig> = {}): void {
    config = signal<AuthConfig>({ ...SIGNED_IN, ...overrides });
    assigned = jasmine.createSpy('assign');
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: AuthConfigService, useValue: { config: config.asReadonly() } },
        {
          provide: DOCUMENT,
          useValue: { location: { origin: 'https://erp.uni.ac.at', assign: assigned } },
        },
      ],
    });
    http = TestBed.inject(HttpTestingController);
    auth = TestBed.inject(AuthService);
  }

  function discovery(): void {
    http
      .expectOne(DISCOVERY)
      .flush({ authorization_endpoint: AUTHORIZE, token_endpoint: TOKEN });
  }

  beforeEach(() => {
    sessionStorage.clear();
    setUp();
  });

  afterEach(() => http.verify());

  describe('signing in', () => {
    it('redirects to the IdP with PKCE, the state it stored, and the return address', async () => {
      const pending = auth.signIn('/kasse/anordnungen');
      discovery();
      await pending;

      expect(assigned).toHaveBeenCalledTimes(1);
      const url = new URL(assigned.calls.mostRecent().args[0] as string);
      expect(url.origin + url.pathname).toBe(AUTHORIZE);
      expect(url.searchParams.get('response_type')).toBe('code');
      expect(url.searchParams.get('client_id')).toBe('campus-erp');
      expect(url.searchParams.get('redirect_uri')).toBe('https://erp.uni.ac.at/auth/callback');
      expect(url.searchParams.get('scope')).toBe('openid profile email');
      expect(url.searchParams.get('code_challenge_method')).toBe('S256');

      // The challenge is the hash of a verifier the tab kept: that is what binds the code.
      const stored = JSON.parse(sessionStorage.getItem('campus-erp.auth.pending')!) as {
        state: string;
        verifier: string;
        returnUrl: string;
      };
      expect(url.searchParams.get('state')).toBe(stored.state);
      expect(url.searchParams.get('code_challenge')).toBe(await challengeFor(stored.verifier));
      expect(stored.returnUrl).toBe('/kasse/anordnungen');
    });

    it('uses a configured redirect address instead of the current origin', async () => {
      setUp({ redirectUri: 'https://erp.uni.ac.at/login/callback' });

      const pending = auth.signIn('/');
      discovery();
      await pending;

      const url = new URL(assigned.calls.mostRecent().args[0] as string);
      expect(url.searchParams.get('redirect_uri')).toBe('https://erp.uni.ac.at/login/callback');
    });

    it('asks the IdP for its endpoints once per page', async () => {
      // Both sign-ins in this test use the same discovery response.
      const first = auth.signIn('/');
      discovery();
      await first;

      await auth.signIn('/');

      http.expectNone(DISCOVERY);
    });

    it('refuses to start when this deployment has no issuer', async () => {
      setUp({ enabled: false, issuer: '', clientId: '' });

      await expectAsync(auth.signIn('/')).toBeRejectedWith(
        jasmine.objectContaining({ messageKey: 'auth.notConfigured' }),
      );
      expect(assigned).not.toHaveBeenCalled();
    });

    it('says the IdP is unreachable when discovery fails', async () => {
      const pending = auth.signIn('/');
      http.expectOne(DISCOVERY).flush('nope', { status: 503, statusText: 'Unavailable' });

      await expectAsync(pending).toBeRejectedWith(
        jasmine.objectContaining({ messageKey: 'auth.unreachable' }),
      );
    });

    it('says the IdP is unreachable when discovery names no endpoints', async () => {
      const pending = auth.signIn('/');
      http.expectOne(DISCOVERY).flush({ issuer: ISSUER });

      await expectAsync(pending).toBeRejectedWith(
        jasmine.objectContaining({ messageKey: 'auth.unreachable' }),
      );
    });
  });

  describe('completing the round trip', () => {
    function beginSignIn(returnUrl = '/kasse/anordnungen'): string {
      rememberSignIn({ state: 'state-1', verifier: 'verifier-1', returnUrl });
      return returnUrl;
    }

    it('exchanges the code for a session and answers the return address', async () => {
      const returnUrl = beginSignIn();
      const pending = auth.completeSignIn(new URLSearchParams({ code: 'code-1', state: 'state-1' }));

      discovery();
      await settle();
      const request = http.expectOne(TOKEN);
      expect(request.request.method).toBe('POST');
      expect(request.request.body as string).toContain('grant_type=authorization_code');
      expect(request.request.body as string).toContain('code=code-1');
      expect(request.request.body as string).toContain('code_verifier=verifier-1');
      expect(request.request.body as string).toContain('client_id=campus-erp');
      request.flush({
        access_token: token({ sub: 'max.maag', name: 'Max Maag', roles: ['erp-kasse'] }),
        expires_in: 300,
      });


      await expectAsync(pending).toBeResolvedTo(returnUrl);
      expect(auth.isAuthenticated()).toBe(true);
      expect(auth.principalName()).toBe('Max Maag');
      expect(auth.roles()).toEqual(['erp-kasse']);
      expect(JSON.parse(sessionStorage.getItem(SESSION_KEY)!).accessToken).toContain('.');
    });

    it('lands on the start screen when the sign-in named no return address', async () => {
      beginSignIn('');
      const pending = auth.completeSignIn(new URLSearchParams({ code: 'code-1', state: 'state-1' }));

      discovery();
      await settle();
      http.expectOne(TOKEN).flush({ access_token: token({ sub: 'a' }) });

      await expectAsync(pending).toBeResolvedTo('/');
    });

    it('refuses a callback from another tab or another attempt', async () => {
      beginSignIn();

      await expectAsync(
        auth.completeSignIn(new URLSearchParams({ code: 'code-1', state: 'state-2' })),
      ).toBeRejectedWith(jasmine.objectContaining({ messageKey: 'auth.failed' }));
      http.expectNone(TOKEN);
    });

    it('refuses a callback with no code', async () => {
      beginSignIn();

      await expectAsync(auth.completeSignIn(new URLSearchParams({ state: 'state-1' }))).toBeRejected();
      http.expectNone(TOKEN);
    });

    it('reports a sign-in the IdP refused', async () => {
      await expectAsync(
        auth.completeSignIn(new URLSearchParams({ error: 'access_denied' })),
      ).toBeRejectedWith(jasmine.objectContaining({ messageKey: 'auth.failed' }));
    });

    it('refuses a token response without an access token', async () => {
      beginSignIn();
      const pending = auth.completeSignIn(new URLSearchParams({ code: 'code-1', state: 'state-1' }));

      discovery();
      await settle();
      http.expectOne(TOKEN).flush({ token_type: 'Bearer' });

      await expectAsync(pending).toBeRejectedWith(jasmine.objectContaining({ messageKey: 'auth.failed' }));
      expect(auth.isAuthenticated()).toBe(false);
    });
  });

  describe('the token it sends', () => {
    async function signedIn(expiresIn: number, refreshToken?: string): Promise<void> {
      rememberSignIn({ state: 'state-1', verifier: 'v', returnUrl: '/' });
      const pending = auth.completeSignIn(new URLSearchParams({ code: 'c', state: 'state-1' }));
      discovery();
      await settle();
      http.expectOne(TOKEN).flush({
        access_token: token({ sub: 'a', name: 'A' }),
        expires_in: expiresIn,
        refresh_token: refreshToken,
      });
      await pending;
    }

    it('answers nothing when nobody is signed in', async () => {
      await expectAsync(auth.freshToken()).toBeResolvedTo(null);
    });

    it('hands over a token that still has time on it, without asking the IdP', async () => {
      await signedIn(300);

      const fresh = await auth.freshToken();
      expect(fresh).toContain('.');
      http.expectNone(TOKEN);
    });

    it('refreshes a token that is about to expire', async () => {
      await signedIn(1, 'refresh-1');
      const pending = auth.freshToken();

      await settle();
      const request = http.expectOne(TOKEN);
      expect(request.request.body as string).toContain('grant_type=refresh_token');
      expect(request.request.body as string).toContain('refresh_token=refresh-1');
      request.flush({ access_token: token({ sub: 'a', name: 'A' }), expires_in: 300 });

      const fresh = await pending;
      expect(fresh).toContain('.');
      expect(auth.isAuthenticated()).toBe(true);
    });

    it('answers nothing when an expired token cannot be refreshed', async () => {
      await signedIn(1);
      const pending = auth.freshToken();
      await settle();

      await expectAsync(pending).toBeResolvedTo(null);
    });

    it('answers nothing when the IdP refuses the refresh', async () => {
      await signedIn(1, 'refresh-1');
      const pending = auth.freshToken();
      await settle();
      http.expectOne(TOKEN).flush('denied', { status: 400, statusText: 'Bad Request' });

      await expectAsync(pending).toBeResolvedTo(null);
      http.expectNone(TOKEN);
    });
  });

  describe('ending the session', () => {
    async function signedIn(): Promise<void> {
      rememberSignIn({ state: 's', verifier: 'v', returnUrl: '/' });
      const pending = auth.completeSignIn(new URLSearchParams({ code: 'c', state: 's' }));
      discovery();
      await settle();
      http.expectOne(TOKEN).flush({
        access_token: token({ sub: 'max.maag', name: 'Max Maag' }),
        expires_in: 300,
      });
      await pending;
    }

    it('forgets the session on sign-out', async () => {
      await signedIn();

      auth.signOut();

      expect(auth.isAuthenticated()).toBe(false);
      expect(auth.session()).toBeNull();
      expect(auth.principalName()).toBe('');
      expect(sessionStorage.getItem(SESSION_KEY)).toBeNull();
    });

    it('forgets the session when the API stops accepting it', async () => {
      await signedIn();

      auth.expire();

      expect(auth.isAuthenticated()).toBe(false);
      expect(sessionStorage.getItem(SESSION_KEY)).toBeNull();
    });

    it('greets by subject when the token carries no name', async () => {
      rememberSignIn({ state: 's', verifier: 'v', returnUrl: '/' });
      const pending = auth.completeSignIn(new URLSearchParams({ code: 'c', state: 's' }));
      discovery();
      await settle();
      http.expectOne(TOKEN).flush({ access_token: token({ sub: 'k.wimmer' }), expires_in: 300 });
      await pending;

      expect(auth.principalName()).toBe('k.wimmer');
      expect(claimsOf(auth.session()!.accessToken).subject).toBe('k.wimmer');
    });
  });
});