import type { AuswertungMessages } from './auswertung.de';

/** Campus ERP — reporting and import: typed against `auswertung.de.ts`. */
export const auswertungEn: AuswertungMessages = {
  reports: {
    loading: 'Loading reports.',
    empty: 'No report is set up.',
    listCaption: 'Reports with their columns',
    id: 'Identifier',
    columns: 'Columns',
    report: 'Report',
    rendering: 'Scheduling the report.',
    render: 'Generate report',
    loadFailed: 'The reports could not be loaded.',
    reportRequired: 'No report is selected.',
    periodReversed: 'The period starts after it ends.',
    periodRequired: 'A period of two dates is required.',
    queued:
      'Report scheduled (job {{ job }}). The file will be at reports/{{ name }} once the worker has produced it.',
    renderFailed: 'The report could not be scheduled.',
  },
  imports: {
    loading: 'Loading imports.',
    empty: 'No imports are set up.',
    dataset: 'Dataset',
    allDatasets: 'all',
    run: 'Start import',
    datasetRequiredHint: 'An import needs a dataset to be chosen.',
    noRuns: 'No import has run yet.',
    historyCaption: 'The latest imports with time, person and result',
    timestampUtc: 'Time (UTC)',
    person: 'Person',
    result: 'Result',
    datasetsLoadFailed: 'The imports could not be loaded.',
    runsLoadFailed: 'The runs could not be read.',
    datasetRequired: 'No dataset is selected.',
    runFailed: 'The import “{{ dataset }}” failed.',
    driftChanged: 'The source has changed since the last run.',
    driftUnchanged: 'The source is unchanged since the last run.',
    firstRun: 'This was the first run of this dataset.',
    summary:
      'Import “{{ dataset }}”: {{ source }} rows read, {{ loaded }} loaded, checksum {{ checksum }}. {{ drift }}',
  },
};
