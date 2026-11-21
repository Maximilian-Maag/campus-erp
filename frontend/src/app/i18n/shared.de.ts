/**
 * Campus ERP — shared German messages: the frame, the navigation, the page headings and the
 * vocabulary every screen reuses.
 *
 * A term that appears on more than one screen lives here and nowhere else, so "Betrag" is
 * never "Summe" on the next page. Screen-specific copy sits in `pages/<area>.de.ts`.
 */
export const sharedDe = {
  app: {
    name: 'Campus ERP',
    tagline: 'Haushalts-, Kassen- und Kostenrechnung',
    skipToContent: 'Zum Inhalt springen',
    openMenu: 'Navigation öffnen',
    closeMenu: 'Navigation schließen',
    language: 'Sprache',
    languageSwitch: 'Sprache wechseln',
    nav: 'Hauptnavigation',
  },
  language: {
    de: 'Deutsch',
    en: 'English',
  },
  nav: {
    group: {
      kasse: 'Kasse',
      haushalt: 'Haushalt',
      beschaffung: 'Beschaffung',
      erechnung: 'E-Rechnung',
      klr: 'Kosten- und Leistungsrechnung',
      finanzen: 'Finanzen',
      auswertung: 'Auswertung',
    },
    item: {
      kasseTag: 'Tagesabschluss',
      kasseKonten: 'Eigene Konten',
      kasseAnordnungen: 'Zahlungsanordnungen',
      kasseZahlungslaeufe: 'Zahlungsläufe',
      kasseKontoauszuege: 'Kontoauszüge',
      kasseOffenePosten: 'Offene Posten',
      kasseEinzuege: 'Einzüge',
      kasseMahnwesen: 'Mahnwesen',
      kasseMahnlauf: 'Mahnlauf',
      kasseRuecklaeufer: 'Rückläufer',
      kasseMandate: 'SEPA-Mandate',
      haushaltStellen: 'Haushaltsstellen',
      haushaltHerkunft: 'Mittelherkunft',
      beschaffungBestellungen: 'Bestellungen',
      beschaffungWareneingang: 'Wareneingang',
      erechnungRechnungen: 'Eingangsrechnungen',
      klrUmlage: 'Umlage',
      klrKostenarten: 'Kostenarten',
      klrKostentraeger: 'Kostenträgerrechnung',
      klrKostenstellen: 'Kostenstellen',
      klrVerrechnungspreise: 'Verrechnungspreise',
      finanzenStiftungen: 'Stiftungen',
      finanzenSteuern: 'Steuersätze',
      auswertungen: 'Auswertungen',
      importe: 'Importe',
    },
  },
  page: {
    kasseTag: {
      title: 'Tagesabschluss',
      subtitle: 'Geschäftstag in der Kasse eröffnen und den Anfangsbestand festhalten.',
    },
    kasseKonten: {
      title: 'Eigene Konten',
      subtitle: 'Konten der Hochschule, aus denen Zahlungen geleistet werden.',
    },
    kasseAnordnungen: {
      title: 'Zahlungsanordnungen',
      subtitle: 'Zahlungen anlegen, vorlegen und im Vier-Augen-Prinzip freigeben.',
    },
    kasseZahlungslaeufe: {
      title: 'Zahlungsläufe',
      subtitle: 'Freigegebene Anordnungen zu einem Lauf bündeln und ausliefern.',
    },
    kasseKontoauszuege: {
      title: 'Kontoauszüge',
      subtitle: 'Bankauszüge einlesen und den Buchungen zuordnen.',
    },
    kasseOffenePosten: {
      title: 'Offene Posten',
      subtitle: 'Forderungen und ihre Fälligkeit im Überblick.',
    },
    kasseEinzuege: {
      title: 'Einzüge',
      subtitle: 'SEPA-Lastschriften einziehen und überwachen.',
    },
    kasseMahnwesen: {
      title: 'Mahnwesen',
      subtitle: 'Mahnstufen, Gebühren und der Stand je Forderung.',
    },
    kasseMahnlauf: {
      title: 'Mahnlauf',
      subtitle: 'Fällige Forderungen mahnen und den Lauf protokollieren.',
    },
    kasseRuecklaeufer: {
      title: 'Rückläufer',
      subtitle: 'Nicht eingelöste Lastschriften und ihre Folgebuchungen.',
    },
    kasseMandate: {
      title: 'SEPA-Mandate',
      subtitle: 'Lastschriftmandate der Schuldner verwalten.',
    },
    haushaltStellen: {
      title: 'Haushaltsstellen',
      subtitle: 'Mittelansatz, Bewirtschaftung und Restmittel je Stelle.',
    },
    haushaltHerkunft: {
      title: 'Mittelherkunft',
      subtitle: 'Mittelquellen, Bewirtschaftungsstellen und die Zuordnung der Positionen.',
    },
    beschaffungBestellungen: {
      title: 'Bestellungen',
      subtitle: 'Bestellungen anlegen und gegen die Haushaltsstelle prüfen.',
    },
    beschaffungWareneingang: {
      title: 'Wareneingang',
      subtitle: 'Lieferungen erfassen und Bestellungen abschließen.',
    },
    erechnungRechnungen: {
      title: 'Eingangsrechnungen',
      subtitle: 'E-Rechnungen prüfen, kontieren und zur Zahlung anweisen.',
    },
    klrUmlage: {
      title: 'Umlage',
      subtitle: 'Innere Verrechnung der Kosten auf Kostenstellen und Kostenträger.',
    },
    klrKostenarten: {
      title: 'Kostenarten',
      subtitle: 'Kostenarten, ihre Schlüssel und ihre Verteilung.',
    },
    klrKostentraeger: {
      title: 'Kostenträgerrechnung',
      subtitle: 'Kosten je Kostenträger und ihre Deckung.',
    },
    klrKostenstellen: {
      title: 'Kostenstellen',
      subtitle: 'Kostenstellen, ihre Leitung und ihre Zurechnung.',
    },
    klrVerrechnungspreise: {
      title: 'Verrechnungspreise',
      subtitle: 'Preise je Leistung und ihre Gültigkeit.',
    },
    finanzenStiftungen: {
      title: 'Stiftungen',
      subtitle: 'Stiftungen, ihre Zwecke und ihre Mittel.',
    },
    finanzenSteuern: {
      title: 'Steuersätze',
      subtitle: 'Steuersätze und die zuständigen Finanzämter.',
    },
    auswertungen: {
      title: 'Auswertungen',
      subtitle: 'Kennzahlen und Berichte für die Leitung.',
    },
    importe: {
      title: 'Importe',
      subtitle: 'Stammdaten und Bewegungsdaten aus Altsystemen übernehmen.',
    },
  },
  common: {
    action: 'Aktion',
    actions: 'Aktionen',
    all: 'Alle',
    amount: 'Betrag',
    cents: 'Cent',
    amountCents: 'Betrag in Cent',
    bic: 'BIC',
    cancel: 'Stornieren',
    chapter: 'Kapitel',
    choose: 'Bitte wählen',
    code: 'Schlüssel',
    costCenter: 'Kostenstelle',
    costType: 'Kostenart',
    created: 'Angelegt',
    currency: 'Währung',
    date: 'Datum',
    debtor: 'Schuldner',
    description: 'Bezeichnung',
    distributionKey: 'Verteilerschlüssel',
    due: 'Fällig',
    dueDate: 'Fälligkeit',
    from: 'Von',
    iban: 'IBAN',
    key: 'Schlüssel',
    mandate: 'Mandat',
    name: 'Name',
    note: 'Hinweis',
    number: 'Nummer',
    purpose: 'Verwendungszweck',
    reference: 'Referenz',
    sequenceType: 'Sequenztyp',
    status: 'Status',
    tariff: 'Staffeln',
    title: 'Titel',
    to: 'Bis',
    type: 'Art',
    validFrom: 'Gültig ab',
    validUntil: 'Gültig bis',
    year: 'Jahr',
    account: 'Konto',
    quantity: 'Menge',
    unit: 'Einheit',
    total: 'Summe',
    remaining: 'Restmittel',
    booked: 'Bewirtschaftet',
  },
  status: {
    draft: 'Entwurf',
    submitted: 'Vorgelegt',
    approved: 'Genehmigt',
    rejected: 'Abgelehnt',
    cancelled: 'Storniert',
    open: 'Offen',
    closed: 'Abgeschlossen',
    paid: 'Bezahlt',
    returned: 'Rückläufer',
  },
  message: {
    loading: 'Daten werden geladen.',
    empty: 'Es sind keine Einträge vorhanden.',
    loadFailed: 'Die Daten konnten nicht geladen werden.',
    actionFailed: 'Die Aktion konnte nicht ausgeführt werden.',
    saved: 'Gespeichert.',
  },
};

/** The shape every language and every message fragment must satisfy. */
export type Messages = typeof sharedDe;
