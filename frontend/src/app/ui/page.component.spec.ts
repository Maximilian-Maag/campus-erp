import { TestBed } from '@angular/core/testing';

import { PageComponent } from './page.component';
import { provideI18n } from '../i18n/provide-i18n';
import { LANGUAGE_STORAGE_KEY, LanguageService } from '../i18n/language.service';

describe('PageComponent', () => {
  // The language is remembered in storage, which outlives a TestBed: a test that switches to
  // English must not leave every later spec in it.
  afterEach(() => localStorage.removeItem(LANGUAGE_STORAGE_KEY));

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PageComponent],
      providers: [provideI18n()],
    }).compileComponents();
  });

  it('renders the heading and the description from their message keys', () => {
    const fixture = TestBed.createComponent(PageComponent);
    fixture.componentRef.setInput('titleKey', 'page.kasseAnordnungen.title');
    fixture.componentRef.setInput('subtitleKey', 'page.kasseAnordnungen.subtitle');
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="page-title"]')?.textContent?.trim()).toBe(
      'Zahlungsanordnungen',
    );
    expect(html.querySelector('.page-subtitle')?.textContent?.trim()).toBe(
      'Zahlungen anlegen, vorlegen und im Vier-Augen-Prinzip freigeben.',
    );
  });

  it('omits the description line when the screen has no subtitle', () => {
    const fixture = TestBed.createComponent(PageComponent);
    fixture.componentRef.setInput('titleKey', 'page.auswertungen.title');
    fixture.detectChanges();

    expect((fixture.nativeElement as HTMLElement).querySelector('.page-subtitle')).toBeNull();
  });

  it('gives the screen action a place of its own', () => {
    const fixture = TestBed.createComponent(PageComponent);
    fixture.componentRef.setInput('titleKey', 'page.auswertungen.title');
    fixture.detectChanges();

    expect((fixture.nativeElement as HTMLElement).querySelector('.page-actions')).not.toBeNull();
  });

  it('names the browser tab after the screen', () => {
    const fixture = TestBed.createComponent(PageComponent);
    fixture.componentRef.setInput('titleKey', 'page.kasseAnordnungen.title');
    fixture.detectChanges();

    expect(document.title).toBe('Zahlungsanordnungen · Campus ERP');
  });

  it('renames the tab when the language changes', () => {
    const fixture = TestBed.createComponent(PageComponent);
    fixture.componentRef.setInput('titleKey', 'page.kasseAnordnungen.title');
    fixture.detectChanges();

    TestBed.inject(LanguageService).use('en');
    fixture.detectChanges();

    expect(document.title).toBe('Payment orders · Campus ERP');
  });
});
