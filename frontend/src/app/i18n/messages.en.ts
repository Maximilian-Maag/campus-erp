import type { Messages } from './messages.de';
import { sharedEn } from './shared.en';
import { kasseEn } from './pages/kasse.en';
import { kasse2En } from './pages/kasse2.en';
import { haushaltEn } from './pages/haushalt.en';
import { beschaffungEn } from './pages/beschaffung.en';
import { erechnungEn } from './pages/erechnung.en';
import { klrEn } from './pages/klr.en';
import { finanzenEn } from './pages/finanzen.en';
import { auswertungEn } from './pages/auswertung.en';

/**
 * Campus ERP — the English dictionary, composed and typed against `messages.de.ts`.
 */
export const en: Messages = {
  ...sharedEn,
  kasse: { ...kasseEn, ...kasse2En },
  haushalt: haushaltEn,
  beschaffung: beschaffungEn,
  erechnung: erechnungEn,
  klr: klrEn,
  finanzen: finanzenEn,
  auswertung: auswertungEn,
};
