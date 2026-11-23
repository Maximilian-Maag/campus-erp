import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { AuthService } from './auth/auth.service';
import { App } from './app';
import { provideI18n } from './i18n/provide-i18n';

describe('App', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [
        provideRouter([]),
        provideI18n(),
        // The frame asks the sign-in service who is here; these tests are about the frame.
        {
          provide: AuthService,
          useValue: {
            enabled: signal(false),
            isAuthenticated: signal(false),
            principalName: signal(''),
            signOut: () => undefined,
          } as unknown as AuthService,
        },
      ],
    }).compileComponents();
  });

  it('should create the app', () => {
    const fixture = TestBed.createComponent(App);
    expect(fixture.componentInstance).toBeTruthy();
  });

  it('renders the frame with the product name', () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.querySelector('app-shell')).toBeTruthy();
    expect(compiled.textContent).toContain('Campus ERP');
  });
});
