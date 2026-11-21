import { TestBed } from '@angular/core/testing';

import { DEFAULT_LANGUAGE, DictionaryLoader, isLanguage, SUPPORTED_LANGUAGES } from './dictionary';
import { de } from './messages.de';
import { en } from './messages.en';

describe('dictionary', () => {
  it('publishes German and English, German the default', () => {
    expect(SUPPORTED_LANGUAGES).toEqual(['de', 'en']);
    expect(DEFAULT_LANGUAGE).toBe('de');
  });

  it('recognises a published language and nothing else', () => {
    expect(isLanguage('de')).toBe(true);
    expect(isLanguage('en')).toBe(true);
    expect(isLanguage('fr')).toBe(false);
    expect(isLanguage(null)).toBe(false);
    expect(isLanguage(undefined)).toBe(false);
  });

  it('gives each language its own tree', () => {
    expect(de.common.amount).toBe('Betrag');
    expect(en.common.amount).toBe('Amount');
  });

  it('has the same shape in both languages', () => {
    const keys = (tree: object): string[] =>
      Object.entries(tree)
        .flatMap(([key, value]) =>
          typeof value === 'object' && value !== null ? keys(value).map((k) => `${key}.${k}`) : [key],
        )
        .sort();

    expect(keys(en)).toEqual(keys(de));
  });

  describe('DictionaryLoader', () => {
    let loader: DictionaryLoader;

    beforeEach(() => {
      TestBed.configureTestingModule({});
      loader = TestBed.inject(DictionaryLoader);
    });

    it('serves the requested language', () => {
      let served: unknown;
      loader.getTranslation('en').subscribe((t) => (served = t));
      expect(served).toBe(en);
    });

    it('falls back to German for a language it does not have', () => {
      let served: unknown;
      loader.getTranslation('fr').subscribe((t) => (served = t));
      expect(served).toBe(de);
    });
  });
});
