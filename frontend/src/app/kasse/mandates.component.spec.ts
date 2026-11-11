import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { MandatesComponent } from './mandates.component';
import { Mandate } from '../core/api/generated';

describe('MandatesComponent', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [MandatesComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  const mandate = (over: Partial<Mandate> = {}): Mandate => ({
    ID: 1,
    Reference: 'MND-1',
    DebtorName: 'Erika Mustermann',
    DebtorIBAN: 'DE02120300000000202051',
    DebtorBIC: 'BYLADEM1001',
    SignatureDate: '2026-04-01T00:00:00Z',
    SequenceType: 'RCUR',
    Active: true,
    ...over,
  });

  let fixture: ComponentFixture<MandatesComponent>;
  let component: MandatesComponent;
  let el: HTMLElement;

  /** Creates the screen and answers the read it starts with. */
  function screen(mandates: Mandate[] = [mandate()]): void {
    fixture = TestBed.createComponent(MandatesComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    el = fixture.nativeElement as HTMLElement;
    http.expectOne('/api/direct-debit/mandates?active=true').flush(mandates);
    fixture.detectChanges();
  }

  it('asks the API for the active mandates as soon as it is shown', () => {
    fixture = TestBed.createComponent(MandatesComponent);
    expect(fixture.componentInstance.loading()).toBeFalse();
    expect(fixture.componentInstance.mandates()).toEqual([]);
    fixture.detectChanges();

    expect(http.expectOne('/api/direct-debit/mandates?active=true').request.method).toBe('GET');
    fixture.detectChanges();
  });

  it('lists a mandate with its debtor, date, sequence type and state', () => {
    screen();
    const row = el.querySelector('[data-testid="mandate-MND-1"]')!.querySelectorAll('td');
    expect(row[0].textContent).toContain('MND-1');
    expect(row[1].textContent).toContain('Erika Mustermann');
    expect(row[2].textContent).toContain('DE02120300000000202051');
    expect(row[3].textContent).toContain('BYLADEM1001');
    expect(row[4].textContent).toContain('01.04.2026');
    expect(row[5].textContent).toContain('wiederkehrend');
    expect(row[6].textContent).toContain('aktiv');
  });

  it('shows a retired mandate as retired and a mandate without a BIC as none', () => {
    screen([mandate({ Reference: 'MND-2', DebtorBIC: '', Active: false, SequenceType: 'OOFF' })]);
    const row = el.querySelector('[data-testid="mandate-MND-2"]')!.querySelectorAll('td');
    expect(row[3].textContent).toContain('—');
    expect(row[5].textContent).toContain('einmalig');
    expect(row[6].textContent).toContain('zurückgezogen');
  });

  it('shows an unknown sequence type as its own code rather than as nothing', () => {
    screen([mandate({ Reference: 'MND-3', SequenceType: 'XXXX' })]);
    expect(el.querySelector('[data-testid="sequence-MND-3"]')!.textContent).toContain('XXXX');
  });

  it('says so when no mandate matches the filter', () => {
    screen([]);
    expect(el.textContent).toContain('Es sind keine Mandate erfasst.');
  });

  it('reports a failed load', () => {
    fixture = TestBed.createComponent(MandatesComponent);
    fixture.detectChanges();
    component = fixture.componentInstance;
    http
      .expectOne('/api/direct-debit/mandates?active=true')
      .flush({ error: 'boom' }, { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    expect(component.loadError()).toBe('Die Mandate konnten nicht geladen werden.');
    expect(component.loading()).toBeFalse();
  });

  it('narrows by state and by sequence type, asking the server each time', () => {
    screen();
    component.changeSequenceFilter('RCUR');
    http.expectOne('/api/direct-debit/mandates?active=true&sequence_type=RCUR').flush([]);
    expect(component.mandates()).toEqual([]);

    component.changeActive('');
    http.expectOne('/api/direct-debit/mandates?sequence_type=RCUR').flush([]);

    component.changeSequenceFilter('');
    http.expectOne('/api/direct-debit/mandates').flush([mandate()]);
    expect(component.mandates().length).toBe(1);
  });

  it('registers a mandate and keeps the list in reference order', () => {
    screen([mandate({ Reference: 'MND-9' })]);
    component.reference.set('MND-1');
    component.debtorName.set('Max Mustermann');
    component.debtorIBAN.set('DE02120300000000202051');
    component.debtorBIC.set('BYLADEM1001');
    component.signatureDate.set('2026-04-02');
    component.sequenceType.set('FRST');
    component.register(new Event('submit'));

    expect(component.busy()).toBeTrue();
    const req = http.expectOne('/api/direct-debit/mandates');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({
      reference: 'MND-1',
      debtor_name: 'Max Mustermann',
      debtor_iban: 'DE02120300000000202051',
      debtor_bic: 'BYLADEM1001',
      signature_date: '2026-04-02',
      sequence_type: 'FRST',
    });
    req.flush(mandate({ ID: 2, Reference: 'MND-1', SequenceType: 'FRST' }));
    fixture.detectChanges();

    expect(component.notice()).toBe('Mandat MND-1 aufgenommen.');
    expect(component.mandates().map((m) => m.Reference)).toEqual(['MND-1', 'MND-9']);
    // The form is cleared for the next one, and the sequence type stays as chosen.
    expect(component.reference()).toBe('');
    expect(component.debtorIBAN()).toBe('');
    expect(component.signatureDate()).toBe('');
    expect(component.sequenceType()).toBe('FRST');
  });

  it('refuses a mandate the screen can see is incomplete, without asking the API', () => {
    screen();
    for (const incomplete of [
      () => {},
      () => component.reference.set('MND-1'),
      () => component.debtorName.set('Max'),
      () => component.debtorIBAN.set('DE02120300000000202051'),
    ]) {
      component.reference.set('');
      component.debtorName.set('');
      component.debtorIBAN.set('');
      component.signatureDate.set('');
      incomplete();
      component.register(new Event('submit'));
      expect(component.actionError()).toBe(
        'Referenz, Schuldner, IBAN und Unterschriftsdatum sind erforderlich.'
      );
    }
    http.expectNone((r) => r.method === 'POST');
    expect(component.busy()).toBeFalse();
  });

  it('reports a refused mandate — the IBAN checksum is the server\'s', () => {
    screen();
    component.reference.set('MND-1');
    component.debtorName.set('Max');
    component.debtorIBAN.set('DE02120300000000202099');
    component.signatureDate.set('2026-04-02');
    component.register(new Event('submit'));

    http
      .expectOne('/api/direct-debit/mandates')
      .flush({ error: 'invalid IBAN' }, { status: 422, statusText: 'Unprocessable Entity' });
    fixture.detectChanges();

    expect(component.actionError()).toBe('Das Mandat konnte nicht aufgenommen werden.');
    expect(component.busy()).toBeFalse();
    // A refused mandate keeps what was typed, so it can be corrected rather than retyped.
    expect(component.reference()).toBe('MND-1');
  });
});
