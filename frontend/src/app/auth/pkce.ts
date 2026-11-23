/**
 * PKCE (RFC 7636) and the state value that ties a callback to the request that started it.
 *
 * The interface is a public client — it cannot keep a secret — so the code it gets back from
 * the IdP is bound to a verifier that only this browser tab ever saw. The verifier lives in
 * `sessionStorage` for the length of the round trip and is deleted the moment it is used.
 */

/**
 * The pending sign-in survives a full redirect to the IdP, so it lives in the tab rather than
 * in a signal: one key, because a state without its verifier is a callback that cannot be
 * completed anyway.
 */
const PENDING_KEY = 'campus-erp.auth.pending';
/** An abandoned sign-in must not be completable an hour later. */
const PENDING_TTL_MS = 10 * 60 * 1000;

/** base64url of `bytes` random bytes — the alphabet PKCE expects. */
export function randomToken(bytes = 32): string {
  const raw = new Uint8Array(bytes);
  crypto.getRandomValues(raw);
  return base64Url(raw);
}

/** base64url without padding, as the OAuth specs define it. */
function base64Url(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/** S256: `base64url(sha256(verifier))`. Plain PKCE is not offered — it is not worth it. */
export async function challengeFor(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
  return base64Url(new Uint8Array(digest));
}

interface PendingSignIn {
  readonly state: string;
  readonly verifier: string;
  readonly returnUrl: string;
  readonly startedAt: number;
}

function readPending(): PendingSignIn | null {
  try {
    const raw = sessionStorage.getItem(PENDING_KEY);
    if (raw === null) {
      return null;
    }
    const pending = JSON.parse(raw) as PendingSignIn;
    if (Date.now() - pending.startedAt > PENDING_TTL_MS) {
      return null;
    }
    return pending;
  } catch {
    return null;
  }
}

/** Remembers what the callback has to know, under one key so the three cannot drift. */
export function rememberSignIn(pending: Omit<PendingSignIn, 'startedAt'>): void {
  const value: PendingSignIn = { ...pending, startedAt: Date.now() };
  sessionStorage.setItem(PENDING_KEY, JSON.stringify(value));
}

/**
 * Takes the pending sign-in for a callback, once. A state that does not match — a replayed
 * callback, a second tab, an expired attempt — yields `null` and the caller refuses the code.
 */
export function takePendingSignIn(state: string): PendingSignIn | null {
  const pending = readPending();
  clearSignIn();
  if (pending === null || pending.state !== state) {
    return null;
  }
  return pending;
}

/** Forgets a sign-in that was abandoned or refused. */
export function clearSignIn(): void {
  sessionStorage.removeItem(PENDING_KEY);
}