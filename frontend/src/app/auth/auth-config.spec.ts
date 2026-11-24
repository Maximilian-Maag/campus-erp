import { TestBed } from '@angular/core/testing';

import { ANONYMOUS_AUTH, AuthConfigService, authConfigFrom } from './auth-config';

describe('authConfigFrom', () => {
  it('reads a complete section', () => {
    expect(
      authConfigFrom({
        auth: {
          issuer: 'https://idp.uni.ac.at/realms/campus',
          clientId: 'campus-erp',
          scope: 'openid profile email offline_access',
          redirectUri: 'https://erp.uni.ac.at/auth/callback',
        },
      }),
    ).toEqual({
      enabled: true,
      issuer: 'https://idp.uni.ac.at/realms/campus',
      clientId: 'campus-erp',
      scope: 'openid profile email offline_access',
      redirectUri: 'https://erp.uni.ac.at/auth/callback',
    });
  });

  it('trims what the deployment wrote', () => {
    const config = authConfigFrom({ auth: { issuer: '  https://idp  ', clientId: ' erp ' } });

    expect(config.issuer).toBe('https://idp');
    expect(config.clientId).toBe('erp');
  });

  it('defaults the scope, and computes the redirect address at run time', () => {
    const config = authConfigFrom({ auth: { issuer: 'https://idp', clientId: 'erp' } });

    expect(config.scope).toBe('openid profile email');
    expect(config.redirectUri).toBe('');
  });

  it('is off when no section is written at all', () => {
    expect(authConfigFrom({})).toEqual(ANONYMOUS_AUTH);
    expect(authConfigFrom(null)).toEqual(ANONYMOUS_AUTH);
  });

  it('is off when the section is not an object', () => {
    expect(authConfigFrom({ auth: 'yes' })).toEqual(ANONYMOUS_AUTH);
  });

  it('is off when the issuer or the client id is missing — a half configuration cannot sign in', () => {
    expect(authConfigFrom({ auth: { issuer: 'https://idp' } }).enabled).toBe(false);
    expect(authConfigFrom({ auth: { clientId: 'erp' } }).enabled).toBe(false);
  });

  it('ignores values that are not strings', () => {
    expect(authConfigFrom({ auth: { issuer: 42, clientId: ['erp'] } })).toEqual(ANONYMOUS_AUTH);
  });
});

describe('AuthConfigService', () => {
  let service: AuthConfigService;

  function answer(response: Response | Error): void {
    spyOn(window, 'fetch').and.callFake(() =>
      response instanceof Error ? Promise.reject(response) : Promise.resolve(response),
    );
  }

  beforeEach(() => {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({});
    service = TestBed.inject(AuthConfigService);
  });

  it('starts anonymous, before anything has been read', () => {
    expect(service.config()).toEqual(ANONYMOUS_AUTH);
  });

  it('reads the deployment configuration', async () => {
    answer(
      new Response(JSON.stringify({ auth: { issuer: 'https://idp', clientId: 'erp' } }), {
        status: 200,
      }),
    );

    await service.load();

    expect(service.config().enabled).toBe(true);
    expect(service.config().issuer).toBe('https://idp');
    expect(window.fetch).toHaveBeenCalledWith('config.json', { cache: 'no-store' });
  });

  it('stays anonymous when the file is not there', async () => {
    answer(new Response('not found', { status: 404 }));

    await service.load();

    expect(service.config()).toEqual(ANONYMOUS_AUTH);
  });

  it('stays anonymous when the file is the SPA fallback rather than JSON', async () => {
    // A static host that answers every path with index.html: the body parses as nothing.
    answer(new Response('<!doctype html><html></html>', { status: 200 }));

    await service.load();

    expect(service.config()).toEqual(ANONYMOUS_AUTH);
  });

  it('stays anonymous when the request itself fails', async () => {
    answer(new Error('offline'));

    await service.load();

    expect(service.config()).toEqual(ANONYMOUS_AUTH);
  });
});