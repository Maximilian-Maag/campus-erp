import { Component } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';

import { PageComponent } from './page.component';

/**
 * The screen a link that leads nowhere lands on.
 *
 * Without it an unknown path renders an empty outlet — a blank page with no heading, which the
 * accessibility gate rightly fails and a user reasonably reads as an outage. The screen says
 * what happened and offers the way back to where the day starts.
 */
@Component({
  selector: 'app-not-found',
  imports: [PageComponent, RouterLink, TranslocoPipe],
  template: `
    <app-page titleKey="page.notFound.title" subtitleKey="page.notFound.subtitle">
      <section>
        <p>
          <a class="back-link" routerLink="/kasse/tag" data-testid="not-found-back">{{
            'page.notFound.back' | transloco
          }}</a>
        </p>
      </section>
    </app-page>
  `,
  styles: `
    .back-link {
      font: var(--mat-sys-label-large);
    }
  `,
})
export class NotFoundComponent {}
