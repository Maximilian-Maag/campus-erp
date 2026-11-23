import { DOCUMENT, Injectable, computed, inject, signal } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Router } from '@angular/router';
import { firstValueFrom } from 'rxjs';

import { AuthConfigService } from './auth-config';
import { challengeFor, randomToken, rememberSignIn, takePendingSignIn } from './pkce';
import { Session, SessionStore, isUsable, sessionFromTokenResponse } from './session';

/** The endpoints the interface needs; everything else about the IdP is not its business. */
interface Endpoints {
  readonly authorization: string;
  readonly token: string;
}

/** What a sign-in attempt may fail with, so the caller can say something true. */
export class AuthFailure extends Error {
  constructor(
    message: string,
    /** Message key for the screen that reports it. */
    readonly messageKey: string,
  ) {
    super(message);
  }
}

/**
 * Signing in, staying signed in, and sending the token.
 *
 * The interface is a public OIDC client using Authorization Code with PKCE: it redirects to
 * the university IdP, comes back with a code, exchanges it for a token bound to a verifier
 * only this tab saw, and keeps that token in `sessionStorage`. The API verifies the token and
 * maps its roles; nothing here decides what a user may do.
 *
 * With no issuer configured (the local compose stack) every method is inert and no bearer
 * token is sent — the API's own role table then answers 401 for anything privileged, which is
 * the truth rather than a client-side pretence.
 */
@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly router = inject(Router);
  private readonly document = inject(DOCUMENT);
  private readonly configService = inject(AuthConfigService);
  private readonly store = new SessionStore();

  private readonly current = signal<Session | null>(this.store.read());
  private endpoints: Endpoints | null = null;

  readonly config = this.configService.config;
  readonly session = this.current.asReadonly();
  readonly enabled = computed(() => this.config().enabled);
  readonly isAuthenticated = computed(() => isUsable(this.current()));
  /** What the application bar greets: a name, falling back to the subject. */
  readonly principalName = computed(() => {
    const session = this.current();
    if (session === null) {
      return '';
    }
    return session.name !== '' ? session.name : session.subject;
  });
  readonly roles = computed<readonly string[]>(() => this.current()?.roles ?? []);

  /** Performs the redirect to the IdP. The tab leaves the application here. */
  async signIn(returnUrl: string): Promise<void> {
    const config = this.config();
    if (!config.enabled) {
      throw new AuthFailure('sign-in is not configured', 'auth.notConfigured');
    }
    const endpoints = await this.discover();
    const state = randomToken(16);
    const verifier = randomToken(32);
    rememberSignIn({ state, verifier, returnUrl });
    const url = new URL(endpoints.authorization);
    url.searchParams.set('response_type', 'code');
    url.searchParams.set('client_id', config.clientId);
    url.searchParams.set('redirect_uri', this.redirectUri());
    url.searchParams.set('scope', config.scope);
    url.searchParams.set('state', state);
    url.searchParams.set('code_challenge', await challengeFor(verifier));
    url.searchParams.set('code_challenge_method', 'S256');
    this.document.location.assign(url.toString());
  }

  /**
   * Completes the round trip. Returns the URL the user was on when they signed in, so the
   * callback can put them back where they were.
   */
  async completeSignIn(query: URLSearchParams): Promise<string> {
    const error = query.get('error');
    if (error !== null) {
      throw new AuthFailure(`the IdP refused the sign-in: ${error}`, 'auth.failed');
    }
    const code = query.get('code');
    const state = query.get('state');
    if (code === null || state === null) {
      throw new AuthFailure('the callback carried no code or no state', 'auth.failed');
    }
    // Taking the pending sign-in consumes it: a replayed callback finds nothing.
    const pending = takePendingSignIn(state);
    if (pending === null) {
      throw new AuthFailure('the callback does not match a sign-in from this tab', 'auth.failed');
    }
    const endpoints = await this.discover();
    const body = new HttpParams()
      .set('grant_type', 'authorization_code')
      .set('code', code)
      .set('redirect_uri', this.redirectUri())
      .set('client_id', this.config().clientId)
      .set('code_verifier', pending.verifier);
    const session = await this.exchange(endpoints.token, body);
    this.keep(session);
    return pending.returnUrl === '' ? '/' : pending.returnUrl;
  }

  /** Forgets the session. The IdP session is left alone: signing out of one product is not
   * signing out of the university account. */
  signOut(): void {
    this.store.clear();
    this.current.set(null);
    this.endpoints = null;
  }

  /**
   * The token to send, refreshed if it is about to expire. `null` means "no session", and the
   * request goes out unauthenticated so the API can answer for itself.
   */
  async freshToken(): Promise<string | null> {
    const session = this.current();
    if (session === null) {
      return null;
    }
    if (isUsable(session)) {
      return session.accessToken;
    }
    // `isUsable` is false here, so the token is stale or about to be.
    if (session.refreshToken === null) {
      return null;
    }
    try {
      const body = new HttpParams()
        .set('grant_type', 'refresh_token')
        .set('refresh_token', session.refreshToken)
        .set('client_id', this.config().clientId);
      const refreshed = await this.exchange((await this.discover()).token, body);
      this.keep(refreshed);
      return refreshed.accessToken;
    } catch {
      // A refresh that fails is a session that is over; the next request answers 401 and the
      // interceptor sends the user to the login screen.
      return null;
    }
  }

  /** A 401 from the API: the token is no longer accepted, whatever it says about itself. */
  expire(): void {
    this.store.clear();
    this.current.set(null);
  }

  private keep(session: Session): void {
    this.store.write(session);
    this.current.set(session);
  }

  private async exchange(tokenEndpoint: string, body: HttpParams): Promise<Session> {
    const response = await firstValueFrom(
      this.http.post<{ access_token?: string; refresh_token?: string; expires_in?: number }>(
        tokenEndpoint,
        body.toString(),
        { headers: { 'Content-Type': 'application/x-www-form-urlencoded' } },
      ),
    );
    const session = sessionFromTokenResponse(response ?? {});
    if (session === null) {
      throw new AuthFailure('the IdP returned no access token', 'auth.failed');
    }
    return session;
  }

  private redirectUri(): string {
    const configured = this.config().redirectUri;
    return configured !== '' ? configured : `${this.document.location.origin}/auth/callback`;
  }

  /** The IdP's endpoints, from its discovery document, fetched once per page. */
  private async discover(): Promise<Endpoints> {
    if (this.endpoints !== null) {
      return this.endpoints;
    }
    const issuer = this.config().issuer.replace(/\/+$/, '');
    let document: { authorization_endpoint?: string; token_endpoint?: string };
    try {
      document = await firstValueFrom(
        this.http.get<{ authorization_endpoint?: string; token_endpoint?: string }>(
          `${issuer}/.well-known/openid-configuration`,
        ),
      );
    } catch {
      throw new AuthFailure('the IdP discovery document is unreachable', 'auth.unreachable');
    }
    const authorization = document?.authorization_endpoint ?? '';
    const token = document?.token_endpoint ?? '';
    if (authorization === '' || token === '') {
      throw new AuthFailure('the IdP discovery document names no endpoints', 'auth.unreachable');
    }
    this.endpoints = { authorization, token };
    return this.endpoints;
  }
}