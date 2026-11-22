import { Component, DOCUMENT, effect, inject, input } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';

import { LanguageService } from '../i18n/language.service';

/**
 * The head of a screen: its name, one sentence about what it is for, and a place for the
 * screen's primary action.
 *
 * Every page starts with this, so a user always knows where they are and what the screen
 * does before reading a table. The title and subtitle are message keys rather than text: the
 * heading cannot drift away from the navigation entry for the same screen.
 *
 * The head carries no styles of its own: its look (`.page-head`, `.page-title`,
 * `.page-subtitle`, `.page-actions`) is part of the design system in `src/styles/layout`,
 * so every screen's head is the same head.
 */
@Component({
  selector: 'app-page',
  imports: [TranslocoPipe],
  template: `
    <header class="page-head">
      <div class="page-titles">
        <h1 class="page-title" data-testid="page-title">{{ titleKey() | transloco }}</h1>
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
})
export class PageComponent {
  private readonly i18n = inject(TranslocoService);
  private readonly document = inject(DOCUMENT);
  private readonly activeLanguage = inject(LanguageService).language;

  /** Message key of the page heading, e.g. `page.kasseAnordnungen.title`. */
  readonly titleKey = input.required<string>();
  /** Message key of the one-sentence description under the heading. */
  readonly subtitleKey = input<string>();

  constructor() {
    effect(() => {
      // Reading the language makes the tab title re-render when the interface does.
      this.activeLanguage();
      const screen = this.i18n.translate(this.titleKey());
      this.document.title = `${screen} · ${this.i18n.translate('app.name')}`;
    });
  }
}
