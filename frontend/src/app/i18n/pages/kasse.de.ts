/**
 * Campus ERP — Kasse: the German copy of these screens.
 *
 * Field labels and messages that appear on exactly one screen live here, namespaced per
 * screen (`kasse.<screen>.*`). A term that another screen also uses belongs in `shared.de.ts`
 * instead: reusing the shared key is what keeps one concept under one name.
 */
export const kasseDe = {
  tag: {
    businessDate: 'Geschäftstag',
    openingCents: 'Anfangsbestand (Cent)',
    open: 'Tag eröffnen',
    dayId: 'Tag-ID',
    openFailed: 'Der Kassenbuch-Tag konnte nicht eröffnet werden.',
  },
};

export type KasseMessages = typeof kasseDe;
