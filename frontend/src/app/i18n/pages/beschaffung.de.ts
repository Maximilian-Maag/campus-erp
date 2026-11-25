/**
 * Campus ERP — Beschaffung: the German copy of these screens.
 *
 * Field labels and messages that appear on exactly one screen live here, namespaced per
 * screen (`beschaffung.<screen>.*`). A term that another screen also uses belongs in `shared.de.ts`
 * instead: reusing the shared key is what keeps one concept under one name.
 */
export const beschaffungDe = {
  orders: {
    loading: 'Bestellungen werden geladen.',
    empty: 'Es sind keine Bestellungen angelegt.',
    emptyFiltered: 'Keine Bestellung in diesem Status.',
    caption: 'Bestellungen mit Lieferant, Betrag und Status',
    supplier: 'Lieferant',
    billed: 'Abgerechnet',
    billedFull: '{{ amount }} (vollständig)',
    billedPartial: '{{ amount }} von {{ total }}',
    approve: 'Genehmigen',
    approveLabel: 'Bestellung {{ number }} genehmigen',
    approveFailed: 'Konnte die Bestellung {{ number }} nicht genehmigen.',
    statusReceived: 'Erhalten',
    loadFailed: 'Konnte die Bestellungen nicht laden.',
    loadMoreFailed: 'Konnte die weiteren Bestellungen nicht laden.',
    loadMore: 'Weitere laden',
    loadingMore: 'Weitere werden geladen …',
  },
  goodsReceipts: {
    loadingOrders: 'Bestellungen werden geladen.',
    loadOrdersFailed: 'Die Bestellungen konnten nicht geladen werden.',
    emptyApproved: 'Es ist keine freigegebene Bestellung offen.',
    order: 'Bestellung',
    summary: 'Bestellung {{ number }} von {{ supplier }}, Auftragssumme {{ amount }}.',
    captionProgress: 'Positionen mit bestellter, gelieferter und offener Menge',
    position: 'Position',
    ordered: 'Bestellt',
    delivered: 'Geliefert',
    open: 'Offen',
    complete: 'Die Bestellung ist vollständig geliefert.',
    reference: 'Lieferschein',
    quantityLabel: 'Menge Position {{ lineNo }} (offen {{ open }})',
    book: 'Wareneingang buchen',
    bookBusy: 'Wareneingang wird gebucht.',
    captionReceipts: 'Gebuchte Wareneingänge',
    positions: 'Positionen',
    value: 'Wert',
    loadFailed: 'Der Wareneingang konnte nicht geladen werden.',
    noOrder: 'Es ist keine Bestellung gewählt.',
    referenceRequired: 'Ein Lieferschein ist erforderlich.',
    dateRequired: 'Ein Datum ist erforderlich.',
    invalidQuantity: 'Die Menge der Position {{ lineNo }} ist keine gültige Menge.',
    noQuantity: 'Es ist keine Menge erfasst.',
    booked: 'Wareneingang gebucht ({{ amount }}).',
    bookedComplete: 'Wareneingang gebucht ({{ amount }}). Die Bestellung ist vollständig geliefert.',
    bookFailed: 'Der Wareneingang konnte nicht gebucht werden.',
    overReceipt: 'Die Menge übersteigt, was noch offen ist.',
    receiptExists: 'Diese Lieferscheinnummer ist bereits gebucht.',
    quantityExceedsOpen: 'Zeile {{ lineNo }}: {{ typed }} ist mehr als die {{ open }} offen.',
  },
};

export type BeschaffungMessages = typeof beschaffungDe;
