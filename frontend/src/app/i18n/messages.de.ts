import { sharedDe } from './shared.de';
import { kasseDe } from './pages/kasse.de';
import { kasse2De } from './pages/kasse2.de';
import { haushaltDe } from './pages/haushalt.de';
import { beschaffungDe } from './pages/beschaffung.de';
import { erechnungDe } from './pages/erechnung.de';
import { klrDe } from './pages/klr.de';
import { finanzenDe } from './pages/finanzen.de';
import { auswertungDe } from './pages/auswertung.de';

/**
 * Campus ERP — the German dictionary, composed.
 *
 * The shared vocabulary plus one fragment per area of the product. `Messages` is the type of
 * the whole tree, and `messages.en.ts` implements it: a key added here without an English
 * counterpart is a compile error, not a surprise in front of the user.
 */
export const de = {
  ...sharedDe,
  kasse: { ...kasseDe, ...kasse2De },
  haushalt: haushaltDe,
  beschaffung: beschaffungDe,
  erechnung: erechnungDe,
  klr: klrDe,
  finanzen: finanzenDe,
  auswertung: auswertungDe,
};

/** The shape every language must implement. */
export type Messages = typeof de;
