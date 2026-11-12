/**
 * Reads the machine code out of a failed HTTP response, defensively.
 *
 * Every screen that reports a refusal branches on this, so it lives in one place: the API answers a
 * refusal with `{ error, code }` (`docs/16`), but a failure that never reached the API — a dropped
 * connection, a proxy's HTML error page, a timeout — carries no such body, and reading `code` off it
 * would throw while the screen is trying to explain what went wrong.
 *
 * Anything that is not an object with a string `code` is "no code", which callers fall back from.
 */
export function problemCode(err: unknown): string {
  if (typeof err !== 'object' || err === null) {
    return '';
  }
  const error = (err as { error?: unknown }).error;
  if (typeof error !== 'object' || error === null) {
    return '';
  }
  const code = (error as { code?: unknown }).code;
  return typeof code === 'string' ? code : '';
}
