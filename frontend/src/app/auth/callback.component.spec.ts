import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, provideRouter } from '@angular/router';

import { provideI18n } from '../i18n/provide-i18n';
import { AuthFailure, AuthService } from './auth.service';
import { AuthCallbackComponent } from './callback.component';

describe('AuthCallbackComponent', () => {
  let completeSignIn: jasmine.Spy;
  let navigateByUrl: jasmine.Spy;

  /** Renders the callback for `query`, with the exchange answering as the test asks. */
  async function render(
    query: Record<string, string>,
    exchange: () => Promise<string> = () => Promise.resolve('/kasse/anordnungen'),
    enabledNow = true,
  ) {
    completeSignIn = jasmine.createSpy('completeSignIn').and.callFake(exchange);
    TestBed.resetTestingModule();
    await TestBed.configureTestingModule({
      imports: [AuthCallbackComponent],
      providers: [
        provideRouter([]),
        provideI18n(),
        { provide: ActivatedRoute, useValue: { snapshot: { queryParams: query } } },
        { provide: AuthService, useValue: { enabled: signal(enabledNow), completeSignIn } },
      ],
    }).compileComponents();
    navigateByUrl = spyOn(TestBed.inject(Router), 'navigateByUrl').and.resolveTo(true);
    const fixture = TestBed.createComponent(AuthCallbackComponent);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture;
  }

  it('completes the exchange and replaces itself with the screen that was asked for', async () => {
    await render({ code: 'code-1', state: 'state-1' });

    expect(completeSignIn).toHaveBeenCalled();
    expect(navigateByUrl).toHaveBeenCalledWith('/kasse/anordnungen', { replaceUrl: true });
  });

  it('says so when the sign-in failed, and offers the way back', async () => {
    const fixture = await render({ error: 'access_denied' }, () =>
      Promise.reject(new AuthFailure('refused', 'auth.failed')),
    );

    const host = fixture.nativeElement as HTMLElement;
    expect(host.querySelector('[data-testid="callback-error"]')?.textContent).toContain(
      'Die Anmeldung ist fehlgeschlagen.',
    );
    expect(host.querySelector('[data-testid="callback-retry"]')).not.toBeNull();
    expect(navigateByUrl).not.toHaveBeenCalled();
  });

  it('reports a failure the service did not name with the general message', async () => {
    const fixture = await render({ code: 'code-1', state: 'state-1' }, () =>
      Promise.reject(new Error('socket closed')),
    );

    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-testid="callback-error"]')
        ?.textContent,
    ).toContain('Die Anmeldung ist fehlgeschlagen.');
  });

  it('is not a screen at all where this deployment has no issuer', async () => {
    await render({ code: 'code-1', state: 'state-1' }, undefined, false);

    expect(completeSignIn).not.toHaveBeenCalled();
    expect(navigateByUrl).toHaveBeenCalledWith('/', { replaceUrl: true });
  });
});