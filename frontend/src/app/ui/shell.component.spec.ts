import { BreakpointObserver } from '@angular/cdk/layout';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { NEVER, of } from 'rxjs';

import { ShellComponent } from './shell.component';
import { NAV_GROUPS } from './nav-model';
import { provideI18n } from '../i18n/provide-i18n';
import { LanguageService } from '../i18n/language.service';

/** A breakpoint observer that answers one fixed question: is this a narrow screen? */
function breakpointMatches(matches: boolean): Partial<BreakpointObserver> {
  return { observe: () => of({ matches, breakpoints: {} }) };
}

/** A breakpoint observer that never answers, as before the layout has been measured. */
function breakpointSilent(observed: string[]): Partial<BreakpointObserver> {
  return {
    observe: (query: string) => {
      observed.push(query);
      return NEVER;
    },
  };
}

async function render(matches = false) {
  TestBed.resetTestingModule();
  await TestBed.configureTestingModule({
    imports: [ShellComponent],
    providers: [
      provideRouter([]),
      provideI18n(),
      { provide: BreakpointObserver, useValue: breakpointMatches(matches) },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(ShellComponent);
  fixture.detectChanges();
  return fixture;
}

describe('ShellComponent', () => {
  afterEach(() => localStorage.clear());

  it('names the product', async () => {
    const fixture = await render();
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-testid="app-name"]')?.textContent?.trim(),
    ).toBe('Campus ERP');
  });

  it('offers every screen in the navigation, grouped', async () => {
    const fixture = await render();
    const links = (fixture.nativeElement as HTMLElement).querySelectorAll('.nav-link');
    const expected = NAV_GROUPS.reduce((sum, group) => sum + group.items.length, 0);

    expect(links.length).toBe(expected);
    expect(expected).toBe(25);
  });

  it('opens with the navigation shown and toggles it', async () => {
    const fixture = await render();
    const component = fixture.componentInstance;
    const toggle = (fixture.nativeElement as HTMLElement).querySelector(
      '[data-testid="nav-toggle"]',
    ) as HTMLButtonElement;

    expect(component.navOpen()).toBe(true);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');

    toggle.click();
    fixture.detectChanges();

    expect(component.navOpen()).toBe(false);
    expect(toggle.getAttribute('aria-label')).toBe('Navigation öffnen');
  });

  it('asks about the handset breakpoint and assumes a wide screen until answered', async () => {
    // The shell is created before the layout has been measured, so it has to start wide;
    // and it has to ask about the breakpoint the stylesheet actually uses.
    const observed: string[] = [];
    TestBed.resetTestingModule();
    await TestBed.configureTestingModule({
      imports: [ShellComponent],
      providers: [
        provideRouter([]),
        provideI18n(),
        { provide: BreakpointObserver, useValue: breakpointSilent(observed) },
      ],
    }).compileComponents();

    const fixture = TestBed.createComponent(ShellComponent);
    fixture.detectChanges();

    // Material's own components observe their media queries too, so this is a containment
    // check rather than an equality one: the shell must have asked about the shared breakpoint.
    expect(observed).toContain('(max-width: 900px)');
    expect(fixture.componentInstance.isHandset()).toBe(false);
  });

  it('gets the menu out of the way after a choice on a narrow screen', async () => {
    const fixture = await render(true);
    const component = fixture.componentInstance;

    (fixture.nativeElement as HTMLElement)
      .querySelectorAll('.nav-link')
      .forEach((link) => (link as HTMLElement).click());
    fixture.detectChanges();

    expect(component.navOpen()).toBe(false);
  });

  it('keeps the menu open after a choice on a wide screen', async () => {
    const fixture = await render(false);

    (fixture.nativeElement as HTMLElement).querySelector('.nav-link')?.dispatchEvent(
      new Event('click'),
    );
    fixture.detectChanges();

    expect(fixture.componentInstance.navOpen()).toBe(true);
  });

  it('renders the navigation in the chosen language', async () => {
    const fixture = await render();
    TestBed.inject(LanguageService).use('en');
    fixture.detectChanges();

    const labels = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('.nav-link'),
    ).map((el) => el.textContent?.trim());

    expect(labels).toContain('Payment orders');
    expect(labels).not.toContain('Zahlungsanordnungen');
  });
});
