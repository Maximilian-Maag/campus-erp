import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ImportsComponent } from './imports.component';
import { EtlReport, EtlRun } from '../core/api/generated';

describe('ImportsComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [ImportsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const run = (over: Partial<EtlRun> = {}): EtlRun => ({
    at: '2026-11-11T04:12:00Z',
    actor: 'alice',
    summary: 'source=3732 loaded=3732 checksum=abc1234',
    ...over,
  });

  const report = (over: Partial<EtlReport> = {}): EtlReport => ({
    dataset: 'bank-leit-zahl',
    source: 3732,
    loaded: 3732,
    checksum: 'abc1234',
    previous: 'abc1234',
    drift: false,
    at: '2026-11-11T04:12:00Z',
    ...over,
  });

  let fixture: ComponentFixture<ImportsComponent>;
  let component: ImportsComponent;
  let el: HTMLElement;

  /** Creates the screen and answers the two reads it starts with. */
  function screen(datasets: string[] = ['bank-leit-zahl', 'krankenkasse'], runs: EtlRun[] = [run()]): void {
    fixture = TestBed.createComponent(ImportsComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    el = fixture.nativeElement as HTMLElement;
    http.expectOne('/api/etl/datasets').flush({ datasets });
    http.expectOne('/api/etl/runs?limit=20').flush({ runs });
    fixture.detectChanges();
  }

  it('asks the server which datasets exist and reads the history as soon as it is shown', () => {
    fixture = TestBed.createComponent(ImportsComponent);
    fixture.detectChanges();
    expect(http.expectOne('/api/etl/datasets').request.method).toBe('GET');
    // No dataset chosen yet, so the history is every dataset's.
    expect(http.expectOne('/api/etl/runs?limit=20').request.method).toBe('GET');
    fixture.detectChanges();
  });

  it('lists the datasets the server registers and the runs as the server recorded them', () => {
    screen(['bank-leit-zahl'], [run({ actor: 'bob', summary: 'source=10 loaded=10 checksum=ffff' })]);
    const options = Array.from(el.querySelectorAll('#import-dataset option')).map((o) => o.getAttribute('value'));
    expect(options).toEqual(['', 'bank-leit-zahl']);

    const cells = el.querySelector('[data-testid="run-0"]')!.querySelectorAll('td');
    // The timestamp is UTC (the API records runs in UTC) and says so in the header.
    expect(cells[0].textContent).toContain('11.11.2026, 04:12');
    expect(cells[1].textContent).toContain('bob');
    expect(cells[2].textContent).toContain('source=10 loaded=10 checksum=ffff');
    expect(el.querySelector('caption')!.textContent).toContain('Zeitpunkt');
  });

  it('says so when nothing is set up and when nothing has run', () => {
    screen([], []);
    expect(el.textContent).toContain('Es sind keine Datenübernahmen eingerichtet.');
  });

  it('says so when the datasets exist but nothing has run yet', () => {
    screen(['bank-leit-zahl'], []);
    expect(el.textContent).toContain('Es ist noch keine Datenübernahme gelaufen.');
  });

  it('reports a failed read of the datasets and of the history', () => {
    fixture = TestBed.createComponent(ImportsComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    http
      .expectOne('/api/etl/datasets')
      .flush({ error: 'boom' }, { status: 500, statusText: 'Server Error' });
    http.expectOne('/api/etl/runs?limit=20').flush({ runs: [] });
    fixture.detectChanges();
    expect(component.loadError()).toBe('Die Datenübernahmen konnten nicht geladen werden.');

    // A history that cannot be read is reported too, and it is the later message that stands.
    component.loadHistory();
    http
      .expectOne('/api/etl/runs?limit=20')
      .flush({ error: 'boom' }, { status: 500, statusText: 'Server Error' });
    expect(component.loadError()).toBe('Die Läufe konnten nicht gelesen werden.');
  });

  it('narrows the history to the chosen dataset', () => {
    screen();
    component.choose('bank-leit-zahl');
    expect(component.dataset()).toBe('bank-leit-zahl');
    const req = http.expectOne('/api/etl/runs?dataset=bank-leit-zahl&limit=20');
    expect(req.request.method).toBe('GET');
    req.flush({ runs: [run({ actor: 'carol' })] });
    fixture.detectChanges();

    expect(component.runs().length).toBe(1);
    expect(el.querySelector('[data-testid="run-0"]')!.textContent).toContain('carol');

    // Back to every dataset: the query drops the empty filter rather than sending it.
    component.choose('');
    http.expectOne('/api/etl/runs?limit=20').flush({ runs: [] });
  });

  it('runs the chosen dataset, states what it did and reads the history back', () => {
    screen();
    component.choose('bank-leit-zahl');
    http.expectOne('/api/etl/runs?dataset=bank-leit-zahl&limit=20').flush({ runs: [] });

    component.run();
    expect(component.running()).toBeTrue();
    const req = http.expectOne('/api/etl/runs');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ dataset: 'bank-leit-zahl' });
    req.flush(report());
    fixture.detectChanges();

    expect(component.running()).toBeFalse();
    expect(component.notice()).toBe(
      'Datenübernahme „bank-leit-zahl“: 3732 Zeilen gelesen, 3732 übernommen, Prüfsumme abc1234. ' +
        'Die Quelle ist gegenüber dem letzten Lauf unverändert.'
    );
    // The run is in the history now, so it is read back rather than assumed.
    http.expectOne('/api/etl/runs?dataset=bank-leit-zahl&limit=20').flush({ runs: [run()] });
  });

  it('states a first run and a drifted source differently', () => {
    screen();
    expect(component.report(report({ previous: '' }))).toContain('Es war der erste Lauf dieses Datensatzes.');
    expect(component.report(report({ drift: true }))).toContain(
      'Die Quelle hat sich seit dem letzten Lauf geändert.'
    );
    expect(component.report(report())).toContain(
      'Die Quelle ist gegenüber dem letzten Lauf unverändert.'
    );
  });

  it('refuses to run without a chosen dataset, and reports a failed run', () => {
    screen();
    component.run();
    expect(component.actionError()).toBe('Es ist kein Datensatz gewählt.');
    http.expectNone((r) => r.method === 'POST');
    expect(component.running()).toBeFalse();

    component.choose('krankenkasse');
    http.expectOne('/api/etl/runs?dataset=krankenkasse&limit=20').flush({ runs: [] });
    component.run();
    http
      .expectOne('/api/etl/runs')
      .flush({ error: 'cannot reconcile' }, { status: 422, statusText: 'Unprocessable Entity' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Die Datenübernahme „krankenkasse“ ist fehlgeschlagen.');
    expect(component.running()).toBeFalse();
  });

  it('shows a timestamp it cannot parse as it came rather than as an empty cell', () => {
    screen([], []);
    expect(component.at('not-a-timestamp')).toBe('not-a-timestamp');
  });
});
