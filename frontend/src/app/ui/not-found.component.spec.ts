import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { NotFoundComponent } from './not-found.component';
import { provideI18n } from '../i18n/provide-i18n';

describe('NotFoundComponent', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [NotFoundComponent],
      providers: [provideRouter([]), provideI18n()],
    }).compileComponents();
  });

  it('says the screen does not exist and offers the way back', () => {
    const fixture = TestBed.createComponent(NotFoundComponent);
    fixture.detectChanges();

    const html = fixture.nativeElement as HTMLElement;
    expect(html.querySelector('[data-testid="page-title"]')?.textContent?.trim()).toBe(
      'Seite nicht gefunden',
    );
    const back = html.querySelector('[data-testid="not-found-back"]') as HTMLAnchorElement;
    expect(back.getAttribute('href')).toBe('/kasse/tag');
  });
});
