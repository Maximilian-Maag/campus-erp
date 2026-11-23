import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';

import { provideI18n } from '../i18n/provide-i18n';
import { AuthFailure, AuthService } from './auth.service';
import { LoginComponent } from './login.component';

describe('LoginComponent', () => {
  let signIn: jasmine.Spy;
  let navigateByUrl: jasmine.Spy;
  let enabled: ReturnType<typeof signal<boolean>>;

  async function render(returnUrl: string | null = null, enabledNow = true) {
    enabled = signal(enabledNow);
    signIn = jasmine.createSpy('signIn').and.resolveTo(undefined);
    TestBed.resetTestingModule();
    await TestBed.configureTestingModule({
      imports: [LoginComponent],
      providers: [
        provideRouter([]),
        provideI18n(),
        {
          provide: ActivatedRoute,
          useValue: { snapshot: { queryParamMap: convertToParamMap(returnUrl === null ? {} : { returnUrl }) } },
        },
        { provide: AuthService, useValue: { enabled: enabled.asReadonly(), signIn } },
      ],
    }).compileComponents();
    navigateByUrl = spyOn(TestBed.inject(Router), 'navigateByUrl').and.resolveTo(true);
    const fixture = TestBed.createComponent(LoginComponent);
    fixture.detectChanges();
    return fixture;
  }

  it('offers one action: signing in', async () => {
    const fixture = await render();
    const button = (fixture.nativeElement as HTMLElement).querySelector('[data-testid="sign-in"]');

    expect(button?.textContent?.trim()).toBe('Anmelden');
  });

  it('signs in with the address the visitor was on', async () => {
    const fixture = await render('/kasse/anordnungen');

    (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('[data-testid="sign-in"]')?.click();
    await fixture.whenStable();

    expect(signIn).toHaveBeenCalledWith('/kasse/anordnungen');
  });

  it('starts at the front page when no address was named', async () => {
    const fixture = await render();

    (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('[data-testid="sign-in"]')?.click();
    await fixture.whenStable();

    expect(signIn).toHaveBeenCalledWith('/');
  });

  it('will not carry a visitor off to another site afterwards', async () => {
    const fixture = await render('//evil.example/steal');

    (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('[data-testid="sign-in"]')?.click();
    await fixture.whenStable();

    expect(signIn).toHaveBeenCalledWith('/');
  });

  it('says what went wrong when the sign-in cannot start', async () => {
    const fixture = await render();
    signIn.and.rejectWith(new AuthFailure('unreachable', 'auth.unreachable'));

    (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('[data-testid="sign-in"]')?.click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect((fixture.nativeElement as HTMLElement).querySelector('[role="alert"]')?.textContent).toContain(
      'Der Anmeldedienst ist nicht erreichbar.',
    );
  });

  it('falls back to the general message for a failure it does not know', async () => {
    const fixture = await render();
    signIn.and.rejectWith(new Error('socket closed'));

    (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('[data-testid="sign-in"]')?.click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect((fixture.nativeElement as HTMLElement).querySelector('[role="alert"]')?.textContent).toContain(
      'Die Anmeldung ist fehlgeschlagen.',
    );
  });

  it('is not a screen at all where this deployment has no issuer', async () => {
    await render('/', false);

    expect(navigateByUrl).toHaveBeenCalledWith('/', { replaceUrl: true });
  });
});