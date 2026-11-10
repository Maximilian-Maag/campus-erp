import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { AuswertungenComponent, artefactName, compactDay, isDay } from './auswertungen.component';
import { Definition } from '../core/api/generated';

describe('AuswertungenComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [AuswertungenComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const definition = (over: Partial<Definition> = {}): Definition => ({
    ID: 'klr-allocation-overview',
    Title: 'Umlageregeln',
    Columns: [
      { Key: 'source', Title: 'Von Kostenstelle', Align: 'left' },
      { Key: 'share', Title: 'Anteil', Align: 'right' },
    ],
    ...over,
  });

  let fixture: ComponentFixture<AuswertungenComponent>;
  let component: AuswertungenComponent;
  let el: HTMLElement;

  /** Creates the screen and answers the catalogue read with the given reports. */
  function screen(reports: Definition[] = [definition()]): void {
    fixture = TestBed.createComponent(AuswertungenComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    el = fixture.nativeElement as HTMLElement;
    http.expectOne('/api/reports').flush(reports);
    fixture.detectChanges();
  }

  it('lists the reports the catalogue holds, with their columns', () => {
    screen();
    expect(component.loading()).toBeFalse();
    expect(component.loadError()).toBeNull();
    const row = el.querySelector('[data-testid="report-klr-allocation-overview"]')!;
    expect(row.textContent).toContain('klr-allocation-overview');
    expect(row.textContent).toContain('Umlageregeln');
    expect(row.textContent).toContain('Von Kostenstelle, Anteil');
    expect(el.querySelector('#report-id')).not.toBeNull();
  });

  it('reports a failed read and offers nothing to render', () => {
    fixture = TestBed.createComponent(AuswertungenComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    el = fixture.nativeElement as HTMLElement;
    http.expectOne('/api/reports').flush({ error: 'boom' }, { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    expect(component.loadError()).toBe('Die Auswertungen konnten nicht geladen werden.');
    expect(el.querySelector('form')).toBeNull();
  });

  it('states an empty catalogue', () => {
    screen([]);
    expect(el.textContent).toContain('keine Auswertung eingerichtet');
    expect(el.querySelector('table')).toBeNull();
  });

  it('queues a render for the stated period and links the file it will produce', () => {
    screen();
    component.report.set('klr-allocation-overview');
    component.from.set('2026-04-01');
    component.to.set('2026-04-30');
    component.render(new Event('submit'));

    expect(component.busy()).toBeTrue();
    const req = http.expectOne('/api/reports/renders');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({
      report_id: 'klr-allocation-overview',
      from: '2026-04-01',
      to: '2026-04-30',
    });
    req.flush({ job_id: 42 });
    fixture.detectChanges();

    expect(component.busy()).toBeFalse();
    expect(component.notice()).toBe(
      'Auswertung eingeplant (Auftrag 42). Die Datei liegt unter reports/klr-allocation-overview-20260401-20260430.pdf, sobald der Worker sie erzeugt hat.'
    );
    const link = el.querySelector('a[href="/api/artefacts/reports/klr-allocation-overview-20260401-20260430.pdf"]');
    expect(link).not.toBeNull();
    expect(link!.textContent).toContain('klr-allocation-overview-20260401-20260430.pdf');
  });

  it('reports a refused render', () => {
    screen();
    component.report.set('klr-allocation-overview');
    component.from.set('2026-04-01');
    component.to.set('2026-04-30');
    component.render(new Event('submit'));
    http.expectOne('/api/reports/renders').flush(
      { error: 'unknown report' },
      { status: 500, statusText: 'Server Error' }
    );
    fixture.detectChanges();

    expect(component.busy()).toBeFalse();
    expect(component.actionError()).toBe('Die Auswertung konnte nicht eingeplant werden.');
    expect(component.notice()).toBeNull();
    expect(component.document()).toBeNull();
    expect(el.textContent).toContain('Die Auswertung konnte nicht eingeplant werden.');
  });

  it('refuses an incomplete request before asking the API', () => {
    screen();

    component.render(new Event('submit'));
    expect(component.actionError()).toBe('Es ist keine Auswertung gewählt.');

    component.report.set('klr-allocation-overview');
    component.from.set('01.04.2026');
    component.render(new Event('submit'));
    expect(component.actionError()).toBe('Ein Zeitraum aus zwei Tagen ist erforderlich.');

    component.from.set('2026-04-01');
    component.to.set('');
    component.render(new Event('submit'));
    expect(component.actionError()).toBe('Ein Zeitraum aus zwei Tagen ist erforderlich.');

    http.expectNone('/api/reports/renders');
    expect(component.busy()).toBeFalse();
  });

  it('names the artefact the render job stores, and reads the day form', () => {
    expect(artefactName('kasse-daily-close', '2026-04-01', '2026-04-30')).toBe(
      'kasse-daily-close-20260401-20260430.pdf'
    );
    expect(compactDay('2026-04-01')).toBe('20260401');
    expect(isDay('2026-04-01')).toBeTrue();
    expect(isDay('01.04.2026')).toBeFalse();
    expect(isDay('')).toBeFalse();
  });

  it('renders the column list of a report', () => {
    screen();
    expect(component.columnList(definition())).toBe('Von Kostenstelle, Anteil');
    expect(component.columnList(definition({ Columns: [] }))).toBe('');
  });
});
