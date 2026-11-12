import { problemCode } from './problem';

/**
 * A failed Angular request wraps the response body in `error`, so the API's problem body
 * (`{ error, code }`, docs/16) is reached as `err.error`. These tests keep the two layers apart on
 * purpose: reading the body's `code` off the response itself is the mistake the helper exists to
 * avoid.
 */
const httpError = (body: unknown): unknown => ({ error: body });

describe('problemCode', () => {
  it('reads the code a refusal carries', () => {
    expect(problemCode(httpError({ error: 'refused', code: 'kasse.booking_not_found' }))).toBe(
      'kasse.booking_not_found',
    );
  });

  it('answers "no code" for every shape that is not the API\'s problem body', () => {
    // Each case is one way a failure can arrive without a code: nothing at all, a response that is
    // not an object, a body that is not an object (an HTML error page, a plain string), a problem
    // object with no code, and a code that is present but not a string. Reading any of them as a
    // code would put rubbish — or an exception — on screen while explaining a failure.
    const cases: unknown[] = [
      undefined,
      null,
      'Internal Server Error',
      500,
      {},
      httpError(null),
      httpError('Internal Server Error'),
      { error: 'refused' },
      httpError({ error: 'refused' }),
      httpError({ error: 'refused', code: null }),
      httpError({ error: 'refused', code: 42 }),
      httpError({ error: {}, code: undefined }),
    ];
    for (const err of cases) {
      expect(problemCode(err))
        .withContext(`problemCode(${JSON.stringify(err)})`)
        .toBe('');
    }
  });
});
