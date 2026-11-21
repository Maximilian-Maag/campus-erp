import { Injectable } from '@angular/core';
import { Translation, TranslocoLoader } from '@jsverse/transloco';
import { Observable, of } from 'rxjs';

import { de, Messages } from './messages.de';
import { en } from './messages.en';

/** The languages the product is published in. Adding one means adding a dictionary. */
export const SUPPORTED_LANGUAGES = ['de', 'en'] as const;

export type Language = (typeof SUPPORTED_LANGUAGES)[number];

/** German is the language of the offices that run the product. */
export const DEFAULT_LANGUAGE: Language = 'de';

const DICTIONARIES: Record<string, Messages> = { de, en };

export function isLanguage(value: string | null | undefined): value is Language {
  return SUPPORTED_LANGUAGES.includes(value as Language);
}

/**
 * Serves the dictionaries from the bundle.
 *
 * The messages ship with the application rather than over HTTP: they are part of the
 * product's vocabulary, they are type-checked against each other, and a screen must never
 * flash a key because a JSON request was still in flight. Swapping this loader for a
 * backend-driven one stays a one-line change here.
 */
@Injectable({ providedIn: 'root' })
export class DictionaryLoader implements TranslocoLoader {
  getTranslation(lang: string): Observable<Translation> {
    return of(DICTIONARIES[lang] ?? de);
  }
}
