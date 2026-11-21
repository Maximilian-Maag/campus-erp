import type { HaushaltMessages } from './haushalt.de';

/** Campus ERP — budget: typed against `haushalt.de.ts`. */
export const haushaltEn: HaushaltMessages = {
  common: {
    allocated: 'Appropriation',
    committed: 'Committed',
    actual: 'Actual',
    available: 'Available',
  },
  positions: {
    loading: 'Budget positions are loading.',
    empty: 'No budget positions have been created.',
    caption: 'Appropriation, commitments and actuals per budget position, amounts in euros',
    loadMore: 'Load more',
    loadingMore: 'Loading more …',
    loadFailed: 'The budget overview could not be loaded.',
    loadMoreFailed: 'The remaining budget positions could not be loaded.',
  },
  sources: {
    loading: 'Funding sources are loading.',
    sourcesCaption: 'Funding sources',
    emptySources: 'No funding source has been created.',
    createSource: 'Create funding source',
    centresCaption: 'Administering centres',
    emptyCentres: 'No administering centre has been created.',
    createCentre: 'Create administering centre',
    emptyPositions: 'No budget position has been created.',
    position: 'Budget position',
    source: 'Funding source',
    centre: 'Administering centre',
    administration: 'Administration',
    none: 'none',
    noneChoice: 'None',
    positionsCaption: 'Budget positions with funding source and administering centre',
    assign: 'Save assignment',
    loadMorePositions: 'Load more budget positions',
    loadingMore: 'Loading more …',
    importHeading: 'Import budget file',
    format: 'Format',
    fileContent: 'Budget file content',
    import: 'Import',
    loadSourcesFailed: 'The funding sources could not be loaded.',
    loadCentresFailed: 'The administering centres could not be loaded.',
    loadPositionsFailed: 'The budget positions could not be loaded.',
    loadMoreFailed: 'The remaining budget positions could not be loaded.',
    sourceRequired: 'A funding source needs a code and a description.',
    centreRequired: 'An administering centre needs a code and a description.',
    sourceCreated: 'Funding source {{code}} created.',
    centreCreated: 'Administering centre {{code}} created.',
    sourceCreateFailed: 'The funding source could not be created.',
    centreCreateFailed: 'The administering centre could not be created.',
    noPosition: 'No budget position is selected.',
    assignSaved: 'The assignment is saved.',
    assignFailed: 'The assignment could not be saved.',
    noFile: 'No budget file has been read.',
    importFailed: 'The budget file could not be read.',
    importNotice:
      '{{sourcesCreated}} funding sources, {{centresCreated}} administering centres and ' +
      '{{positionsCreated}} budget positions created; {{sourcesKept}} funding sources and ' +
      '{{positionsKept}} budget positions already existed; ' +
      '{{positionsNamed}} budget positions are assigned.',
  },
};
