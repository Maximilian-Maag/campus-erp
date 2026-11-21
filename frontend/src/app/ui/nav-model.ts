/**
 * The navigation: what the product is made of, in the order a clerk moves through it.
 *
 * Each entry names its route and its message key, and nothing else. The sidebar renders this
 * list and the router serves the same paths, so adding a screen is one entry here plus one
 * route — the two cannot drift into different labels for the same concept.
 */
export interface NavItem {
  /** Route path, as declared in `app.routes.ts`. */
  readonly path: string;
  /** Message key for the label, e.g. `nav.item.kasseAnordnungen`. */
  readonly labelKey: string;
}

export interface NavGroup {
  /** Message key for the group heading, e.g. `nav.group.kasse`. */
  readonly labelKey: string;
  readonly items: readonly NavItem[];
}

export const NAV_GROUPS: readonly NavGroup[] = [
  {
    labelKey: 'nav.group.kasse',
    items: [
      { path: '/kasse/tag', labelKey: 'nav.item.kasseTag' },
      { path: '/kasse/anordnungen', labelKey: 'nav.item.kasseAnordnungen' },
      { path: '/kasse/zahlungslaeufe', labelKey: 'nav.item.kasseZahlungslaeufe' },
      { path: '/kasse/kontoauszuege', labelKey: 'nav.item.kasseKontoauszuege' },
      { path: '/kasse/offene-posten', labelKey: 'nav.item.kasseOffenePosten' },
      { path: '/kasse/einzuege', labelKey: 'nav.item.kasseEinzuege' },
      { path: '/kasse/mahnwesen', labelKey: 'nav.item.kasseMahnwesen' },
      { path: '/kasse/mahnlauf', labelKey: 'nav.item.kasseMahnlauf' },
      { path: '/kasse/ruecklaeufer', labelKey: 'nav.item.kasseRuecklaeufer' },
      { path: '/kasse/mandate', labelKey: 'nav.item.kasseMandate' },
      { path: '/kasse/konten', labelKey: 'nav.item.kasseKonten' },
    ],
  },
  {
    labelKey: 'nav.group.haushalt',
    items: [
      { path: '/budget/haushaltsstellen', labelKey: 'nav.item.haushaltStellen' },
      { path: '/budget/mittelherkunft', labelKey: 'nav.item.haushaltHerkunft' },
    ],
  },
  {
    labelKey: 'nav.group.beschaffung',
    items: [
      { path: '/procurement/bestellungen', labelKey: 'nav.item.beschaffungBestellungen' },
      { path: '/procurement/wareneingang', labelKey: 'nav.item.beschaffungWareneingang' },
    ],
  },
  {
    labelKey: 'nav.group.erechnung',
    items: [
      { path: '/einvoice/rechnungen', labelKey: 'nav.item.erechnungRechnungen' },
    ],
  },
  {
    labelKey: 'nav.group.klr',
    items: [
      { path: '/klr/umlage', labelKey: 'nav.item.klrUmlage' },
      { path: '/klr/kostenarten', labelKey: 'nav.item.klrKostenarten' },
      { path: '/klr/kostentraeger', labelKey: 'nav.item.klrKostentraeger' },
      { path: '/klr/kostenstellen', labelKey: 'nav.item.klrKostenstellen' },
      { path: '/klr/verrechnungspreise', labelKey: 'nav.item.klrVerrechnungspreise' },
    ],
  },
  {
    labelKey: 'nav.group.finanzen',
    items: [
      { path: '/finanzen/stiftungen', labelKey: 'nav.item.finanzenStiftungen' },
      { path: '/finanzen/steuern', labelKey: 'nav.item.finanzenSteuern' },
    ],
  },
  {
    labelKey: 'nav.group.auswertung',
    items: [
      { path: '/auswertungen', labelKey: 'nav.item.auswertungen' },
      { path: '/importe', labelKey: 'nav.item.importe' },
    ],
  },
];
