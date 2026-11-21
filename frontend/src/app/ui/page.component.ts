import { Component, input } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';

/**
 * The head of a screen: its name, one sentence about what it is for, and a place for the
 * screen's primary action.
 *
 * Every page starts with this, so a user always knows where they are and what the screen
 * does before reading a table. The title and subtitle are message keys rather than text: the
 * heading cannot drift away from the navigation entry for the same screen.
 */
@Component({
  selector: 'app-page',
  imports: [TranslocoPipe],
  template: `
    <header class="page-head">
      <div class="page-titles">
        <h1 data-testid="page-title">{{ titleKey() | transloco }}</h1>
        @if (subtitleKey(); as key) {
          <p class="page-subtitle">{{ key | transloco }}</p>
        }
      </div>
      <div class="page-actions">
        <ng-content select="[actions]" />
      </div>
    </header>
    <ng-content />
  `,
  styles: `
    .page-head {
      display: flex;
      flex-wrap: wrap;
      gap: 1rem;
      align-items: flex-start;
      justify-content: space-between;
      padding: 1.5rem var(--erp-gutter) 0.5rem;
      max-width: var(--erp-content-max);
    }

    .page-titles {
      min-width: 0;
    }

    h1 {
      margin: 0;
      font: var(--mat-sys-headline-medium);
      letter-spacing: -0.015em;
    }

    .page-subtitle {
      margin: 0.25rem 0 0;
      color: var(--mat-sys-on-surface-variant);
      font: var(--mat-sys-body-large);
    }

    .page-actions {
      display: flex;
      gap: 0.5rem;
      align-items: center;
    }
  `,
})
export class PageComponent {
  /** Message key of the page heading, e.g. `page.kasseAnordnungen.title`. */
  readonly titleKey = input.required<string>();
  /** Message key of the one-sentence description under the heading. */
  readonly subtitleKey = input<string>();
}
