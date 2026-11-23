import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';

import { AuthService } from './auth.service';
import { authInterceptor } from './auth.interceptor';

/** Lets the interceptor's promise chain run: it asks for a token before it sends anything. */
const settle = (): Promise<void> => new Promise((resolve) => setTimeout(resolve));

describe('authInterceptor', () => {
  let http: HttpTestingController;
  let client: HttpClient;
  let router: Router;
  let expire: jasmine.Spy;

  function setUp(configEnabled = true, currentToken: string | null = 'token-1'): void {
    expire = jasmine.createSpy('expire');
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        provideHttpClient(withInterceptors([authInterceptor])),
        provideHttpClientTesting(),
        {
          provide: AuthService,
          useValue: {
            enabled: signal(configEnabled).asReadonly(),
            freshToken: () => Promise.resolve(currentToken),
            expire,
          },
        },
      ],
    });
    http = TestBed.inject(HttpTestingController);
    client = TestBed.inject(HttpClient);
    router = TestBed.inject(Router);
  }

  beforeEach(() => setUp());

  afterEach(() => http.verify());

  it('sends the bearer token with API requests', async () => {
    client.get('/api/payment-orders').subscribe();
    await settle();

    const request = http.expectOne('/api/payment-orders');
    expect(request.request.headers.get('Authorization')).toBe('Bearer token-1');
    request.flush([]);
  });

  it('leaves a request alone when the deployment has no issuer', () => {
    setUp(false);
    client.get('/api/payment-orders').subscribe();

    // No token is asked for at all, so the request leaves at once.
    const request = http.expectOne('/api/payment-orders');
    expect(request.request.headers.has('Authorization')).toBe(false);
    request.flush([]);
  });

  it('leaves a request alone when nobody is signed in', async () => {
    setUp(true, null);
    client.get('/api/payment-orders').subscribe();
    await settle();

    const request = http.expectOne('/api/payment-orders');
    expect(request.request.headers.has('Authorization')).toBe(false);
    request.flush([]);
  });

  it('does not add the token to anything but the API', () => {
    client.get('/healthz').subscribe();

    const request = http.expectOne('/healthz');
    expect(request.request.headers.has('Authorization')).toBe(false);
    request.flush('ok');
  });

  it('forgets the session and asks again at the IdP when the API rejects the token', async () => {
    setUp(true, 'stale-token');
    spyOn(router, 'navigate').and.resolveTo(true);
    let failed = false;
    client.get('/api/payment-orders').subscribe({ error: () => (failed = true) });
    await settle();

    http.expectOne('/api/payment-orders').flush('no', { status: 401, statusText: 'Unauthorized' });

    expect(failed).toBe(true);
    expect(expire).toHaveBeenCalled();
    expect(router.navigate).toHaveBeenCalledWith(['/login'], {
      queryParams: { returnUrl: router.url },
    });
  });

  it('does not blame the session for a 401 on a request that carried no token', async () => {
    setUp(true, null);
    client.get('/api/payment-orders').subscribe({ error: () => undefined });
    await settle();

    http.expectOne('/api/payment-orders').flush('no', { status: 401, statusText: 'Unauthorized' });

    expect(expire).not.toHaveBeenCalled();
  });

  it('lets other failures through untouched', async () => {
    let status = 0;
    client.get('/api/payment-orders').subscribe({ error: (e: { status: number }) => (status = e.status) });
    await settle();

    http.expectOne('/api/payment-orders').flush('boom', { status: 500, statusText: 'Server Error' });

    expect(status).toBe(500);
    expect(expire).not.toHaveBeenCalled();
  });
});