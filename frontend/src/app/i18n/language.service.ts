import { DOCUMENT, inject, Injectable, signal } from '@angular/core';
import { TranslocoService } from '@jsverse/transloco';

import { DEFAULT_LANGUAGE, isLanguage, Language, SUPPORTED_LANGUAGES } from './dictionary';

/** Where the office's choice is remembered between visits. */
export const LANGUAGE_STORAGE_KEY = 'campus-erp.language';

/**
 * The language the interface is in, and the one place that decides it.
 *
 * A picker sets it; everything else reads it. The choice is remembered in local storage so a
 * clerk who works in English does not have to say so again each morning, and it is mirrored
 * into `<html lang>` so assistive technology and hyphenation follow the same language the
 * screen is in.
 */
@Injectable({ providedIn: 'root' })
export class LanguageService {
  private readonly transloco = inject(TranslocoService);
  private readonly document = inject(DOCUMENT);

  private readonly current = signal<Language>(DEFAULT_LANGUAGE);

  /** The active language, for templates that need to mark the chosen entry. */
  readonly language = this.current.asReadonly();

  /** The languages offered, in the order the picker shows them. */
  readonly languages = SUPPORTED_LANGUAGES;

  constructor() {
    this.apply(this.stored() ?? DEFAULT_LANGUAGE);
    // Load eagerly: the dictionary ships with the bundle and resolves synchronously, so the
    // first render already has text — and `translate()` in a component never returns a key.
    this.transloco.load(this.current()).subscribe();
  }

  /** Switch the interface to `language` and remember it. */
  use(language: Language): void {
    this.apply(language);
    this.transloco.load(language).subscribe();
    this.remember(language);
  }

  /** Translate a key outside a template — the one place that reaches into the service. */
  translate(key: string): string {
    return this.transloco.translate(key);
  }

  private apply(language: Language): void {
    this.current.set(language);
    this.transloco.setActiveLang(language);
    this.document.documentElement.lang = language;
  }

  private stored(): Language | null {
    try {
      const saved = localStorage.getItem(LANGUAGE_STORAGE_KEY);
      return isLanguage(saved) ? saved : null;
    } catch {
      // A browser that refuses storage still gets a working language: the default.
      return null;
    }
  }

  private remember(language: Language): void {
    try {
      localStorage.setItem(LANGUAGE_STORAGE_KEY, language);
    } catch {
      // Persisting is a convenience, not a requirement; the session keeps the choice.
    }
  }
}
