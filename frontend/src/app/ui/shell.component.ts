import { Component, computed, inject, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { BreakpointObserver } from '@angular/cdk/layout';
import { MatButtonModule } from '@angular/material/button';
import { MatSidenavModule } from '@angular/material/sidenav';
import { MatToolbarModule } from '@angular/material/toolbar';
import { RouterLink, RouterLinkActive } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';

import { LanguagePickerComponent } from './language-picker.component';
import { NAV_GROUPS } from './nav-model';

/** Below this width the navigation covers the content instead of sharing the width. */
const HANDSET_QUERY = '(max-width: 900px)';

/**
 * The frame every screen sits in: the product name and the language picker in the bar, the
 * navigation in a side panel, the screen in the content area.
 *
 * The panel is permanent on a wide screen and overlaid on a narrow one, where it starts
 * closed — a clerk on a tablet should see the table, not the menu. Navigating closes it, so
 * the destination is what is left on screen.
 */
@Component({
  selector: 'app-shell',
  imports: [
    MatButtonModule,
    MatSidenavModule,
    MatToolbarModule,
    RouterLink,
    RouterLinkActive,
    TranslocoPipe,
    LanguagePickerComponent,
  ],
  template: `
    <a class="skip-link" href="#app-main">{{ 'app.skipToContent' | transloco }}</a>

    <mat-toolbar class="app-bar">
      <button
        type="button"
        matIconButton
        class="app-bar-nav"
        (click)="toggleNav()"
        [attr.aria-label]="(navOpen() ? 'app.closeMenu' : 'app.openMenu') | transloco"
        [attr.aria-expanded]="navOpen()"
        data-testid="nav-toggle"
      >
        <svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true" focusable="false">
          <path
            fill="currentColor"
            d="M3 6h18v2H3V6zm0 5h18v2H3v-2zm0 5h18v2H3v-2z"
          />
        </svg>
      </button>

      <span class="brand">
        <span class="brand-mark" aria-hidden="true">CE</span>
        <span class="brand-text">
          <span class="brand-name" data-testid="app-name">{{ 'app.name' | transloco }}</span>
          <span class="brand-tagline">{{ 'app.tagline' | transloco }}</span>
        </span>
      </span>

      <span class="app-bar-spacer"></span>

      <app-language-picker />
    </mat-toolbar>

    <mat-sidenav-container class="app-frame" [hasBackdrop]="isHandset()">
      <mat-sidenav
        class="app-nav"
        [mode]="isHandset() ? 'over' : 'side'"
        [opened]="navOpen()"
        (openedChange)="navOpen.set($event)"
      >
        <nav [attr.aria-label]="'app.nav' | transloco" data-testid="main-nav">
          @for (group of groups; track group.labelKey) {
            <div class="nav-group">
              <p class="nav-group-label">{{ group.labelKey | transloco }}</p>
              @for (item of group.items; track item.path) {
                <a
                  class="nav-link"
                  [routerLink]="item.path"
                  routerLinkActive="nav-link-active"
                  (click)="onNavigate()"
                >
                  {{ item.labelKey | transloco }}
                </a>
              }
            </div>
          }
        </nav>
      </mat-sidenav>

      <mat-sidenav-content>
        <main id="app-main" class="app-content" tabindex="-1">
          <ng-content />
        </main>
      </mat-sidenav-content>
    </mat-sidenav-container>
  `,
  styles: `
    :host {
      display: block;
      min-height: 100dvh;
    }

    .skip-link {
      position: absolute;
      left: -999px;
      z-index: 100;
      padding: 0.5rem 1rem;
      background: var(--mat-sys-surface);
      color: var(--mat-sys-on-surface);

      &:focus {
        left: 0.5rem;
        top: 0.5rem;
      }
    }

    .app-bar {
      position: sticky;
      top: 0;
      z-index: 2;
      gap: 0.75rem;
      background: var(--mat-sys-surface);
      border-bottom: 1px solid var(--mat-sys-outline-variant);
      color: var(--mat-sys-on-surface);
    }

    .brand {
      display: flex;
      gap: 0.75rem;
      align-items: center;
      min-width: 0;
    }

    .brand-mark {
      display: grid;
      place-items: center;
      width: 2rem;
      height: 2rem;
      border-radius: 0.5rem;
      background: var(--mat-sys-primary);
      color: var(--mat-sys-on-primary);
      font: var(--mat-sys-label-large);
      letter-spacing: 0.02em;
    }

    .brand-text {
      display: flex;
      flex-direction: column;
      line-height: 1.15;
      min-width: 0;
    }

    .brand-name {
      font: var(--mat-sys-title-medium);
    }

    .brand-tagline {
      color: var(--mat-sys-on-surface-variant);
      font: var(--mat-sys-label-small);

      /* The tagline is the first thing to go when the bar is narrow. */
      @media (max-width: 600px) {
        display: none;
      }
    }

    .app-bar-spacer {
      flex: 1 1 auto;
    }

    .app-frame {
      height: calc(100dvh - 4rem);
      background: var(--mat-sys-surface-container-low);
    }

    .app-nav {
      width: 17rem;
      border-right: 1px solid var(--mat-sys-outline-variant);
      background: var(--mat-sys-surface);
    }

    nav {
      display: flex;
      flex-direction: column;
      gap: 0.25rem;
      padding: 0.75rem 0.5rem 2rem;
    }

    .nav-group-label {
      margin: 1rem 0 0.25rem;
      padding: 0 0.75rem;
      color: var(--mat-sys-on-surface-variant);
      font: var(--mat-sys-label-small);
      letter-spacing: 0.06em;
      text-transform: uppercase;
    }

    .nav-link {
      display: block;
      padding: 0.5rem 0.75rem;
      border-radius: 1.25rem;
      color: var(--mat-sys-on-surface-variant);
      font: var(--mat-sys-label-large);
      text-decoration: none;

      &:hover {
        background: var(--mat-sys-surface-container-high);
        color: var(--mat-sys-on-surface);
      }

      &:focus-visible {
        outline: 2px solid var(--mat-sys-primary);
        outline-offset: 1px;
      }

      &.nav-link-active {
        background: var(--mat-sys-secondary-container);
        color: var(--mat-sys-on-secondary-container);
      }
    }

    .app-content {
      min-height: 100%;
      outline: none;
    }
  `,
})
export class ShellComponent {
  private readonly breakpoint = inject(BreakpointObserver);

  readonly groups = NAV_GROUPS;

  private readonly handset = toSignal(this.breakpoint.observe(HANDSET_QUERY), {
    initialValue: { matches: false, breakpoints: {} },
  });

  /** True on a narrow screen, where the navigation is an overlay. */
  readonly isHandset = computed(() => this.handset().matches);

  /** Whether the navigation panel is shown. */
  readonly navOpen = signal(true);

  toggleNav(): void {
    this.navOpen.update((open) => !open);
  }

  /** A destination has been chosen: on a narrow screen, get the menu out of the way. */
  onNavigate(): void {
    if (this.isHandset()) {
      this.navOpen.set(false);
    }
  }
}
