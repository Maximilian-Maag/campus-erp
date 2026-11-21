/**
 * Campus ERP — Haushalt: the German copy of these screens.
 *
 * Field labels and messages that appear on exactly one screen live here, namespaced per
 * screen (`haushalt.<screen>.*`). A term that another screen also uses belongs in `shared.de.ts`
 * instead: reusing the shared key is what keeps one concept under one name.
 *
 * `common` is the vocabulary these Haushalt screens share with each other but with no other area
 * of the product — it too has exactly one key per concept, so "Ansatz" is never "Betrag" beside it.
 */
export const haushaltDe = {
  common: {
    allocated: 'Ansatz',
    committed: 'Verpflichtet',
    actual: 'Ist',
    available: 'Verfügbar',
  },
  positions: {
    loading: 'Haushaltsstellen werden geladen.',
    empty: 'Es sind keine Haushaltsstellen angelegt.',
    caption: 'Ansatz, Verpflichtungen und Ist je Haushaltsstelle, Beträge in Euro',
    loadMore: 'Weitere laden',
    loadingMore: 'Weitere werden geladen …',
    loadFailed: 'Konnte die Mittelübersicht nicht laden.',
    loadMoreFailed: 'Konnte die weiteren Haushaltsstellen nicht laden.',
  },
  sources: {
    loading: 'Mittelherkunft wird geladen.',
    sourcesCaption: 'Mittelquellen',
    emptySources: 'Es ist keine Mittelquelle angelegt.',
    createSource: 'Mittelquelle anlegen',
    centresCaption: 'Bewirtschaftungsstellen',
    emptyCentres: 'Es ist keine Bewirtschaftungsstelle angelegt.',
    createCentre: 'Bewirtschaftungsstelle anlegen',
    emptyPositions: 'Es ist keine Haushaltsstelle angelegt.',
    position: 'Haushaltsstelle',
    source: 'Mittelquelle',
    centre: 'Bewirtschaftungsstelle',
    administration: 'Bewirtschaftung',
    none: 'keine',
    noneChoice: 'Keine',
    positionsCaption: 'Haushaltsstellen mit Mittelherkunft und Bewirtschaftung',
    assign: 'Zuordnung speichern',
    loadMorePositions: 'Weitere Haushaltsstellen laden',
    loadingMore: 'Weitere werden geladen …',
    importHeading: 'Budgetdatei einlesen',
    format: 'Format',
    fileContent: 'Inhalt der Budgetdatei',
    import: 'Einlesen',
    loadSourcesFailed: 'Die Mittelherkunft konnte nicht geladen werden.',
    loadCentresFailed: 'Die Bewirtschaftungsstellen konnten nicht geladen werden.',
    loadPositionsFailed: 'Die Haushaltsstellen konnten nicht geladen werden.',
    loadMoreFailed: 'Die weiteren Haushaltsstellen konnten nicht geladen werden.',
    sourceRequired: 'Schlüssel und Bezeichnung der Mittelquelle sind erforderlich.',
    centreRequired: 'Schlüssel und Bezeichnung der Bewirtschaftungsstelle sind erforderlich.',
    sourceCreated: 'Mittelquelle {{code}} angelegt.',
    centreCreated: 'Bewirtschaftungsstelle {{code}} angelegt.',
    sourceCreateFailed: 'Die Mittelquelle konnte nicht angelegt werden.',
    centreCreateFailed: 'Die Bewirtschaftungsstelle konnte nicht angelegt werden.',
    noPosition: 'Es ist keine Haushaltsstelle gewählt.',
    assignSaved: 'Die Zuordnung ist gespeichert.',
    assignFailed: 'Die Zuordnung konnte nicht gespeichert werden.',
    noFile: 'Es ist keine Budgetdatei eingelesen.',
    importFailed: 'Die Budgetdatei konnte nicht eingelesen werden.',
    importNotice:
      '{{sourcesCreated}} Mittelquellen, {{centresCreated}} Bewirtschaftungsstellen und ' +
      '{{positionsCreated}} Haushaltsstellen angelegt; {{sourcesKept}} Mittelquellen und ' +
      '{{positionsKept}} Haushaltsstellen waren schon vorhanden; ' +
      '{{positionsNamed}} Haushaltsstellen sind zugeordnet.',
  },
};

export type HaushaltMessages = typeof haushaltDe;
