import type { ErechnungMessages } from './erechnung.de';

/** Campus ERP — e-invoicing: typed against `erechnung.de.ts`. */
export const erechnungEn: ErechnungMessages = {
  invoices: {
    xml: 'XRechnung (XML)',
    importButton: 'Read invoice in',
    loading: 'Loading invoices.',
    empty: 'No invoices have been recorded.',
    caption: 'Incoming invoices with date, amount and the purchase order they are matched to',
    direction: 'Direction',
    allocation: 'Matching',
    documents: 'Documents',
    inbound: 'Incoming',
    outbound: 'Outgoing',
    matchedOrder: 'Purchase order {{ number }}',
    unmatched: 'open',
    ubl: 'XRechnung (UBL)',
    cii: 'ZUGFeRD (CII)',
    match: 'Match',
    matchLabel: 'Match invoice {{ number }} to the purchase order',
    matchFailed: 'Could not match invoice {{ number }}.',
    loadFailed: 'Could not load the invoices.',
    loadMoreFailed: 'Could not load further invoices.',
    loadMore: 'Load more',
    loadingMore: 'Loading more …',
    noXml: 'No XML has been entered.',
    importFailed: 'Could not read the invoice in.',
  },
};
