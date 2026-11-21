import { TestBed } from '@angular/core/testing';
import { provideNoopAnimations } from '@angular/platform-browser/animations';

import { LanguagePickerComponent } from './language-picker.component';
import { LANGUAGE_STORAGE_KEY } from '../i18n/language.service';
import { provideI18n } from '../i18n/provide-i18n';

describe('LanguagePickerComponent', () => {
  beforeEach(async () => {
    localStorage.removeItem(LANGUAGE_STORAGE_KEY);
    await TestBed.configureTestingModule({
      imports: [LanguagePickerComponent],
      providers: [provideI18n(), provideNoopAnimations()],
    }).compileComponents();
  });

  afterEach(() => localStorage.removeItem(LANGUAGE_STORAGE_KEY));

  it('shows the language the interface is in', () => {
    const fixture = TestBed.createComponent(LanguagePickerComponent);
    fixture.detectChanges();
    const label = fixture.nativeElement.querySelector('[data-testid="language-current"]') as HTMLElement;
    expect(label.textContent?.trim()).toBe('Deutsch');
  });

  it('offers every published language, each named in its own language', () => {
    const fixture = TestBed.createComponent(LanguagePickerComponent);
    fixture.detectChanges();

    (fixture.nativeElement.querySelector('[data-testid="language-picker"]') as HTMLButtonElement).click();
    fixture.detectChanges();

    const entries = Array.from(
      document.querySelectorAll('button[data-testid^="language-option-"]'),
    ).map((el) => el.textContent?.trim());
    expect(entries).toEqual(['Deutsch', 'English']);
  });

  it('marks the active language as current', () => {
    const fixture = TestBed.createComponent(LanguagePickerComponent);
    fixture.componentInstance.choose('en');
    fixture.detectChanges();

    const label = fixture.nativeElement.querySelector('[data-testid="language-current"]') as HTMLElement;
    expect(label.textContent?.trim()).toBe('English');
  });

  it('switches the interface when a language is chosen', () => {
    const fixture = TestBed.createComponent(LanguagePickerComponent);

    fixture.componentInstance.choose('en');

    expect(document.documentElement.lang).toBe('en');
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('en');
  });
});
