import {
  ApplicationConfig,
  inject,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
  provideZoneChangeDetection,
} from '@angular/core';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { provideAnimationsAsync } from '@angular/platform-browser/animations/async';
import { provideRouter } from '@angular/router';

import { AuthConfigService } from './auth/auth-config';
import { authInterceptor } from './auth/auth.interceptor';
import { provideI18n } from './i18n/provide-i18n';
import { routes } from './app.routes';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideZoneChangeDetection({ eventCoalescing: true }),
    provideAnimationsAsync(),
    provideRouter(routes),
    provideHttpClient(withInterceptors([authInterceptor])),
    provideI18n(),
    // The deployment's sign-in configuration is read before the first render: a screen that
    // flashed before the answer would either show data it has no right to or hide a login the
    // visitor needs.
    provideAppInitializer(() => inject(AuthConfigService).load()),
  ],
};