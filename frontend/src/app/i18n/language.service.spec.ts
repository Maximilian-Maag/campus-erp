import { TestBed } from '@angular/core/testing';

import { LANGUAGE_STORAGE_KEY, LanguageService } from './language.service';
import { provideI18n } from './provide-i18n';

describe('LanguageService', () => {
  beforeEach(() => {
    localStorage.removeItem(LANGUAGE_STORAGE_KEY);
    document.documentElement.lang = '';
    TestBed.configureTestingModule({ providers: [provideI18n()] });
  });

  afterEach(() => localStorage.removeItem(LANGUAGE_STORAGE_KEY));

  it('opens in German, the language of the offices', () => {
    const service = TestBed.inject(LanguageService);
    expect(service.language()).toBe('de');
    expect(document.documentElement.lang).toBe('de');
  });

  it('offers exactly the published languages, German first', () => {
    expect(TestBed.inject(LanguageService).languages).toEqual(['de', 'en']);
  });

  it('honours the language remembered from a previous visit', () => {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, 'en');
    expect(TestBed.inject(LanguageService).language()).toBe('en');
    expect(document.documentElement.lang).toBe('en');
  });

  it('ignores a remembered value that is not a published language', () => {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, 'fr');
    expect(TestBed.inject(LanguageService).language()).toBe('de');
  });

  it('switches, remembers, and tells the document', () => {
    const service = TestBed.inject(LanguageService);

    service.use('en');

    expect(service.language()).toBe('en');
    expect(document.documentElement.lang).toBe('en');
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('en');

    service.use('de');

    expect(service.language()).toBe('de');
    expect(document.documentElement.lang).toBe('de');
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('de');
  });

  it('translates a key in the active language', () => {
    const service = TestBed.inject(LanguageService);
    expect(service.translate('common.amount')).toBe('Betrag');

    service.use('en');
    TestBed.tick();

    expect(service.translate('common.amount')).toBe('Amount');
  });

  it('still works when the browser refuses to remember the choice', () => {
    const service = TestBed.inject(LanguageService);
    spyOn(Storage.prototype, 'setItem').and.throwError('QuotaExceeded');

    expect(() => service.use('en')).not.toThrow();
    expect(service.language()).toBe('en');
  });

  it('falls back to German when storage cannot be read', () => {
    spyOn(Storage.prototype, 'getItem').and.throwError('SecurityError');
    expect(TestBed.inject(LanguageService).language()).toBe('de');
  });
});
