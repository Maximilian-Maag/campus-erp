import {
  REFRESH_MARGIN_MS,
  SESSION_KEY,
  SessionStore,
  claimsOf,
  isUsable,
  sessionFromTokenResponse,
} from './session';

/** base64url, no padding — what a JWT segment looks like. */
function b64(value: object): string {
  return btoa(JSON.stringify(value)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/** A token shaped like the IdP's, with `claims` in the payload and a signature that is never
 * read by the interface. */
function token(claims: object): string {
  return `${b64({ alg: 'RS256', typ: 'JWT' })}.${b64(claims)}.signature`;
}

describe('claimsOf', () => {
  it('reads subject, name, email and roles out of the payload', () => {
    const claims = claimsOf(
      token({ sub: 'max.maag', name: 'Max Maag', email: 'max@uni.ac.at', roles: ['erp-kasse'] }),
    );

    expect(claims).toEqual({
      subject: 'max.maag',
      name: 'Max Maag',
      email: 'max@uni.ac.at',
      roles: ['erp-kasse'],
    });
  });

  it('falls back to the email, then the subject, when no name is claimed', () => {
    expect(claimsOf(token({ sub: 'max.maag', email: 'max@uni.ac.at' })).name).toBe('max@uni.ac.at');
    expect(claimsOf(token({ sub: 'max.maag' })).name).toBe('max.maag');
  });

  it('keeps only string roles, and no roles at all when the claim is not a list', () => {
    expect(claimsOf(token({ sub: 'a', roles: ['erp-kasse', 7, null] })).roles).toEqual(['erp-kasse']);
    expect(claimsOf(token({ sub: 'a', roles: 'erp-kasse' })).roles).toEqual([]);
  });

  it('answers empty claims for a token that is not a JWT', () => {
    expect(claimsOf('not-a-token')).toEqual({ subject: '', name: '', email: '', roles: [] });
  });

  it('answers empty claims for a payload that is not JSON', () => {
    expect(claimsOf('header.%%%.signature').roles).toEqual([]);
  });
});

describe('sessionFromTokenResponse', () => {
  it('builds a session from the token response and its claims', () => {
    const session = sessionFromTokenResponse(
      {
        access_token: token({ sub: 'max.maag', name: 'Max Maag', roles: ['erp-kasse'] }),
        refresh_token: 'refresh-1',
        expires_in: 300,
      },
      1_000,
    );

    expect(session?.refreshToken).toBe('refresh-1');
    expect(session?.expiresAt).toBe(1_000 + 300_000);
    expect(session?.name).toBe('Max Maag');
    expect(session?.roles).toEqual(['erp-kasse']);
  });

  it('assumes five minutes when the IdP names no lifetime', () => {
    const session = sessionFromTokenResponse({ access_token: token({ sub: 'a' }) }, 0);

    expect(session?.expiresAt).toBe(300_000);
  });

  it('treats a lifetime of zero as unknown rather than as expired', () => {
    const session = sessionFromTokenResponse({ access_token: token({ sub: 'a' }), expires_in: 0 }, 0);

    expect(session?.expiresAt).toBe(300_000);
  });

  it('has no refresh token when the IdP sent none', () => {
    expect(sessionFromTokenResponse({ access_token: token({ sub: 'a' }) })?.refreshToken).toBeNull();
  });

  it('refuses a response without an access token', () => {
    expect(sessionFromTokenResponse({ refresh_token: 'r' })).toBeNull();
  });
});

describe('isUsable', () => {
  const session = { ...sessionFromTokenResponse({ access_token: token({ sub: 'a' }), expires_in: 600 }, 0)! };

  it('accepts a token with time left on it', () => {
    expect(isUsable(session, 0)).toBe(true);
  });

  it('refuses a token inside the refresh margin, before it actually expires', () => {
    expect(isUsable(session, 600_000 - REFRESH_MARGIN_MS + 1)).toBe(false);
  });

  it('refuses a missing session', () => {
    expect(isUsable(null, 0)).toBe(false);
  });
});

describe('SessionStore', () => {
  const store = new SessionStore();
  const session = sessionFromTokenResponse({ access_token: token({ sub: 'a' }) })!;

  afterEach(() => sessionStorage.clear());

  it('round-trips a session through the tab', () => {
    store.write(session);

    expect(store.read()?.subject).toBe('a');
    expect(sessionStorage.getItem(SESSION_KEY)).not.toBeNull();
  });

  it('answers nothing when the tab holds no session', () => {
    // The precondition is this spec's own: karma shares one window (and one sessionStorage)
    // between spec files, so "no session" must be established, not assumed.
    sessionStorage.clear();

    expect(store.read()).toBeNull();
  });

  it('answers nothing, rather than throwing, for a corrupt value', () => {
    sessionStorage.setItem(SESSION_KEY, 'not json');

    expect(store.read()).toBeNull();
  });

  it('clears the session', () => {
    store.write(session);
    store.clear();

    expect(store.read()).toBeNull();
  });
});