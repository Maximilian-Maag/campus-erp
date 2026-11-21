/**
 * Campus ERP — E-Rechnung: the German copy of these screens.
 *
 * Field labels and messages that appear on exactly one screen live here, namespaced per
 * screen (`erechnung.<screen>.*`). A term that another screen also uses belongs in `shared.de.ts`
 * instead: reusing the shared key is what keeps one concept under one name.
 */
export const erechnungDe = {
  invoices: {
    xml: 'XRechnung (XML)',
    importButton: 'Rechnung einlesen',
    loading: 'Rechnungen werden geladen.',
    empty: 'Es sind keine Rechnungen erfasst.',
    caption: 'Eingangsrechnungen mit Datum, Betrag und der Bestellung, der sie zugeordnet sind',
    direction: 'Richtung',
    allocation: 'Zuordnung',
    documents: 'Belege',
    inbound: 'Eingang',
    outbound: 'Ausgang',
    matchedOrder: 'Bestellung {{ number }}',
    unmatched: 'offen',
    ubl: 'XRechnung (UBL)',
    cii: 'ZUGFeRD (CII)',
    match: 'Zuordnen',
    matchLabel: 'Rechnung {{ number }} der Bestellung zuordnen',
    matchFailed: 'Konnte die Rechnung {{ number }} nicht zuordnen.',
    loadFailed: 'Konnte die Rechnungen nicht laden.',
    loadMoreFailed: 'Konnte die weiteren Rechnungen nicht laden.',
    loadMore: 'Weitere laden',
    loadingMore: 'Weitere werden geladen …',
    noXml: 'Es ist kein XML eingegeben.',
    importFailed: 'Konnte die Rechnung nicht einlesen.',
  },
};

export type ErechnungMessages = typeof erechnungDe;
