/**
 * The signed-in session: the bearer token, the refresh token, and what the interface shows
 * about the person holding it.
 *
 * It lives in `sessionStorage`, not `localStorage`: two tabs are two sessions, closing the
 * tab ends it, and a shared machine does not keep the last clerk's token for the next one.
 */
export interface Session {
  readonly accessToken: string;
  readonly refreshToken: string | null;
  /** Wall clock when the access token stops being usable, in epoch milliseconds. */
  readonly expiresAt: number;
  readonly subject: string;
  readonly name: string;
  readonly email: string;
  readonly roles: readonly string[];
}

export const SESSION_KEY = 'campus-erp.session';

/** A token that expires in less than this is refreshed before it is used. */
export const REFRESH_MARGIN_MS = 60_000;

/** What a token says about its holder; only ever for display, never for authorisation. */
export interface TokenClaims {
  readonly subject: string;
  readonly name: string;
  readonly email: string;
  readonly roles: readonly string[];
}

/**
 * Reads the claims out of a JWT without verifying it.
 *
 * The signature is the API's business — it verifies every request against the IdP's keys.
 * The interface decodes only to greet the user and to know when to refresh; a forged token
 * would buy nothing here and is rejected where it matters.
 */
export function claimsOf(accessToken: string): TokenClaims {
  const empty: TokenClaims = { subject: '', name: '', email: '', roles: [] };
  const payload = accessToken.split('.')[1];
  if (payload === undefined) {
    return empty;
  }
  try {
    const padded = payload.replace(/-/g, '+').replace(/_/g, '/');
    const json = JSON.parse(atob(padded + '='.repeat((4 - (padded.length % 4)) % 4))) as Record<
      string,
      unknown
    >;
    const text = (key: string): string =>
      typeof json[key] === 'string' ? (json[key] as string) : '';
    const roles = Array.isArray(json['roles'])
      ? (json['roles'] as unknown[]).filter((r): r is string => typeof r === 'string')
      : [];
    return {
      subject: text('sub'),
      name: text('name') || text('email') || text('sub'),
      email: text('email'),
      roles,
    };
  } catch {
    return empty;
  }
}

/** Turns a token response from the IdP into the session the interface keeps. */
export function sessionFromTokenResponse(
  response: {
    access_token?: string;
    refresh_token?: string;
    expires_in?: number;
  },
  now = Date.now(),
): Session | null {
  const accessToken = response.access_token ?? '';
  if (accessToken === '') {
    return null;
  }
  const lifetime = typeof response.expires_in === 'number' && response.expires_in > 0
    ? response.expires_in
    : 300;
  const claims = claimsOf(accessToken);
  return {
    accessToken,
    refreshToken: typeof response.refresh_token === 'string' ? response.refresh_token : null,
    expiresAt: now + lifetime * 1000,
    subject: claims.subject,
    name: claims.name,
    email: claims.email,
    roles: claims.roles,
  };
}

/** Whether a token is worth sending: present and not within the refresh margin of expiry. */
export function isUsable(session: Session | null, now = Date.now()): boolean {
  return session !== null && session.expiresAt - REFRESH_MARGIN_MS > now;
}

/** Reads and writes the session in the tab. Any storage failure is treated as "no session". */
export class SessionStore {
  read(): Session | null {
    try {
      const raw = sessionStorage.getItem(SESSION_KEY);
      return raw === null ? null : (JSON.parse(raw) as Session);
    } catch {
      return null;
    }
  }

  write(session: Session): void {
    try {
      sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
    } catch {
      // Storage full or blocked: the session then lasts only as long as the page.
    }
  }

  clear(): void {
    try {
      sessionStorage.removeItem(SESSION_KEY);
    } catch {
      // Nothing to do: the value is gone with the page anyway.
    }
  }
}