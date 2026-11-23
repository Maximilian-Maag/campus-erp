import {
  challengeFor,
  clearSignIn,
  randomToken,
  rememberSignIn,
  takePendingSignIn,
} from './pkce';

const PENDING_KEY = 'campus-erp.auth.pending';

describe('randomToken', () => {
  it('produces base64url without padding', () => {
    const value = randomToken(32);

    expect(value).toMatch(/^[A-Za-z0-9_-]+$/);
    // 32 bytes are 43 base64 characters, unpadded.
    expect(value.length).toBe(43);
  });

  it('does not repeat itself', () => {
    expect(randomToken()).not.toBe(randomToken());
  });
});

describe('challengeFor', () => {
  it('matches the RFC 7636 example for S256', async () => {
    // The verifier/challenge pair from RFC 7636, Appendix B.
    const verifier = 'dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk';

    expect(await challengeFor(verifier)).toBe('E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM');
  });
});

describe('the pending sign-in', () => {
  afterEach(() => sessionStorage.clear());

  it('comes back for the state that took it', () => {
    rememberSignIn({ state: 'state-1', verifier: 'verifier-1', returnUrl: '/kasse/anordnungen' });

    expect(takePendingSignIn('state-1')).toEqual(
      jasmine.objectContaining({ state: 'state-1', verifier: 'verifier-1', returnUrl: '/kasse/anordnungen' }),
    );
  });

  it('is consumed: a replayed callback finds nothing', () => {
    rememberSignIn({ state: 'state-1', verifier: 'v', returnUrl: '/' });
    takePendingSignIn('state-1');

    expect(takePendingSignIn('state-1')).toBeNull();
  });

  it('is refused for a state from another tab or another attempt', () => {
    rememberSignIn({ state: 'state-1', verifier: 'v', returnUrl: '/' });

    expect(takePendingSignIn('state-2')).toBeNull();
  });

  it('is refused once it is too old to belong to this sign-in', () => {
    rememberSignIn({ state: 'state-1', verifier: 'v', returnUrl: '/' });
    const stored = JSON.parse(sessionStorage.getItem(PENDING_KEY)!) as Record<string, unknown>;
    sessionStorage.setItem(
      PENDING_KEY,
      JSON.stringify({ ...stored, startedAt: Date.now() - 11 * 60 * 1000 }),
    );

    expect(takePendingSignIn('state-1')).toBeNull();
  });

  it('is refused when the tab holds nonsense', () => {
    sessionStorage.setItem(PENDING_KEY, 'not json');

    expect(takePendingSignIn('state-1')).toBeNull();
  });

  it('is forgotten on demand', () => {
    rememberSignIn({ state: 'state-1', verifier: 'v', returnUrl: '/' });
    clearSignIn();

    expect(sessionStorage.getItem(PENDING_KEY)).toBeNull();
  });
});