/**
 * Campus ERP — Auswertung und Import: the German copy of these screens.
 *
 * Field labels and messages that appear on exactly one screen live here, namespaced per
 * screen (`auswertung.<screen>.*`). A term that another screen also uses belongs in `shared.de.ts`
 * instead: reusing the shared key is what keeps one concept under one name.
 */
export const auswertungDe = {
  reports: {
    loading: 'Auswertungen werden geladen.',
    empty: 'Es ist keine Auswertung eingerichtet.',
    listCaption: 'Auswertungen mit ihren Spalten',
    id: 'Kennung',
    columns: 'Spalten',
    report: 'Auswertung',
    rendering: 'Auswertung wird eingeplant.',
    render: 'Auswertung erzeugen',
    loadFailed: 'Die Auswertungen konnten nicht geladen werden.',
    reportRequired: 'Es ist keine Auswertung gewählt.',
    periodRequired: 'Ein Zeitraum aus zwei Tagen ist erforderlich.',
    queued:
      'Auswertung eingeplant (Auftrag {{ job }}). Die Datei liegt unter reports/{{ name }}, sobald der Worker sie erzeugt hat.',
    renderFailed: 'Die Auswertung konnte nicht eingeplant werden.',
  },
  imports: {
    loading: 'Datenübernahmen werden geladen.',
    empty: 'Es sind keine Datenübernahmen eingerichtet.',
    dataset: 'Datensatz',
    allDatasets: 'alle',
    run: 'Datenübernahme starten',
    datasetRequiredHint: 'Für eine Datenübernahme ist ein Datensatz zu wählen.',
    noRuns: 'Es ist noch keine Datenübernahme gelaufen.',
    historyCaption: 'Die letzten Datenübernahmen mit Zeitpunkt, Person und Ergebnis',
    timestampUtc: 'Zeitpunkt (UTC)',
    person: 'Person',
    result: 'Ergebnis',
    datasetsLoadFailed: 'Die Datenübernahmen konnten nicht geladen werden.',
    runsLoadFailed: 'Die Läufe konnten nicht gelesen werden.',
    datasetRequired: 'Es ist kein Datensatz gewählt.',
    runFailed: 'Die Datenübernahme „{{ dataset }}“ ist fehlgeschlagen.',
    driftChanged: 'Die Quelle hat sich seit dem letzten Lauf geändert.',
    driftUnchanged: 'Die Quelle ist gegenüber dem letzten Lauf unverändert.',
    firstRun: 'Es war der erste Lauf dieses Datensatzes.',
    summary:
      'Datenübernahme „{{ dataset }}“: {{ source }} Zeilen gelesen, {{ loaded }} übernommen, Prüfsumme {{ checksum }}. {{ drift }}',
  },
};

export type AuswertungMessages = typeof auswertungDe;
