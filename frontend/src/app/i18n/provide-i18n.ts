import { EnvironmentProviders, inject, makeEnvironmentProviders, provideAppInitializer } from '@angular/core';
import { provideTransloco } from '@jsverse/transloco';

import { DEFAULT_LANGUAGE, DictionaryLoader, SUPPORTED_LANGUAGES } from './dictionary';
import { LanguageService } from './language.service';

/**
 * The product's translation setup: the dictionaries, the fallback, and the language the
 * application opens in.
 *
 * `provideAppInitializer` instantiates `LanguageService` before the first render, so the
 * remembered language is applied once — rather than the interface painting in German and
 * re-rendering into English a frame later.
 */
export function provideI18n(): EnvironmentProviders {
  return makeEnvironmentProviders([
    provideTransloco({
      config: {
        availableLangs: [...SUPPORTED_LANGUAGES],
        defaultLang: DEFAULT_LANGUAGE,
        fallbackLang: DEFAULT_LANGUAGE,
        reRenderOnLangChange: true,
        prodMode: true,
      },
      loader: DictionaryLoader,
    }),
    provideAppInitializer(() => {
      inject(LanguageService);
    }),
  ]);
}
