import { Component, computed, effect, inject, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { BreakpointObserver } from '@angular/cdk/layout';
import { MatButtonModule } from '@angular/material/button';
import { MatSidenavModule } from '@angular/material/sidenav';
import { MatToolbarModule } from '@angular/material/toolbar';
import { Router, RouterLink, RouterLinkActive } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';

import { AuthService } from '../auth/auth.service';
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
        <!-- The bar is a <header>: a landmark, so the product name and the skip link are inside
         one rather than floating in the document body (axe: "region"). -->
    <header class="app-header">
      <a class="skip-link" href="#app-main">{{ 'app.skipToContent' | transloco }}</a>

      <mat-toolbar class="app-bar">
        @if (showNav()) {
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
        }

        <span class="brand">
          <span class="brand-mark" aria-hidden="true">CE</span>
          <span class="brand-text">
            <span class="brand-name" data-testid="app-name">{{ 'app.name' | transloco }}</span>
            <span class="brand-tagline">{{ 'app.tagline' | transloco }}</span>
          </span>
        </span>

        <span class="app-bar-spacer"></span>

        <!-- Who is signed in, and the one action that ends it. A deployment without an
             issuer shows neither: there is no session to name or to end. -->
        @if (principalName(); as name) {
          <span class="principal" data-testid="principal">{{ name }}</span>
          <button
            type="button"
            class="secondary sign-out"
            (click)="signOut()"
            data-testid="sign-out"
          >
            {{ 'auth.signOut' | transloco }}
          </button>
        }

        <app-language-picker />
      </mat-toolbar>
    </header>

    <mat-sidenav-container class="app-frame" [hasBackdrop]="isHandset()">
      <!-- The navigation belongs to a signed-in person: a visitor at the login screen gets
           the screen alone, and a deployment without an issuer has no such wall to stand
           behind, so there it is always there. -->
      @if (showNav()) {
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
      }

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
      background: var(--erp-page);
    }

    .skip-link {
      position: absolute;
      left: -999px;
      z-index: 100;
      padding: var(--erp-space-sm) var(--erp-space-lg);
      color: var(--mat-sys-on-primary);
      background: var(--erp-accent);
      border-radius: var(--erp-radius-sm);

      &:focus {
        top: var(--erp-space-sm);
        left: var(--erp-space-sm);
      }
    }

    .app-header {
      position: sticky;
      top: 0;
      z-index: 2;
    }

    /* The bar stays out of the content's way: one line, a hairline under it, and enough */
    /* translucency that a long table scrolls *under* it rather than being cut off by it. */
    .app-bar {
      height: var(--erp-header-h);
      gap: var(--erp-space-md);
      padding: 0 var(--erp-space-lg);
      color: var(--erp-text);
      background: color-mix(in srgb, var(--erp-surface) 82%, transparent);
      backdrop-filter: blur(12px) saturate(1.4);
      border-bottom: 1px solid var(--erp-hairline);
    }

    .app-bar-nav {
      color: var(--erp-text-muted);

      &:hover {
        color: var(--erp-text);
      }
    }

    .brand {
      display: flex;
      gap: var(--erp-space-md);
      align-items: center;
      min-width: 0;
    }

    /* The mark is the one place the palette is a gradient; it is a logo, not a surface. */
    .brand-mark {
      display: grid;
      place-items: center;
      width: 2rem;
      height: 2rem;
      background: linear-gradient(135deg, var(--mat-sys-primary), var(--mat-sys-tertiary));
      border-radius: var(--erp-radius-sm);
      box-shadow: var(--erp-shadow-1);
      color: var(--mat-sys-on-primary);
      font: var(--mat-sys-label-large);
      font-weight: 700;
      letter-spacing: 0.02em;
    }

    .brand-text {
      display: flex;
      flex-direction: column;
      min-width: 0;
      line-height: 1.2;
    }

    .brand-name {
      font: var(--mat-sys-title-medium);
      font-weight: 600;
      letter-spacing: -0.01em;
    }

    .brand-tagline {
      color: var(--erp-text-muted);
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
      height: calc(100dvh - var(--erp-header-h));
      background: var(--erp-page);
    }

    /* The navigation is a quiet column: no shadow, one hairline, and the active entry is */
    /* the only filled thing in it. */
    .app-nav {
      width: var(--erp-nav-w);
      background: var(--erp-surface);
      border-right: 1px solid var(--erp-hairline);
    }

    nav {
      display: flex;
      flex-direction: column;
      gap: var(--erp-space-2xs);
      padding: var(--erp-space-md) var(--erp-space-sm) var(--erp-space-2xl);
    }

    .nav-group {
      display: block;
      margin-top: var(--erp-space-md);
    }

    .nav-group-label {
      padding: 0 var(--erp-space-md) var(--erp-space-xs);
      color: var(--erp-text-muted);
      font: var(--mat-sys-label-small);
      font-weight: 600;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }

    .nav-link {
      display: block;
      padding: var(--erp-space-sm) var(--erp-space-md);
      color: var(--erp-text-muted);
      border-radius: var(--erp-radius-sm);
      font: var(--mat-sys-label-large);
      text-decoration: none;
      transition:
        background-color var(--erp-duration-fast) var(--erp-ease),
        color var(--erp-duration-fast) var(--erp-ease);

      &:hover {
        color: var(--erp-text);
        background: var(--erp-surface-raised);
        text-decoration: none;
      }

      &:focus-visible {
        outline: 2px solid var(--erp-accent);
        outline-offset: -2px;
      }

      /* Active: the accent as a tint and an inset bar, so the entry is unmistakable */
      /* without a second colour appearing in the navigation. */
      &.nav-link-active {
        color: var(--erp-accent);
        background: var(--erp-accent-soft);
        box-shadow: inset 3px 0 0 var(--erp-accent);
        font-weight: 600;
      }
    }

    .principal {
      color: var(--erp-text-muted);
      font: var(--mat-sys-label-large);
      white-space: nowrap;
    }

    .sign-out {
      min-height: 2rem;
      padding: 0 var(--erp-space-md);

      /* The bar is 56px: a full-height button would touch both edges. */
      box-shadow: none;
    }
  `,
})
export class ShellComponent {
  private readonly breakpoint = inject(BreakpointObserver);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);

  /** The name of the signed-in person, or an empty string when nobody is. */
  readonly principalName = this.auth.principalName;

  readonly groups = NAV_GROUPS;

  private readonly handset = toSignal(this.breakpoint.observe(HANDSET_QUERY), {
    initialValue: { matches: false, breakpoints: {} },
  });

  /** True on a narrow screen, where the navigation is an overlay. */
  readonly isHandset = computed(() => this.handset().matches);

  /**
   * Whether the navigation is shown at all.
   *
   * It belongs to a signed-in person: the login screen is the one place a visitor without a
   * session stands, and a menu of screens they cannot open would only be a wall of redirects.
   * A deployment without an issuer authenticates nobody and guards nothing, so there the
   * navigation is simply always there.
   */
  readonly showNav = computed(() => !this.auth.enabled() || this.auth.isAuthenticated());

  /** Whether the navigation panel is shown. */
  readonly navOpen = signal(true);

  constructor() {
    // On a narrow screen the panel covers the content, so it starts closed: a clerk who
    // opens the product on a phone wants the screen, not the menu. A wide screen keeps it
    // open, and the choice stays the user's once the layout has been measured.
    effect(() => {
      if (this.isHandset()) {
        this.navOpen.set(false);
      }
    });
  }

  toggleNav(): void {
    this.navOpen.update((open) => !open);
  }

  signOut(): void {
    this.auth.signOut();
    void this.router.navigate(['/login']);
  }

  /** A destination has been chosen: on a narrow screen, get the menu out of the way. */
  onNavigate(): void {
    if (this.isHandset()) {
      this.navOpen.set(false);
    }
  }
}
