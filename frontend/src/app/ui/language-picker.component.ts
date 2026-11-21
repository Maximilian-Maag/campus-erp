import { Component, inject } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatMenuModule } from '@angular/material/menu';
import { TranslocoPipe } from '@jsverse/transloco';

import { Language, SUPPORTED_LANGUAGES } from '../i18n/dictionary';
import { LanguageService } from '../i18n/language.service';

/**
 * The language picker in the application bar.
 *
 * It shows the language the interface is in and offers the ones the product is published in;
 * choosing one switches the whole interface and remembers the choice. The entries are named in
 * their own language ("Deutsch", "English") because a clerk who cannot read the current
 * interface language still has to be able to find their own.
 */
@Component({
  selector: 'app-language-picker',
  imports: [MatButtonModule, MatMenuModule, TranslocoPipe],
  template: `
    <button
      type="button"
      matButton="outlined"
      class="language-picker"
      [matMenuTriggerFor]="menu"
      [attr.aria-label]="'app.languageSwitch' | transloco"
      data-testid="language-picker"
    >
      <span data-testid="language-current">{{ 'language.' + language() | transloco }}</span>
      <span class="language-caret" aria-hidden="true">▾</span>
    </button>

    <mat-menu #menu="matMenu">
      @for (code of languages; track code) {
        <button
          type="button"
          mat-menu-item
          (click)="choose(code)"
          [attr.aria-current]="code === language() ? 'true' : null"
          [attr.data-testid]="'language-option-' + code"
        >
          {{ 'language.' + code | transloco }}
        </button>
      }
    </mat-menu>
  `,
  styles: `
    .language-picker {
      text-transform: none;
    }

    .language-caret {
      margin-left: 0.35rem;
    }
  `,
})
export class LanguagePickerComponent {
  private readonly i18n = inject(LanguageService);

  readonly languages = SUPPORTED_LANGUAGES;
  readonly language = this.i18n.language;

  choose(code: Language): void {
    this.i18n.use(code);
  }
}
