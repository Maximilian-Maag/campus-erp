import { Injectable, signal } from '@angular/core';

/**
 * Where the interface signs in (ADR-0003: the university IdP, not a password in this
 * repository).
 *
 * The values are not baked into the bundle: `config.json` is written when the container
 * starts, so the same image runs against a test IdP and the production one. A deployment that
 * leaves the issuer empty runs unauthenticated, which is what the local compose stack does —
 * the API answers 401 for anything that needs a principal, so the interface never pretends
 * to be signed in.
 */
export interface AuthConfig {
  /** Sign-in is offered only when an issuer and a client id are configured. */
  readonly enabled: boolean;
  /** OIDC issuer; the endpoints come from its discovery document. */
  readonly issuer: string;
  readonly clientId: string;
  /** Scopes requested at the IdP. */
  readonly scope: string;
  /** Where the IdP sends the user back to. Empty means `<origin>/auth/callback`. */
  readonly redirectUri: string;
}

/** What the interface uses when nothing is configured: no sign-in, no bearer token. */
export const ANONYMOUS_AUTH: AuthConfig = {
  enabled: false,
  issuer: '',
  clientId: '',
  scope: 'openid profile email',
  redirectUri: '',
};

const CONFIG_URL = 'config.json';

/**
 * Reads `config.json` from the deployment and turns it into an `AuthConfig`.
 *
 * Anything unexpected — a missing file, a body that is not JSON (a misconfigured nginx
 * answers the SPA's index.html), a half-filled section — degrades to `ANONYMOUS_AUTH` rather
 * than a blank screen: an interface that cannot sign in still shows what the API lets it see.
 */
export function authConfigFrom(raw: unknown): AuthConfig {
  const auth = (raw as { auth?: Record<string, unknown> } | null)?.auth;
  if (!auth || typeof auth !== 'object') {
    return ANONYMOUS_AUTH;
  }
  const text = (value: unknown, fallback: string): string =>
    typeof value === 'string' && value.trim() !== '' ? value.trim() : fallback;
  const issuer = text(auth['issuer'], '');
  const clientId = text(auth['clientId'], '');
  return {
    enabled: issuer !== '' && clientId !== '',
    issuer,
    clientId,
    scope: text(auth['scope'], ANONYMOUS_AUTH.scope),
    redirectUri: text(auth['redirectUri'], ''),
  };
}

/** The deployment's sign-in configuration, loaded once before the first render. */
@Injectable({ providedIn: 'root' })
export class AuthConfigService {
  private readonly current = signal<AuthConfig>(ANONYMOUS_AUTH);

  readonly config = this.current.asReadonly();

  /**
   * Loads `config.json`. Called by `provideAppInitializer`, so the first screen already knows
   * whether it is behind a login instead of flashing the shell and then redirecting.
   */
  async load(): Promise<void> {
    try {
      const response = await fetch(CONFIG_URL, { cache: 'no-store' });
      if (!response.ok) {
        return;
      }
      this.current.set(authConfigFrom(await response.json()));
    } catch {
      // A deployment without the file runs unauthenticated; the API still enforces its own
      // role table, so this is a smaller failure than refusing to render.
    }
  }
}